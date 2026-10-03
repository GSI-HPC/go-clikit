// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progresstest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// fakeTB is a testing.TB that keeps what a helper reports, and the
// functions it registers to run when the test ends, for a test of the
// helper itself.
type fakeTB struct {
	testing.TB
	errors   []string
	cleanups []func()
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}

func (f *fakeTB) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }

// end runs what was registered to run when the test ends, the last first.
func (f *fakeTB) end() {
	for _, fn := range slices.Backward(f.cleanups) {
		fn()
	}
}

// Check reports each broken promise as an error of its own, prefixed
// "progress: ", and past twenty the number of the rest in one.
func TestCheckReportsEachProblemAndHowManyMore(t *testing.T) {
	t.Parallel()

	var es events
	for i := range 25 {
		es = es.add(start, progress.SpanID(i+1), 0, call)
	}
	tb := &fakeTB{}
	Check(tb, es)
	if len(tb.errors) != 21 {
		t.Fatalf("Check reported %d errors, want 21: %q", len(tb.errors), tb.errors)
	}
	if got := tb.errors[0]; got != `progress: call "s1" (event 1) never ends` {
		t.Errorf("the first error is %q", got)
	}
	if got := tb.errors[20]; got != "progress: and 5 more" {
		t.Errorf("the last error is %q, want the number of the rest", got)
	}

	tb = &fakeTB{}
	Check(tb, events{}.add(start, 1, 0, call).add(end, 1, 0, call))
	if len(tb.errors) != 0 {
		t.Errorf("Check reported %q of events that keep every promise", tb.errors)
	}
}

// A test that never calls Finish has its events checked when it ends,
// before the Bus is closed, so that a span the work left open is
// reported, not ended by Close; and the lines of output are asked for.
func TestWatchFinishesWhenTheTestEnds(t *testing.T) {
	t.Parallel()

	tb := &fakeTB{}
	ctx, w := Watch(context.Background(), tb)
	_, span := progress.Start(ctx, progress.KindCall, "ssh")
	if !w.capture.WantsLines() {
		t.Error("the capture of Watch asks for no lines")
	}
	tb.end()
	if len(tb.errors) != 1 || !strings.Contains(tb.errors[0], `call "ssh" (event 1) never ends`) {
		t.Errorf("Watch reported %q, want the span that never ends", tb.errors)
	}
	// The Bus is closed once the check has run: nothing more is sent.
	n := len(w.Events())
	span.End(nil)
	if len(w.Events()) != n {
		t.Error("the Bus of Watch is still open once the test has ended")
	}
}

// Finish checks the events before it closes the Bus, and returns their
// tree, which the Bus's Close has changed nothing in; called again, or
// when the test ends, it checks nothing more. Events and Tree read the
// events so far and check nothing.
func TestWatchChecksBeforeTheBusIsClosed(t *testing.T) {
	t.Parallel()

	tb := &fakeTB{}
	ctx, w := Watch(context.Background(), tb)
	_, done := progress.Start(ctx, progress.KindCall, "ssh")
	done.End(nil)
	_, open := progress.Start(ctx, progress.KindCall, "scp")
	if got, want := w.Tree(), "call scp: running\ncall ssh: ok\n"; got != want {
		t.Errorf("tree so far:\n%s\nwant:\n%s", got, want)
	}
	if got := len(w.Events()); got != 3 {
		t.Errorf("%d events so far, want 3", got)
	}
	if len(tb.errors) != 0 {
		t.Errorf("Events and Tree reported %q", tb.errors)
	}
	got := w.Finish()
	if len(tb.errors) != 1 || !strings.Contains(tb.errors[0], `call "scp" (event 3) never ends`) {
		t.Errorf("Watch reported %q, want the span that never ends", tb.errors)
	}
	want := "call scp: canceled (canceled): not finished\ncall ssh: ok\n"
	if got != want {
		t.Errorf("tree:\n%s\nwant:\n%s", got, want)
	}
	if again := w.Finish(); again != want {
		t.Errorf("Finish again drew:\n%s\nwant:\n%s", again, want)
	}
	tb.end()
	if len(tb.errors) != 1 {
		t.Errorf("a second Finish reported %q", tb.errors[1:])
	}
	open.End(nil)
}

// Classify has the Bus of Watch class an error by the fallback
// given, as the program's own Bus would.
func TestClassifySetsTheFallbackOfTheBus(t *testing.T) {
	t.Parallel()

	unreachable := errors.New("no route to host")
	ctx, w := Watch(context.Background(), t, Classify(func(error) progress.Class { return progress.ClassTransport }))
	_, span := progress.Start(ctx, progress.KindCall, "ssh")
	span.End(unreachable)
	if got, want := w.Finish(), "call ssh: failed (transport): no route to host\n"; got != want {
		t.Errorf("tree:\n%s\nwant:\n%s", got, want)
	}
}

// sinkFunc is a sink that calls itself with every event.
type sinkFunc func(progress.Event)

func (f sinkFunc) Handle(e progress.Event) { f(e) }

// Sinks puts sinks on the Bus of Watch, each use adding to
// the ones before, ahead of the Capture, which no option takes off; the
// slice given is left as it was.
func TestSinksAddsSinksAheadOfTheCapture(t *testing.T) {
	t.Parallel()

	var order []string
	first := sinkFunc(func(progress.Event) { order = append(order, "first") })
	untouched := &Capture{}
	given := []progress.Sink{first, untouched}
	ctx, w := Watch(context.Background(), t,
		Sinks(given[:1]...), Classify(nil), Sinks(sinkFunc(func(progress.Event) {
			order = append(order, "third")
		})))
	if given[1] != untouched {
		t.Error("Sinks wrote into the slice it was given")
	}
	_, span := progress.Start(ctx, progress.KindCall, "ssh")
	if want := []string{"first", "third"}; !slices.Equal(order, want) {
		t.Errorf("the sinks given saw the start in the order %q, want %q", order, want)
	}
	span.End(nil)
	if got, want := w.Finish(), "call ssh: ok\n"; got != want {
		t.Errorf("the Capture drew\n%s\nwant\n%s", got, want)
	}
}

// Every string field of an Event, its Fields among them, is checked for
// escapes by Check and drawn by Tree, so that a field added to either is
// not left out of both without a decision. Text is left out of the tree on
// purpose: lines of output are never drawn.
func TestEveryTextOfAnEventIsCheckedAndDrawn(t *testing.T) {
	t.Parallel()

	notDrawn := map[string]bool{"Text": true}
	for _, name := range stringFields(reflect.TypeFor[progress.Event]()) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := progress.Event{Seq: 1, Type: progress.TypeStart, Span: 1, Kind: progress.KindCall, State: progress.StateRunning}
			reflect.ValueOf(&e).Elem().FieldByName(name).SetString("\x1b[2J")
			var problems []string
			checkText(e, func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) })
			if len(problems) == 0 {
				t.Errorf("Check does not look at the %s of an event", name)
			}

			if notDrawn[name] {
				return
			}
			marker := "marker-" + strings.ToLower(name)
			start := progress.Event{Seq: 1, Type: progress.TypeStart, Span: 1, Kind: progress.KindCall, State: progress.StateRunning}
			end := start
			end.Seq, end.Type, end.State, end.Status = 2, progress.TypeEnd, progress.StateEnded, progress.StatusFailed
			reflect.ValueOf(&start).Elem().FieldByName(name).SetString(marker)
			reflect.ValueOf(&end).Elem().FieldByName(name).SetString(marker)
			if got := tree([]progress.Event{start, end}); !strings.Contains(got, marker) {
				t.Errorf("Tree does not draw the %s of a span: %q", name, got)
			}
		})
	}
}

// stringFields returns the names of the string fields of t, those of the
// structs it embeds among them.
func stringFields(t reflect.Type) []string {
	var names []string
	for f := range t.Fields() {
		switch {
		case f.Anonymous && f.Type.Kind() == reflect.Struct:
			names = append(names, stringFields(f.Type)...)
		case f.Type.Kind() == reflect.String:
			names = append(names, f.Name)
		}
	}
	return names
}

// Tree draws the fields of a span that have a value, the status of an
// HTTP request among them.
func TestTreeDrawsTheStatusOfARequest(t *testing.T) {
	t.Parallel()

	c := &Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{c}})
	_, span := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCall, "redfish",
		progress.HTTP("GET", "/redfish/v1"), progress.Timeout(time.Second))
	span.End(nil, progress.HTTPStatus(200))
	bus.Close()
	if got, want := c.Tree(), "call redfish method=GET path=/redfish/v1 http=200 timeout=1s: ok\n"; got != want {
		t.Errorf("tree:\n%s\nwant:\n%s", got, want)
	}
}

// natural compares the numbers in names by value, whatever their leading
// zeros, and a name before a longer one it begins.
func TestNaturalOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"batch 2/10", "batch 10/10", -1},
		{"exe007a", "exe7b", -1},
		{"exe7", "exe007", 0},
		{"exe", "exe1", -1},
		{"b", "a", 1},
	} {
		if got := natural(tc.a, tc.b); got != tc.want {
			t.Errorf("natural(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
