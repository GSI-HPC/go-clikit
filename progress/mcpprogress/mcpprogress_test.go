// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package mcpprogress_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/mcpprogress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// note is one notification sent.
type note struct {
	progress, total float64
	message         string
}

// client records the notifications a Notifier sends, as a session would.
type client struct {
	mu    sync.Mutex
	notes []note
	// fail is the error each Send returns, and panic what it panics with.
	fail  error
	panic any
}

func (c *client) send(progress, total float64, message string) error {
	c.mu.Lock()
	c.notes = append(c.notes, note{progress, total, message})
	c.mu.Unlock()
	if c.panic != nil {
		panic(c.panic)
	}
	return c.fail
}

func (c *client) sent() []note {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]note(nil), c.notes...)
}

// call runs work as a call whose progress c receives, and returns what was
// sent by the time the Notifier closed. It fails the test if anything is
// sent after that.
func call(t *testing.T, c *client, o mcpprogress.Options, work func(ctx context.Context)) []note {
	t.Helper()
	o.Send = c.send
	n := mcpprogress.New(o)
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{n, capture}})
	work(progress.WithBus(context.Background(), bus))
	bus.Close()
	n.Close()
	progresstest.Check(t, capture.Events())
	notes := c.sent()
	time.Sleep(10 * time.Millisecond)
	if late := c.sent()[len(notes):]; len(late) > 0 {
		t.Errorf("sent after Close: %+v", late)
	}
	return notes
}

// step starts a step of targets, as fanout.Map does, and returns it with
// the targets, queued.
func step(ctx context.Context, name string, targets int, opts ...progress.Option) (*progress.Span, []*progress.Span) {
	opts = append([]progress.Option{progress.Flagged(progress.FlagFold), progress.Total(targets)}, opts...)
	stepCtx, s := progress.Start(ctx, progress.KindStep, name, opts...)
	spans := make([]*progress.Span, targets)
	for i := range spans {
		node := fmt.Sprintf("exe%d", i+1)
		_, spans[i] = progress.Start(stepCtx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	return s, spans
}

// checkProgress fails the test unless the notifications count up to total:
// the targets ended grow with every notification, as MCP asks, and never
// pass what is expected, which never falls, and the last says all of it
// ended.
func checkProgress(t *testing.T, notes []note, total float64) {
	t.Helper()
	if len(notes) == 0 {
		t.Fatal("no progress was sent")
	}
	done, expected := -1.0, 0.0
	for i, n := range notes {
		if n.progress <= done || n.total < expected || n.progress > n.total {
			t.Errorf("notification %d says %v of %v after %v of %v", i, n.progress, n.total, done, expected)
		}
		done, expected = n.progress, n.total
	}
	if last := notes[len(notes)-1]; last.progress != total || last.total != total {
		t.Errorf("the last notification says %v of %v, want %v of %v", last.progress, last.total, total, total)
	}
}

// A call's progress is sent as it goes, at most every Interval and as a
// step ends, counted up to what the call expects and never back.
func TestProgressIsSentAsTheCallGoes(t *testing.T) {
	c := &client{}
	notes := call(t, c, mcpprogress.Options{}, func(ctx context.Context) {
		s, targets := step(ctx, "probe the nodes", 6)
		for _, target := range targets {
			target.Run()
			time.Sleep(mcpprogress.Interval / 4)
			target.End(nil)
		}
		s.End(nil)
	})
	checkProgress(t, notes, 6)
	if len(notes) < 3 || len(notes) > 6 {
		t.Errorf("%d notifications for a call of 1.5 intervals, want one at its start, one or two while it ran and one as its step ended: %+v", len(notes), notes)
	}
	for _, n := range notes {
		if !regexp.MustCompile(`^probe the nodes: \d/6 done$`).MatchString(n.message) {
			t.Errorf("message %q, want the step and how far it has got", n.message)
		}
	}
}

// What a call leaves open is ended as its Bus is closed, and counted in the
// last notification, which Close sends.
func TestProgressCountsWhatTheCallLeftOpen(t *testing.T) {
	c := &client{}
	notes := call(t, c, mcpprogress.Options{}, func(ctx context.Context) {
		_, targets := step(ctx, "probe the nodes", 4)
		targets[0].Run()
		targets[0].End(nil)
	})
	checkProgress(t, notes, 4)
}

// A step that starts once another has ended raises what is expected, but
// no notification says so until more targets have ended, since each must
// say more are done than the last; and a line about the step under way
// says what the numbers count in all once they count more than it.
func TestProgressOfStepsInTurnOnlyGrows(t *testing.T) {
	c := &client{}
	notes := call(t, c, mcpprogress.Options{}, func(ctx context.Context) {
		for _, name := range []string{"read the power state", "read the uptime"} {
			s, targets := step(ctx, name, 2)
			for _, target := range targets {
				target.Run()
				target.End(nil)
			}
			s.End(nil)
		}
	})
	checkProgress(t, notes, 4)
	for _, n := range notes {
		if n.total > 2 && !strings.HasSuffix(n.message, fmt.Sprintf("; %v/%v in all", n.progress, n.total)) {
			t.Errorf("%v of %v: %q, want the line to end with what the numbers count in all", n.progress, n.total, n.message)
		}
	}
	if last := notes[len(notes)-1].message; last != "read the uptime: 2/2 done; 4/4 in all" {
		t.Errorf("the last line is %q", last)
	}
}

// The line says how the targets ended, and which batch ran last.
func TestTheLineSaysHowTheTargetsEnded(t *testing.T) {
	c := &client{}
	notes := call(t, c, mcpprogress.Options{}, func(ctx context.Context) {
		stepCtx, s := progress.Start(ctx, progress.KindStep, "reboot", progress.Flagged(progress.FlagFold), progress.Total(4))
		errs := []error{nil, errors.New("no route to host"), context.Canceled, progress.Skip("drained")}
		ctxs, batches := make([]context.Context, len(errs)), make([]*progress.Span, len(errs))
		for i := range errs {
			ctxs[i], batches[i] = progress.Start(stepCtx, progress.KindBatch, fmt.Sprintf("%d/4", i+1),
				progress.Queued(), progress.Batch(i+1, 4), progress.Total(1))
		}
		for i, err := range errs {
			batches[i].Run()
			node := fmt.Sprintf("exe%d", i+1)
			_, target := progress.Start(ctxs[i], progress.KindTarget, node, progress.Queued(), progress.Node(node))
			target.Run()
			target.End(err)
			batches[i].End(nil)
		}
		s.End(nil)
	})
	checkProgress(t, notes, 4)
	if last := notes[len(notes)-1].message; last != "reboot: batch 4/4, 4/4 done, 1 failed, 1 canceled, 1 skipped" {
		t.Errorf("the last line is %q", last)
	}
}

// A call that counts nothing, or counts only in hidden steps, sends
// nothing.
func TestNothingCountedSendsNothing(t *testing.T) {
	for name, work := range map[string]func(ctx context.Context){
		"no step": func(ctx context.Context) {
			_, cmd := progress.Start(ctx, progress.KindCommand, "status")
			cmd.End(nil)
		},
		"a hidden step": func(ctx context.Context) {
			s, targets := step(ctx, "probe the nodes", 2, progress.Flagged(progress.FlagHidden))
			for _, target := range targets {
				target.End(nil)
			}
			s.End(nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := &client{}
			if notes := call(t, c, mcpprogress.Options{}, work); len(notes) != 0 {
				t.Errorf("sent %+v", notes)
			}
		})
	}
}

// A Send that fails, as on a session that has gone, stops the Notifier.
func TestASendThatFailsStopsTheNotifier(t *testing.T) {
	c := &client{fail: errors.New("connection closed")}
	notes := call(t, c, mcpprogress.Options{}, func(ctx context.Context) {
		s, targets := step(ctx, "probe the nodes", 2)
		for _, target := range targets {
			target.End(nil)
			s.End(nil) // urgent, each time; only the first is sent.
		}
	})
	if len(notes) != 1 {
		t.Errorf("%d notifications were tried, want the one that failed alone: %+v", len(notes), notes)
	}
}

// A Send that panics stops the notifications of its call, with the stack
// in the panic log, and ends neither the call nor the server.
func TestASendThatPanicsStopsAndSaysWhy(t *testing.T) {
	for _, program := range []string{"prog", ""} {
		t.Run(program, func(t *testing.T) {
			var log bytes.Buffer
			c := &client{panic: "the session is gone"}
			notes := call(t, c, mcpprogress.Options{PanicLog: &log, Program: program}, func(ctx context.Context) {
				s, targets := step(ctx, "read the groups", 2)
				for _, target := range targets {
					target.Run()
					target.End(nil)
				}
				s.End(nil)
			})
			want := "progress notifications stopped: the session is gone\n"
			if program != "" {
				want = program + ": " + want
			}
			if !strings.HasPrefix(log.String(), want) || !strings.Contains(log.String(), "goroutine") {
				t.Errorf("the log says %q, want %q and the stack", log.String(), want)
			}
			if len(notes) != 1 {
				t.Errorf("%d notifications were tried, want the one that panicked alone", len(notes))
			}
		})
	}
}

// A second Close does nothing, and a Notifier with no panic log given
// writes to standard error.
func TestCloseTwice(t *testing.T) {
	c := &client{}
	n := mcpprogress.New(mcpprogress.Options{Send: c.send})
	n.Close()
	n.Close()
	if notes := c.sent(); len(notes) != 0 {
		t.Errorf("sent %+v", notes)
	}
}

// Close sends what the interval held back.
func TestCloseSendsWhatWasHeldBack(t *testing.T) {
	c := &client{}
	n := mcpprogress.New(mcpprogress.Options{Send: c.send})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{n}})
	_, targets := step(progress.WithBus(context.Background(), bus), "probe the nodes", 2)
	deadline := time.Now().Add(5 * time.Second)
	for len(c.sent()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	// Within the interval, a target that ends is held back.
	targets[0].Run()
	targets[0].End(nil)
	n.Close()
	bus.Close()
	want := []note{{0, 2, "probe the nodes: 0/2 done"}, {1, 2, "probe the nodes: 1/2 done"}}
	if got := c.sent(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("sent %+v, want %+v", got, want)
	}
}
