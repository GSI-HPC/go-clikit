// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
	"github.com/GSI-HPC/go-nodeset"
)

// lifecycle is the life every display lives, as the package comment gives
// it: a sink of the Bus, suspended around a question, started, drawn on
// demand and closed. The assertions below hold the displays to it at
// compile time; the package exports no such interface.
type lifecycle interface {
	progress.Sink
	progress.Suspender
	Start()
	Draw()
	Close()
}

var (
	_ lifecycle         = (*display.Tree)(nil)
	_ lifecycle         = (*display.Counter)(nil)
	_ lifecycle         = (*display.Plain)(nil)
	_ progress.LineSink = (*display.Tree)(nil)
	_ progress.Sink     = (*display.Summary)(nil)
)

// The tests in this file hold a display to what it promises the command it
// shares the terminal with, as a program sees it: a command runs its work
// in a pool, writes its output and its questions through the Terminal, and
// a progresstest.Screen shows what a terminal would, erasing included.

// session is a command on a terminal, a progresstest.Screen, with a display
// drawn only when the test calls draw, on a clock that moves a second each
// time, which the Bus reads too. The Summary is the line the command
// leaves, and its events are checked when it ends.
type session struct {
	ctx     context.Context
	command *progress.Span
	bus     *progress.Bus
	capture *progresstest.Capture
	summary *display.Summary
	screen  *progresstest.Screen
	shown   drawer
	// out and errOut are the command's standard output and standard
	// error, both on the terminal.
	out, errOut io.Writer
	clock       *clock

	mu     sync.Mutex
	frames []string
}

// drawer is a display a session draws by hand.
type drawer interface {
	progress.Sink
	Draw()
	Close()
}

// newSession starts command on a terminal 100 columns wide and 40 rows high,
// with the display newDisplay makes.
func newSession(t *testing.T, command string, newDisplay func(*display.Terminal, func() time.Time) drawer) *session {
	t.Helper()
	s := &session{
		screen:  &progresstest.Screen{Width: 100},
		clock:   &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)},
		capture: &progresstest.Capture{},
		summary: &display.Summary{},
	}
	term := display.NewTerminal(s.screen, display.TerminalOptions{
		Size:    func() (int, int, error) { return 100, 40, nil },
		Program: "prog",
	})
	s.out, s.errOut = term.Writer(s.screen), term.Writer(s.screen)
	s.shown = newDisplay(term, s.clock.Now)
	s.bus = progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{s.capture, s.shown, s.summary}, Now: s.clock.Now})
	s.ctx, s.command = progress.Start(progress.WithBus(context.Background(), s.bus), progress.KindCommand, command)
	t.Cleanup(s.bus.Close)
	return s
}

func counterDisplay(term *display.Terminal, now func() time.Time) drawer {
	return display.NewCounter(term, display.CounterOptions{Now: now})
}

func treeDisplay(term *display.Terminal, now func() time.Time) drawer {
	return display.NewTree(term, display.TreeOptions{Now: now})
}

// draw moves the clock on by a second, draws a frame and keeps what the
// screen shows then.
func (s *session) draw() {
	s.clock.Add(time.Second)
	s.shown.Draw()
	s.mu.Lock()
	s.frames = append(s.frames, s.screen.String())
	s.mu.Unlock()
}

// end ends the command with err, checks its events, closes the display, and
// writes the summary and the error the way a command line does; it returns
// what the screen shows then.
func (s *session) end(t *testing.T, err error) string {
	t.Helper()
	s.command.End(err)
	progresstest.Check(t, s.capture.Events())
	s.bus.Close()
	s.shown.Close()
	if line := s.summary.Line(); line != "" {
		_, _ = fmt.Fprintf(s.errOut, "prog: %s\n", line)
	}
	if err != nil {
		_, _ = fmt.Fprintf(s.errOut, "prog: %v\n", err)
	}
	return s.screen.String()
}

// checkFrames compares the frames drawn with want, each given with a
// leading newline.
func (s *session) checkFrames(t *testing.T, want ...string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, frame := range s.frames {
		if i < len(want) && frame != want[i][1:] {
			t.Errorf("frame %d:\n%s\nwant:\n%s", i+1, frame, want[i][1:])
		}
	}
	if len(s.frames) != len(want) {
		t.Errorf("%d frames, want %d", len(s.frames), len(want))
	}
}

func checkEnd(t *testing.T, got, want string) {
	t.Helper()
	if got != want[1:] {
		t.Errorf("terminal:\n%s\nwant:\n%s", got, want[1:])
	}
}

// uptime runs on each node, one at a time, drawing a frame once it has
// started, and fails on exe0002; the command writes what each node said
// once the pool is done, as a command that collects output does.
func uptime(s *session, call bool) error {
	nodes := []string{"exe0001", "exe0002", "exe0003"}
	outcomes, _ := fanout.Map(s.ctx, nodes, fanout.MapOptions[string]{Step: "run", Limit: 1},
		func(ctx context.Context, node string) (string, error) {
			if call {
				_, span := progress.Start(ctx, progress.KindCall, "ssh", progress.Timeout(10*time.Minute))
				defer span.End(nil)
			}
			s.draw()
			if node == "exe0002" {
				return "", fmt.Errorf("%s: command exited 1", node)
			}
			return "up 3 days", nil
		})
	summary := fanout.Summary{Total: len(nodes)}
	for i, o := range outcomes {
		if o.Err != nil {
			_, _ = fmt.Fprintf(s.errOut, "%s: %v\n", nodes[i], o.Err)
			summary.Failed = append(summary.Failed, fanout.Failed{Name: nodes[i], Err: o.Err})
			continue
		}
		_, _ = fmt.Fprintf(s.out, "%s: %s\n", nodes[i], o.Value)
	}
	return fanout.Failure("hosts", summary)
}

// The counter is drawn on the terminal, and taken off before anything else
// is written there: the command's output and its failures, the summary it
// leaves behind, and the error, which the command writes once the counter
// is gone.
func TestTheCounterIsTakenOffBeforeEveryWrite(t *testing.T) {
	t.Parallel()
	s := newSession(t, "exec", counterDisplay)
	err := uptime(s, false)
	s.checkFrames(t, `
run · 0/3 · 1 running · 2 queued · 0:01.0
`, `
run · 1/3 · 1 running · 1 queued · 0:02.0
`, `
run · 2/3 · 1 failed · 1 running · 0:03.0
`)
	checkEnd(t, s.end(t, err), `
exe0001: up 3 days
exe0002: exe0002: command exited 1
exe0003: up 3 days
prog: exec: failed in 3.0s: 2 ok, 1 failed
prog: 1 of 3 hosts failed: exe0002
`)
}

// A power-on in batches is counted as a whole, with the batch under way
// and the pause between two; the notes the command writes between the
// batches take the counter off, and it comes back below them. The test
// runs in a testing/synctest bubble, so that a frame is drawn during the
// pause.
func TestTheCounterOfAPowerOnInBatches(t *testing.T) {
	t.Parallel()
	synctest.Test(t, testTheCounterOfAPowerOnInBatches)
}

func testTheCounterOfAPowerOnInBatches(t *testing.T) {
	s := newSession(t, "bmc power", counterDisplay)
	o := duringPauses(fanout.BatchOptions{
		Step:  "power on",
		Size:  2,
		Limit: 1,
		Pause: 5 * time.Second,
		BeforePause: func(pause time.Duration) {
			_, _ = fmt.Fprintf(s.errOut, "waiting %s before the next batch\n", pause)
		},
		Before: func(i, n int, batch *nodeset.NodeSet) {
			_, _ = fmt.Fprintf(s.errOut, "powering on %s (%d of %d)\n", batch, i+1, n)
		},
	}, func(time.Duration) { s.draw() })
	batches := fanout.Batches(s.ctx, nodeset.MustParse("exe[1-4]"), o, func(ctx context.Context, batch *nodeset.NodeSet) error {
		outcomes, _ := fanout.Map(ctx, batch.Expand(), fanout.MapOptions[string]{Limit: 1},
			func(_ context.Context, node string) (struct{}, error) {
				s.draw()
				if node == "exe4" {
					return struct{}{}, errors.New("exe4: connection timeout")
				}
				return struct{}{}, nil
			})
		for _, o := range outcomes {
			if o.Err != nil {
				return o.Err
			}
		}
		return nil
	})
	err := batches[1].Err
	s.checkFrames(t, `
powering on exe[1-2] (1 of 2)
power on · batch 1/2 · 0/4 · 1 running · 3 queued · 0:01.0
`, `
powering on exe[1-2] (1 of 2)
power on · batch 1/2 · 1/4 · 1 running · 2 queued · 0:02.0
`, `
powering on exe[1-2] (1 of 2)
waiting 5s before the next batch
power on · batch 1/2 · 2/4 · 2 queued · waiting · 0:03.0
`, `
powering on exe[1-2] (1 of 2)
waiting 5s before the next batch
powering on exe[3-4] (2 of 2)
power on · batch 2/2 · 2/4 · 1 running · 1 queued · 0:04.0
`, `
powering on exe[1-2] (1 of 2)
waiting 5s before the next batch
powering on exe[3-4] (2 of 2)
power on · batch 2/2 · 3/4 · 1 running · 0:05.0
`)
	checkEnd(t, s.end(t, err), `
powering on exe[1-2] (1 of 2)
waiting 5s before the next batch
powering on exe[3-4] (2 of 2)
prog: bmc power: failed in 5.0s: 3 ok, 1 failed
prog: exe4: connection timeout
`)
}

// question asks for a confirmation the way a command does: with the
// displays off the terminal, it writes a preview and the question, and
// reads the answer, which the terminal echoes, ending the line.
func question(s *session) {
	resume := progress.Suspend(s.ctx)
	defer resume()
	_, _ = io.WriteString(s.errOut, "About to drain 1 host: exe0007\n  reason: \"failing DIMM\"\nContinue? [y/N] ")
	// A frame that falls due while the question waits draws nothing.
	s.draw()
	_, _ = io.WriteString(s.screen, "y\n")
}

// The confirmation is never drawn over: the counter leaves the terminal
// before the preview and the question, and comes back below them once the
// question has been answered.
func TestTheCounterLeavesTheQuestionAlone(t *testing.T) {
	t.Parallel()
	s := newSession(t, "slurm node drain", counterDisplay)
	_, call := progress.Start(s.ctx, progress.KindCall, "ssh login", progress.Timeout(10*time.Minute))
	s.draw()
	question(s)
	s.draw()
	call.End(nil)
	_, _ = io.WriteString(s.out, "drained exe0007\n")
	s.checkFrames(t, `
slurm node drain · 0:01.0
`, `
About to drain 1 host: exe0007
  reason: "failing DIMM"
Continue? [y/N] 
`, `
About to drain 1 host: exe0007
  reason: "failing DIMM"
Continue? [y/N] y
slurm node drain · 0:03.0
`)
	checkEnd(t, s.end(t, nil), `
About to drain 1 host: exe0007
  reason: "failing DIMM"
Continue? [y/N] y
drained exe0007
prog: slurm node drain: done in 3.0s
`)
}

// The live tree shows the command, the step with how its targets stand, and
// the one running with the request it waits for, against its bound. What
// the command writes lands above it, and once the command is over the tree
// is gone: the step's line, with the target that failed, the command's
// output, the summary and the error are what is left.
func TestTheTreeKeepsTheCommandsOutputAboveIt(t *testing.T) {
	t.Parallel()
	s := newSession(t, "exec", treeDisplay)
	err := uptime(s, true)
	s.checkFrames(t, `
exec · 0:01.0
  run  0/3 · 1 running · 2 queued
    ▸ exe0001  1.0s/10m  ssh
`, `
exec · 0:02.0
  run  1/3 · 1 running · 1 queued
    ▸ exe0002  1.0s/10m  ssh
    ✓ exe0001
`, `
exec · 0:03.0
  run  2/3 · 1 failed · 1 running
    ✗ exe0002  target: {}: command exited 1
    ▸ exe0003  1.0s/10m  ssh
    ✓ exe0001
`)
	checkEnd(t, s.end(t, err), `
✗ run  3.0s  2 ok, 1 failed
  ✗ exe0002  target: {}: command exited 1
exe0001: up 3 days
exe0002: exe0002: command exited 1
exe0003: up 3 days
prog: exec: failed in 3.0s: 2 ok, 1 failed
prog: 1 of 3 hosts failed: exe0002
`)
}

// The confirmation is never drawn over: the tree leaves the terminal before
// the preview and the question, and comes back below them once the
// question has been answered. A call made for the command itself has a row
// of its own.
func TestTheTreeLeavesTheQuestionAlone(t *testing.T) {
	t.Parallel()
	s := newSession(t, "slurm node drain", treeDisplay)
	_, call := progress.Start(s.ctx, progress.KindCall, "ssh login", progress.Timeout(10*time.Minute))
	s.draw()
	question(s)
	s.draw()
	call.End(nil)
	_, _ = io.WriteString(s.out, "drained exe0007\n")
	s.checkFrames(t, `
slurm node drain · 0:01.0
  ssh login  1.0s/10m
`, `
About to drain 1 host: exe0007
  reason: "failing DIMM"
Continue? [y/N] 
`, `
About to drain 1 host: exe0007
  reason: "failing DIMM"
Continue? [y/N] y
slurm node drain · 0:03.0
  ssh login  3.0s/10m
`)
	checkEnd(t, s.end(t, nil), `
About to drain 1 host: exe0007
  reason: "failing DIMM"
Continue? [y/N] y
drained exe0007
prog: slurm node drain: done in 3.0s
`)
	if strings.Contains(s.screen.String(), "·") {
		t.Error("the tree was left on the terminal")
	}
}
