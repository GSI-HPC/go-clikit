// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// logged runs work on a Bus that writes an event log, closes both, and
// returns the log with every span id the Bus drew replaced by "#n", n in
// the order the spans started, and the events a capture was sent, which
// asks for lines as a live display does.
func logged(t *testing.T, o progress.BusOptions, work func(ctx context.Context)) (string, []progress.Event) {
	t.Helper()
	var out bytes.Buffer
	log := progress.NewLog(&out, progress.LogOptions{Run: "0123456789abcdef", Version: "v1.2.3"})
	capture := &progresstest.Capture{Lines: true}
	o.Sinks = []progress.Sink{capture, log}
	bus := progress.NewBus(o)
	work(progress.WithBus(context.Background(), bus))
	bus.Close()
	if err := log.Close(); err != nil {
		t.Fatalf("closing the log: %v", err)
	}
	events := capture.Events()
	progresstest.Check(t, events)
	var ids []string
	for _, e := range events {
		if e.Type == progress.TypeStart {
			ids = append(ids, e.Span.String(), fmt.Sprintf("#%d", len(ids)/2+1))
		}
	}
	return strings.NewReplacer(ids...).Replace(out.String()), events
}

// update rewrites the fixtures a test compares with, rather than failing.
var update = flag.Bool("update", false, "rewrite testdata/log-v1.jsonl with what the log writes")

// logFixture is what version 1 of the event log is, line for line: a run
// that has every key and every value of a key whose values are a set of
// the kit's, and an example of the others, which a reader of version 1 may
// count on. A change to it is a change to the log's format, which keeps
// its version only when no key and no value means anything else.
const logFixture = "testdata/log-v1.jsonl"

// classed is an error of the class it says.
type classed struct {
	class progress.Class
	msg   string
}

func (e classed) Error() string                 { return e.msg }
func (e classed) ProgressClass() progress.Class { return e.class }

// The event log has the run and its trace first, where the trace came from
// with it and the version that wrote it, and then one line for every event
// the Bus sent, in its order, with what the event says in fields of its
// own, as the fixture has it. A line of output is there without its text.
func TestTheEventLogWritesEveryEventOnALineOfItsOwn(t *testing.T) {
	t.Parallel()

	tc, ok := progress.ParseTraceContext("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "rojo=00f067aa0ba902b7")
	if !ok {
		t.Fatal("the traceparent was refused")
	}
	clock := newClock()
	log, events := logged(t, progress.BusOptions{
		Now: clock.Now, Trace: tc,
	}, func(busCtx context.Context) {
		ctx, cmd := progress.Start(busCtx, progress.KindCommand, "exec", progress.WithFlags(progress.DryRun))
		_, cred := progress.Start(ctx, progress.KindCall, "credential bmc", progress.WithFlags(progress.Hidden), progress.Source("env"))
		cred.End(nil)
		stepCtx, step := progress.Start(ctx, progress.KindStep, "run",
			progress.WithFlags(progress.Fold|progress.ShowLines), progress.Total(2), progress.Limit(1))
		ctx1, one := progress.Start(stepCtx, progress.KindTarget, "exe0001", progress.Queued(),
			progress.Node("exe0001"), progress.Host("exe0001.hpc.example.org"), progress.Role("compute"))
		ctx2, two := progress.Start(stepCtx, progress.KindTarget, "exe0002", progress.Queued(),
			progress.Node("exe0002"), progress.Host("exe0002.hpc.example.org"))
		clock.Add(time.Second)
		one.Run()
		callCtx, call := progress.Start(ctx1, progress.KindCall, "ssh", progress.Timeout(1500*time.Millisecond))
		_, _ = io.WriteString(progress.Tee(callCtx, io.Discard, progress.Stdout, nil), "the secret line\n")
		_, _ = io.WriteString(progress.Tee(callCtx, io.Discard, progress.Stderr, nil), "the secret warning\n")
		clock.Add(time.Second)
		call.End(nil, progress.Exit(0))
		one.End(nil)
		two.Run()
		_, get := progress.Start(ctx2, progress.KindCall, "redfish", progress.HTTP("GET", "/redfish/v1/Systems/1"),
			progress.Host("exe0002.mgmt"))
		get.End(classed{progress.ClassTransport, "exe0002.mgmt: no answer"}, progress.HTTPStatus(503))
		two.End(classed{progress.ClassTransport, "exe0002.mgmt: no answer"})
		step.Update(progress.Message("1 of 2 failed"))
		step.End(classed{progress.ClassTarget, "1 of 2 hosts failed: exe0002"})

		// A staggered power on in two batches: the first ends each way a
		// target can fail, and the second is left out.
		powerCtx, power := progress.Start(ctx, progress.KindStep, "power on",
			progress.WithFlags(progress.Fold), progress.Total(6), progress.Limit(5))
		nodes := []string{"exe0003", "exe0004", "exe0005", "exe0006", "exe0007"}
		batchCtx, first := progress.Start(powerCtx, progress.KindBatch, "batch 1/2", progress.Queued(),
			progress.Batch(1, 2), progress.Node("exe[0003-0007]"), progress.Total(len(nodes)), progress.Limit(5))
		_, second := progress.Start(powerCtx, progress.KindBatch, "batch 2/2", progress.Queued(),
			progress.Batch(2, 2), progress.Node("exe0008"), progress.Total(1), progress.Limit(5))
		first.Run()
		targets := make([]*progress.Span, len(nodes))
		for i, node := range nodes {
			_, targets[i] = progress.Start(batchCtx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
		}
		for i, class := range []progress.Class{progress.ClassTimeout, progress.ClassAuth, progress.ClassPin, progress.ClassUsage, progress.ClassCanceled} {
			targets[i].Run()
			targets[i].End(classed{class, nodes[i] + ": " + class.String()})
		}
		first.End(classed{progress.ClassTarget, "5 of 5 hosts failed"})
		second.Skip("not tried: an earlier batch failed")
		_, pause := progress.Start(powerCtx, progress.KindWait, "stagger", progress.Timeout(30*time.Second))
		pause.End(context.Canceled)
		power.End(classed{progress.ClassTarget, "5 of 6 hosts failed"})

		waitCtx, wait := progress.Start(ctx, progress.KindWait, "confirm", progress.Message("reset 2 hosts"))
		progress.Suspend(waitCtx)()
		wait.Skip("dry run: nothing was done")
		cmd.End(nil)

		// The lines above are those version 1 was first released with, and
		// stay as they are: what the fixture says since is added after
		// them, by a second command on the same Bus.
		ctx, status := progress.Start(busCtx, progress.KindCommand, "status")
		// Lookups answered from each place a cache can answer from.
		for _, cache := range []string{"hit", "miss", "memory", "disk"} {
			_, lookup := progress.Start(ctx, progress.KindCall, "inventory", progress.WithFlags(progress.Hidden))
			lookup.End(nil, progress.Cache(cache))
		}
		// A burst of output longer than the lines a display is sent at
		// once: the line before the last is left out, and the last is sent
		// as the call ends, saying so.
		solCtx, sol := progress.Start(ctx, progress.KindCall, "sol", progress.WithFlags(progress.ShowLines))
		_, _ = io.WriteString(progress.Tee(solCtx, io.Discard, progress.Stdout, nil), strings.Repeat("the secret console\n", 22))
		sol.End(nil)
		status.End(nil)
	})

	if *update {
		if err := os.WriteFile(logFixture, []byte(log), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(logFixture)
	if err != nil {
		t.Fatal(err)
	}
	if log != string(want) {
		t.Errorf("event log:\n%s\nwant %s:\n%s", log, logFixture, want)
	}
	if n := strings.Count(log, "\n"); n != len(events)+1 {
		t.Errorf("%d lines for %d events and the trace", n, len(events))
	}
	if strings.Contains(log, "secret") {
		t.Errorf("the log holds the text of a line:\n%s", log)
	}
}

// releasedLines is how many lines the fixture had when version 1 was first
// released, the trace and 47 events, and releasedSum the SHA-256 of them.
const (
	releasedLines = 48
	releasedSum   = "77a0022057971a5a026b06b63cd9241dadc18832d5e9865a4a2b44f939475b88"
)

// The fixture is changed only by adding to it: the lines version 1 was
// first released with are its first lines, byte for byte, so that a
// rewrite with -update that changes one of them fails here.
func TestTheGoldenLogKeepsTheLinesFirstReleased(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(logFixture)
	if err != nil {
		t.Fatal(err)
	}
	var released []byte
	n := 0
	for line := range strings.Lines(string(data)) {
		if n == releasedLines {
			break
		}
		released = append(released, line...)
		n++
	}
	sum := sha256.Sum256(released)
	if n != releasedLines || hex.EncodeToString(sum[:]) != releasedSum {
		t.Errorf("the first %d lines of %s are not those version 1 was released with: add to the file, do not change it", releasedLines, logFixture)
	}
}

// Every value of every kind of thing an event says has a name of its own
// in the log, and the fixture says each of them, so that a reader of
// version 1 knows them all: a value added without a name, or not in the
// fixture, fails this test. The values are counted in the source, so that
// none is missed here. Every key an event is logged with is on a line of
// the fixture too, and so is every value of Cache.
func TestEveryValueHasANameInTheLog(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile(logFixture)
	if err != nil {
		t.Fatal(err)
	}
	var lines []map[string]any
	for _, text := range strings.Split(strings.TrimSpace(string(fixture)), "\n") {
		var line map[string]any
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			t.Fatalf("%s: %q: %v", logFixture, text, err)
		}
		lines = append(lines, line)
	}
	said := func(key, value string) bool {
		for _, line := range lines {
			switch v := line[key].(type) {
			case string:
				if v == value {
					return true
				}
			case []any:
				if slices.Contains(v, any(value)) {
					return true
				}
			}
		}
		return false
	}

	values := map[string][]fmt.Stringer{
		"Kind":   {progress.KindCommand, progress.KindStep, progress.KindBatch, progress.KindTarget, progress.KindCall, progress.KindWait},
		"State":  {progress.StateQueued, progress.StateRunning, progress.StateEnded},
		"Status": {progress.StatusOK, progress.StatusFailed, progress.StatusCanceled, progress.StatusSkipped},
		"Class":  {progress.ClassTarget, progress.ClassTransport, progress.ClassTimeout, progress.ClassAuth, progress.ClassPin, progress.ClassUsage, progress.ClassCanceled},
		"Flags":  {progress.Hidden, progress.Fold, progress.ShowLines, progress.DryRun},
		"Type":   {progress.TypeStart, progress.TypeRun, progress.TypeUpdate, progress.TypeLine, progress.TypeEnd, progress.TypeSuspend, progress.TypeResume},
		"Stream": {progress.Stdout, progress.Stderr},
	}
	keys := map[string]string{"Kind": "kind", "State": "state", "Status": "status", "Class": "class", "Flags": "flags", "Type": "type", "Stream": "stream"}
	// ClassNone is a span that did not fail, and has no class in the log.
	declared := declaredConstants(t, "progress.go")
	declared["Class"]--
	for typ, vs := range values {
		if declared[typ] != len(vs) {
			t.Errorf("progress.go declares %d values of %s, and this test knows %d: name the new one here", declared[typ], typ, len(vs))
		}
		seen := map[string]bool{}
		for _, v := range vs {
			name := v.String()
			if name == "" || strings.ContainsAny(name, "(),") || seen[name] {
				t.Errorf("%s value %q has no name of its own in the log", typ, name)
			}
			seen[name] = true
			if !said(keys[typ], name) {
				t.Errorf("%s has no line whose %q is %q", logFixture, keys[typ], name)
			}
		}
	}

	// Cache is a string, but one of a set that the log's reference lists.
	for _, name := range []string{"hit", "miss", "memory", "disk"} {
		if !said("cache", name) {
			t.Errorf("%s has no line whose %q is %q", logFixture, "cache", name)
		}
	}
	// And every key an event may have is on a line of the fixture.
	for member, key := range loggedAs {
		if key == "" {
			continue
		}
		if !slices.ContainsFunc(lines, func(line map[string]any) bool { _, ok := line[key]; return ok }) {
			t.Errorf("%s has no line with %q, which Event.%s is logged as", logFixture, key, member)
		}
	}
}

// loggedAs is the key each member of Event is logged under, "" for never.
var loggedAs = map[string]string{
	"Seq": "seq", "Time": "time", "Type": "type", "Span": "span", "Parent": "parent",
	"Kind": "kind", "Name": "name", "Flags": "flags", "State": "state",
	"Node": "node", "Host": "host", "Role": "role", "Total": "total", "Limit": "limit",
	"Batch": "batch", "Message": "message", "Method": "method", "Path": "path",
	"HTTPStatus": "httpStatus", "Cache": "cache", "Source": "source", "Timeout": "timeout",
	"Exit": "exit", "Status": "status", "Class": "class", "Err": "err",
	"Stream": "stream", "Dropped": "dropped",
	// The text of a line of output never leaves the process.
	"Text": "",
}

// declaredConstants counts the constants of each named type file declares,
// the way the compiler gives them their types: a constant with no type and
// no value has the type of the one before it.
func declaredConstants(t *testing.T, file string) map[string]int {
	t.Helper()
	f, err := goparser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		typ := ""
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			switch {
			case value.Type != nil:
				typ = ""
				if id, ok := value.Type.(*ast.Ident); ok {
					typ = id.Name
				}
			case len(value.Values) > 0:
				typ = ""
			}
			if typ != "" {
				counts[typ] += len(value.Names)
			}
		}
	}
	return counts
}

// A trace that began here has no parent, and a parent's flags are written
// even when they are all zero, a trace the other program does not sample.
// The run and the version are there either way.
func TestTheEventLogSaysWhereItsTraceCameFrom(t *testing.T) {
	t.Parallel()

	first := func(o progress.BusOptions) map[string]any {
		t.Helper()
		log, _ := logged(t, o, func(context.Context) {})
		var line map[string]any
		if err := json.Unmarshal([]byte(strings.SplitN(log, "\n", 2)[0]), &line); err != nil {
			t.Fatalf("the first line of %q: %v", log, err)
		}
		return line
	}
	here := first(progress.BusOptions{})
	if len(here) != 5 || here["type"] != "trace" || len(here["trace"].(string)) != 32 || here["v"] != float64(1) ||
		here["run"] != "0123456789abcdef" || here["version"] != "v1.2.3" {
		t.Errorf("the first line of a trace that began here: %v", here)
	}
	tc, _ := progress.ParseTraceContext("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00", "")
	unsampled := first(progress.BusOptions{Trace: tc})
	if unsampled["parent"] != "00f067aa0ba902b7" || unsampled["traceFlags"] != "00" || unsampled["traceState"] != nil {
		t.Errorf("the first line of a trace not sampled: %v", unsampled)
	}
}

// Everything an event says is either written to the log under a key of its
// own or left out on purpose, and nothing else is written. A member added
// to Event or Fields fails this test until it is decided which it is, so
// that nothing reaches the log, and whoever reads it, by default.
func TestEveryPartOfAnEventIsLoggedOrLeftOutOnPurpose(t *testing.T) {
	t.Parallel()

	decided := loggedAs
	var e progress.Event
	var members []string
	fill(t, reflect.ValueOf(&e).Elem(), &members)
	slices.Sort(members)
	if known := slices.Sorted(maps.Keys(decided)); !slices.Equal(members, known) {
		for _, m := range members {
			if _, ok := decided[m]; !ok {
				t.Errorf("Event.%s is neither logged nor left out: give it a key in progress.Log, "+
					"or leave it out on purpose, and say which here", m)
			}
		}
		for _, m := range known {
			if !slices.Contains(members, m) {
				t.Errorf("%s is decided on here, but Event has no such member", m)
			}
		}
	}

	var out bytes.Buffer
	log := progress.NewLog(&out, progress.LogOptions{Run: "0123456789abcdef"})
	log.Handle(e)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	if err := json.Unmarshal(out.Bytes(), &line); err != nil {
		t.Fatalf("%q: %v", out.String(), err)
	}
	want := []string{"run", "trace", "v"}
	for _, key := range decided {
		if key != "" {
			want = append(want, key)
		}
	}
	slices.Sort(want)
	if got := slices.Sorted(maps.Keys(line)); !slices.Equal(got, want) {
		t.Errorf("an event with every member set is logged with the keys\n%v\nwant\n%v", got, want)
	}
	if strings.Contains(out.String(), e.Text) {
		t.Errorf("the log holds the text of a line: %s", out.String())
	}
}

// fill sets every member of v, which is a struct, to a value that is not
// zero, and adds the names of the members it set to names, those of an
// embedded struct as its own.
func fill(t *testing.T, v reflect.Value, names *[]string) {
	t.Helper()
	for i := range v.NumField() {
		f, field := v.Field(i), v.Type().Field(i)
		switch {
		case field.Anonymous:
			fill(t, f, names)
			continue
		case field.Type == reflect.TypeFor[time.Time]():
			f.Set(reflect.ValueOf(time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)))
		case f.Kind() == reflect.String:
			f.SetString("a value of " + field.Name)
		case f.CanInt():
			f.SetInt(1)
		case f.CanUint():
			f.SetUint(1)
		case f.Kind() == reflect.Pointer && f.Type().Elem().Kind() == reflect.Int:
			f.Set(reflect.New(f.Type().Elem()))
		default:
			t.Fatalf("Event.%s is a %s, which this test cannot set; teach fill how", field.Name, field.Type)
		}
		*names = append(*names, field.Name)
	}
}

// unwritable is a writer that fails every write, and counts them.
type unwritable struct {
	mu     sync.Mutex
	writes int
}

func (f *unwritable) Write([]byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	return 0, errors.New("no space left on device")
}

// A log that cannot be written stops at its first failure and says why
// when it is closed, and the work, and the other sinks, go on as if it
// had not.
func TestALogThatCannotBeWrittenStopsAndSaysWhy(t *testing.T) {
	t.Parallel()

	w := &unwritable{}
	log := progress.NewLog(w, progress.LogOptions{Run: "0123456789abcdef"})
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log, capture}})
	ctx := progress.WithBus(context.Background(), bus)
	const n = 2000
	for i := range n {
		_, s := progress.Start(ctx, progress.KindCall, "ssh", progress.Node(fmt.Sprintf("exe%04d", i)))
		s.End(nil)
	}
	bus.Close()
	err := log.Close()
	if err == nil || !strings.Contains(err.Error(), "no space left") {
		t.Errorf("Close = %v, want the error of the write", err)
	}
	if w.writes != 1 {
		t.Errorf("%d writes were tried, want the one that failed", w.writes)
	}
	if got := len(capture.Events()); got != 2*n {
		t.Errorf("the other sink was sent %d events, want %d", got, 2*n)
	}
	if again := log.Close(); !errors.Is(again, err) {
		t.Errorf("a second Close = %v, want %v again", again, err)
	}
}

// writes keeps each write it is given apart.
type writes struct {
	mu   sync.Mutex
	each [][]byte
}

func (w *writes) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.each = append(w.each, bytes.Clone(p))
	return len(p), nil
}

// written reports what w has been written.
func written(w *writes) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(bytes.Join(w.each, nil))
}

// soon waits for what w has been written to hold want, and fails the test
// when it does not within five seconds.
func soon(t *testing.T, w *writes, want, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(written(w), want) {
		if time.Now().After(deadline) {
			t.Fatalf("%s was not written within 5s: %q", what, written(w))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A line is written within FlushEvery, whatever comes after it, and as
// soon as a step, a batch or the command ends, so that a log followed as it
// grows, or that of a run killed, is never far behind.
func TestTheLogIsWrittenSoonAfterEachLine(t *testing.T) {
	t.Parallel()

	t.Run("within FlushEvery", func(t *testing.T) {
		t.Parallel()
		w := &writes{}
		log := progress.NewLog(w, progress.LogOptions{Run: "0123456789abcdef", FlushEvery: 20 * time.Millisecond})
		bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}})
		_, cmd := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "exec")
		soon(t, w, `"kind":"command"`, "the command's start")
		cmd.End(nil)
		bus.Close()
		if err := log.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("as a step ends", func(t *testing.T) {
		t.Parallel()
		w := &writes{}
		// Nothing but the end of the step has the lines written here.
		log := progress.NewLog(w, progress.LogOptions{Run: "0123456789abcdef", FlushEvery: time.Hour})
		bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}})
		ctx, cmd := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "exec")
		_, step := progress.Start(ctx, progress.KindStep, "run")
		step.End(nil)
		soon(t, w, `"type":"end","span"`, "the end of the step")
		cmd.End(nil)
		bus.Close()
		if err := log.Close(); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(written(w), "\n"); n != 5 {
			t.Errorf("%d lines written, want the trace and 4 events:\n%s", n, written(w))
		}
	})
}

// stalled is a writer that takes nothing until released, as a pipe does
// whose reader has stopped.
type stalled struct{ release chan struct{} }

func (s stalled) Write(p []byte) (int, error) {
	<-s.release
	return len(p), nil
}

// A log whose writer has stopped taking its lines holds nothing up: the
// events go on as fast as the work makes them, and Close gives up once it
// has waited CloseWait, saying that the log stops short.
func TestALogThatStallsHoldsNothingUp(t *testing.T) {
	t.Parallel()
	w := stalled{release: make(chan struct{})}
	defer close(w.release)
	log := progress.NewLog(w, progress.LogOptions{FlushEvery: time.Millisecond, CloseWait: 50 * time.Millisecond})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}})
	ctx := progress.WithBus(context.Background(), bus)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 20000 {
			_, s := progress.Start(ctx, progress.KindCall, "ssh", progress.Node(fmt.Sprintf("exe%05d", i)))
			s.End(nil)
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the events waited for a writer that took nothing")
	}
	bus.Close()
	start := time.Now()
	err := log.Close()
	if err == nil || !strings.Contains(err.Error(), "were not written") {
		t.Errorf("Close = %v, want it to say the log stops short", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Close took %v, want about CloseWait", elapsed)
	}
}

// gate is a writer that takes nothing until it is opened, as a pipe does
// whose reader has paused, and then keeps all it is given.
type gate struct {
	open chan struct{}
	writes
}

func (g *gate) Write(p []byte) (int, error) {
	<-g.open
	return g.writes.Write(p)
}

// A log that falls behind keeps the lines it had already taken and leaves
// out only those that came after: once the writer takes lines again, what
// it is given is the run's first events, seq 1 up without a gap, as much as
// fitted in 8 MiB, and Close says why the rest are missing.
func TestALogThatFallsBehindKeepsWhatItHad(t *testing.T) {
	t.Parallel()
	w := &gate{open: make(chan struct{})}
	log := progress.NewLog(w, progress.LogOptions{Run: "0123456789abcdef", FlushEvery: time.Millisecond, CloseWait: time.Minute})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}})
	ctx := progress.WithBus(context.Background(), bus)
	const n = 50000
	for i := range n {
		_, s := progress.Start(ctx, progress.KindCall, "ssh", progress.Node(fmt.Sprintf("exe%05d", i)))
		s.End(nil)
	}
	close(w.open)
	bus.Close()
	err := log.Close()
	if err == nil || !strings.Contains(err.Error(), "not written as fast as the work went on") {
		t.Errorf("Close = %v, want it to say the log fell behind", err)
	}
	out := written(&w.writes)
	if len(out) < 7<<20 {
		t.Errorf("%d bytes written, want the 8 MiB the log had taken before it fell behind", len(out))
	}
	seq := 0.0
	for line := range strings.Lines(out) {
		var got map[string]any
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("a line that is not whole: %q", line)
		}
		if got["type"] == "trace" {
			continue
		}
		if s, _ := got["seq"].(float64); s != seq+1 {
			t.Fatalf("seq %v after %v: a gap in the lines the log had taken", s, seq)
		}
		seq++
	}
	if seq == 0 || seq >= 2*n {
		t.Errorf("%v events written of %d, want the first of them and not all", seq, 2*n)
	}
}

// The log is written in pieces, each of them whole lines, so that the
// lines of runs appending to one file at once do not cut into each other;
// a line longer than the buffer is written on its own.
func TestTheLogIsWrittenInWholeLines(t *testing.T) {
	t.Parallel()

	w := &writes{}
	log := progress.NewLog(w, progress.LogOptions{Run: "0123456789abcdef"})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}})
	ctx := progress.WithBus(context.Background(), bus)
	for i := range 3000 {
		_, s := progress.Start(ctx, progress.KindCall, "ssh", progress.Node(fmt.Sprintf("exe%04d", i)))
		s.End(nil)
	}
	// A node set is never cut, so a line can be longer than the buffer.
	_, s := progress.Start(ctx, progress.KindTarget, "a batch", progress.Node(strings.Repeat("exe0001,", 10000)))
	s.End(nil)
	bus.Close()
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if len(w.each) < 3 {
		t.Fatalf("%d writes; the test wants the buffer filled more than once", len(w.each))
	}
	lines := 0
	for i, p := range w.each {
		if !bytes.HasSuffix(p, []byte("\n")) {
			t.Fatalf("write %d ends inside a line: %q", i+1, p[max(0, len(p)-40):])
		}
		for line := range bytes.Lines(p) {
			if !json.Valid(line) {
				t.Fatalf("write %d holds a line that is not JSON: %q", i+1, line)
			}
			lines++
		}
	}
	if want := 1 + 2*3000 + 2; lines != want {
		t.Errorf("%d lines written, want %d", lines, want)
	}
}

// Two runs appending to one file at once, as the commands of a CI job that
// names one log file for all of them do, and sharing the trace the job
// hands on, leave lines that are whole, each of one run, and each run's
// own events, seq 1 up without a gap, in their order.
func TestTwoRunsAppendingToOneFileCanBeToldApart(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/progress.jsonl"
	trace := progress.TraceID{0x4b, 0xf9, 0x2f, 0x35}
	const events = 4000
	runs := []string{"aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"}
	var wg sync.WaitGroup
	for _, run := range runs {
		wg.Go(func() {
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = f.Close() }()
			log := progress.NewLog(f, progress.LogOptions{Run: run})
			bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}, Trace: progress.TraceContext{Trace: trace}})
			ctx := progress.WithBus(context.Background(), bus)
			for i := range events / 2 {
				_, s := progress.Start(ctx, progress.KindCall, "ssh", progress.Node(fmt.Sprintf("exe%04d", i)))
				s.End(nil)
			}
			bus.Close()
			if err := log.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	seq := map[string]float64{}
	for line := range bytes.Lines(data) {
		var got map[string]any
		if err := json.Unmarshal(line, &got); err != nil {
			t.Fatalf("a line that is not whole: %q", line)
		}
		run, _ := got["run"].(string)
		if !slices.Contains(runs, run) {
			t.Fatalf("a line of no run: %q", line)
		}
		if got["trace"] != trace.String() {
			t.Fatalf("a line of another trace: %q", line)
		}
		if got["type"] == "trace" {
			continue
		}
		if n, _ := got["seq"].(float64); n != seq[run]+1 {
			t.Fatalf("run %s: seq %v after %v", run, n, seq[run])
		}
		seq[run]++
	}
	for _, run := range runs {
		if seq[run] != events {
			t.Errorf("run %s: %v events, want %d", run, seq[run], events)
		}
	}
}

// A log whose writer takes nothing, though it is not behind, says at Close
// that its last lines were not written.
func TestALogThatCannotFinishSaysSo(t *testing.T) {
	t.Parallel()
	w := stalled{release: make(chan struct{})}
	defer close(w.release)
	log := progress.NewLog(w, progress.LogOptions{FlushEvery: time.Millisecond, CloseWait: 20 * time.Millisecond})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}})
	_, s := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCall, "ssh")
	s.End(nil)
	bus.Close()
	if err := log.Close(); err == nil || !strings.Contains(err.Error(), "not written within 20ms") {
		t.Errorf("Close = %v, want it to say the last lines were not written within 20ms", err)
	}
}

// counted is a writer whose first write waits until it is released, and
// which counts the writes it is given.
type counted struct {
	release chan struct{}
	mu      sync.Mutex
	writes  int
}

func (c *counted) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writes++
	first := c.writes == 1
	c.mu.Unlock()
	if first {
		<-c.release
	}
	return len(p), nil
}

func (c *counted) Writes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes
}

// Once Close has given up waiting, the log starts no write: only the one
// under way when the time ran out goes on, and Done says when it is over,
// so that the writer can then be closed.
func TestALogStartsNoWriteOnceCloseHasGivenUp(t *testing.T) {
	t.Parallel()
	w := &counted{release: make(chan struct{})}
	log := progress.NewLog(w, progress.LogOptions{FlushEvery: time.Hour, CloseWait: 20 * time.Millisecond})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}})
	ctx := progress.WithBus(context.Background(), bus)
	// Well over the 64 KiB at which the log writes, so that the first write
	// is under way and more lines wait behind it.
	for i := range 2000 {
		_, s := progress.Start(ctx, progress.KindCall, fmt.Sprintf("ssh %d", i))
		s.End(nil)
	}
	bus.Close()
	err := log.Close()
	if err == nil || !strings.Contains(err.Error(), "may still be under way") {
		t.Errorf("Close = %v, want it to say a write may still be under way", err)
	}
	select {
	case <-log.Done():
		t.Fatal("Done was closed while a write was under way")
	default:
	}
	close(w.release)
	select {
	case <-log.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Done was not closed once the write had returned")
	}
	if n := w.Writes(); n != 1 {
		t.Errorf("the log made %d writes, want only the one under way when Close gave up", n)
	}
}

// The first line names the program that wrote the log when the log was
// told it, and leaves the key out otherwise, as the fixture shows.
func TestTheEventLogNamesTheProgram(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	log := progress.NewLog(&out, progress.LogOptions{Run: "0123456789abcdef", Program: "sind", Version: "v1.2.3"})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}})
	bus.Close()
	if err := log.Close(); err != nil {
		t.Fatalf("closing the log: %v", err)
	}
	first, _, _ := strings.Cut(out.String(), "\n")
	if !strings.HasSuffix(first, `,"program":"sind","version":"v1.2.3"}`) {
		t.Errorf("the first line is %s, want the program before the version", first)
	}
}

// A log told no program takes the Bus's, and one told its own keeps it,
// so that a program that names itself once, on the Bus, finds its name in
// the log.
func TestTheEventLogTakesTheProgramFromTheBus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, log, bus, want string }{
		{"from the bus", "", "sind", `"program":"sind"`},
		{"the log's own", "clusterctl", "sind", `"program":"clusterctl"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			log := progress.NewLog(&out, progress.LogOptions{Run: "0123456789abcdef", Program: tc.log})
			bus := progress.NewBus(progress.BusOptions{Program: tc.bus, Sinks: []progress.Sink{log}})
			bus.Close()
			if err := log.Close(); err != nil {
				t.Fatalf("closing the log: %v", err)
			}
			first, _, _ := strings.Cut(out.String(), "\n")
			if !strings.HasSuffix(first, ","+tc.want+"}") {
				t.Errorf("the first line is %s, want %s", first, tc.want)
			}
		})
	}
}
