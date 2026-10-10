// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
	"github.com/GSI-HPC/go-clikit/termtext"
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

// The work a span reports adds to what the counter draws and the lines it
// leaves, and takes nothing from them: a session whose calls advance their
// work draws the frames of one whose calls report none, with the work of
// the step after its counts, and the summary leaves what it moved.
func TestWorkAddsToTheFramesOfTheCounter(t *testing.T) {
	t.Parallel()
	var frames [2][]string
	var ends [2]string
	for i, advance := range []bool{false, true} {
		s := newSession(t, "fetch", counterDisplay)
		_, _ = fanout.Map(s.ctx, []string{"exe0001", "exe0002"}, fanout.MapOptions[string]{Step: "copy", Limit: 1},
			func(ctx context.Context, _ string) (struct{}, error) {
				_, call := progress.Start(ctx, progress.KindCall, "copy")
				if advance {
					call.Update(progress.Work(progress.Bytes, 2<<30))
					call.Advance(1 << 30)
				}
				s.draw()
				if advance {
					call.Advance(1 << 30)
				}
				s.draw()
				call.End(nil)
				return struct{}{}, nil
			})
		ends[i] = s.end(t, nil)
		frames[i] = s.frames
	}
	if want := []string{
		"copy · 0/2 · 1 running · 1 queued · 0:01.0\n",
		"copy · 0/2 · 1 running · 1 queued · 0:02.0\n",
		"copy · 1/2 · 1 running · 0:03.0\n",
		"copy · 1/2 · 1 running · 0:04.0\n",
	}; !slices.Equal(frames[0], want) {
		t.Errorf("without work:\n%s", strings.Join(frames[0], "\n"))
	}
	if want := []string{
		"copy · 0/2 · 1 running · 1 queued · 1.0 GiB · 25% · 0.0 B/s · 0:01.0\n",
		"copy · 0/2 · 1 running · 1 queued · 2.0 GiB · 50% · 512 MiB/s · ~4s left · 0:02.0\n",
		"copy · 1/2 · 1 running · 3.0 GiB · 75% · 683 MiB/s · ~2s left · 0:03.0\n",
		"copy · 1/2 · 1 running · 4.0 GiB · 99% · 768 MiB/s · ~1s left · 0:04.0\n",
	}; !slices.Equal(frames[1], want) {
		t.Errorf("with work:\n%s", strings.Join(frames[1], "\n"))
	}
	if want := "prog: fetch: done in 4.0s: 2 ok, 4.0 GiB at 1.0 GiB/s\n"; !strings.HasSuffix(ends[1], want) {
		t.Errorf("the summary is not the last line of\n%s", ends[1])
	}
	if want := "prog: fetch: done in 4.0s: 2 ok\n"; !strings.HasSuffix(ends[0], want) {
		t.Errorf("the summary is not the last line of\n%s", ends[0])
	}
}

// copyImage runs a pool of f's command that copies an image of 2 GiB to
// each of 12 nodes, four at a time, each target's copy a call that rolls up
// into it, and draws a frame: six copies have ended, one failed, four run,
// each at its own rate, and one is queued.
func copyImage(f *treeFixture) {
	copyImageOn(f.ctx, f.clock)
	f.draw(0)
}

// pool is the step copyImageOn runs and what runs in it.
type pool struct {
	step *progress.Span
	// targets and calls are the four copies under way, with the targets
	// they are made for, and queued is the target still waiting.
	targets, calls []*progress.Span
	queued         *progress.Span
}

// finish ends the copies under way a second after the clock c, well, each
// having moved another GiB, and then the target that waited, which ends at
// once, and the step.
func (p pool) finish(c *clock) {
	c.Add(time.Second)
	for i, call := range p.calls {
		call.Advance(1 << 30)
		call.End(nil)
		p.targets[i].End(nil)
	}
	p.queued.Run()
	p.queued.End(nil)
	p.step.End(nil)
}

// copyImageOn runs the pool of copyImage under ctx, on the clock c that the
// Bus reads, and returns it with the step still open: six copies have
// ended, one failed, four run, each at its own rate, and one is queued.
func copyImageOn(ctx context.Context, c *clock) pool {
	ctx, step := progress.Start(ctx, progress.KindStep, "copying the image",
		progress.WithFlags(progress.Fold), progress.Total(12), progress.Limit(4))
	ctxs, spans := targets(ctx, nodes(12)...)
	copying := func(i int) *progress.Span {
		spans[i].Run()
		_, call := progress.Start(ctxs[i], progress.KindCall, "copy", progress.Work(progress.Bytes, 2<<30))
		return call
	}
	for i := range 7 {
		call := copying(i)
		if i == 3 {
			c.Add(time.Second)
			call.Advance(1 << 29)
			err := unreachable("transport: dial tcp: i/o timeout")
			call.End(err)
			spans[i].End(err)
			continue
		}
		for range 4 {
			c.Add(time.Second)
			call.Advance(1 << 29)
		}
		call.End(nil)
		spans[i].End(nil)
	}
	var calls []*progress.Span
	for i := 7; i < 11; i++ {
		calls = append(calls, copying(i))
	}
	for range 10 {
		c.Add(time.Second)
		for j, call := range calls {
			call.Advance(int64(j+1) << 26)
		}
	}
	return pool{step: step, targets: spans[7:11], calls: calls, queued: spans[11]}
}

// The rows of the tree draw the work of their spans: the step's after how
// its targets stand, rolled up from its targets, and a target's after its
// name, before how long it has run. On a terminal too narrow for the whole
// row, the step's row gives up the time left, the rate and the amount, in
// that order, to keep its share; a target's row gives up its request
// first, as it does now, and then the same parts of its work.
func TestTheTreeDrawsTheWorkOfAPool(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		width int
		want  string
	}{
		{120, `
deploy · 0:35.0
  copying the image  7/12 · 1 failed · 4 running · 1 queued · 18.8 GiB · 82% · 640 MiB/s · ~9s left
    ✗ exe4  transport: transport: dial tcp: i/o timeout
    ▸ exe8  0.6/2.0 GiB · 31% · 64.0 MiB/s · ~22s left  10.0s  copy
    ▸ exe9  1.2/2.0 GiB · 62% · 128 MiB/s · ~6s left  10.0s  copy
    ▸ exe10  1.9/2.0 GiB · 93% · 192 MiB/s · ~1s left  10.0s  copy
    ▸ exe11  2.5/2.0 GiB · 100% · 256 MiB/s  10.0s  copy
    ✓ exe[1-3,5-7]
`},
		{80, `
deploy · 0:35.0
  copying the image  7/12 · 1 failed · 4 running · 1 queued · 18.8 GiB · 82%
    ✗ exe4  transport: transport: dial tcp: i/o timeout
    ▸ exe8  0.6/2.0 GiB · 31% · 64.0 MiB/s · ~22s left  10.0s  copy
    ▸ exe9  1.2/2.0 GiB · 62% · 128 MiB/s · ~6s left  10.0s  copy
    ▸ exe10  1.9/2.0 GiB · 93% · 192 MiB/s · ~1s left  10.0s  copy
    ▸ exe11  2.5/2.0 GiB · 100% · 256 MiB/s  10.0s  copy
    ✓ exe[1-3,5-7]
`},
		{60, `
deploy · 0:35.0
  copying the image  7/12 · 1 failed · 4 running · 1 queued
    ✗ exe4  transport: transport: dial tcp: i/o timeout
    ▸ exe8  0.6/2.0 GiB · 31% · 64.0 MiB/s  10.0s  copy
    ▸ exe9  1.2/2.0 GiB · 62% · 128 MiB/s · ~6s left  10.0s
    ▸ exe10  1.9/2.0 GiB · 93% · 192 MiB/s  10.0s  copy
    ▸ exe11  2.5/2.0 GiB · 100% · 256 MiB/s  10.0s  copy
    ✓ exe[1-3,5-7]
`},
	} {
		f := newTreeFixture(t, "deploy", treeSetup{width: tc.width})
		copyImage(f)
		if got := f.screen.String(); got != tc.want[1:] {
			t.Errorf("on %d columns:\n%s\nwant:\n%s", tc.width, got, tc.want[1:])
		}
	}
}

// workKinds returns which parts of a span's work row holds, as the tree
// draws them with no theme: the amount, the share, the rate and the time
// left. The last part of a row as wide as a terminal of width columns
// allows may have been cut by the terminal: whole leaves it out, and drawn
// counts it in.
func workKinds(row string, width int) (whole, drawn map[string]bool) {
	var parts []string
	for _, field := range strings.Split(row, "  ") {
		parts = append(parts, strings.Split(field, " · ")...)
	}
	whole, drawn = map[string]bool{}, map[string]bool{}
	for i, part := range parts {
		kind := ""
		switch {
		case strings.HasSuffix(part, " left"):
			kind = "left"
		case strings.HasSuffix(part, "/s"):
			kind = "rate"
		case strings.HasSuffix(part, "%"):
			kind = "share"
		case strings.HasSuffix(part, "iB"):
			kind = "amount"
		default:
			continue
		}
		drawn[kind] = true
		if i < len(parts)-1 || termtext.Width(row) < width-1 {
			whole[kind] = true
		}
	}
	return whole, drawn
}

// On every width, each row of the pool fits the terminal, gives up the
// parts of its work in order, the time left first, then the rate, then the
// amount, and keeps its share while it keeps anything else; and a wider
// terminal never draws less of a row's work than a narrower one.
func TestTheWorkOfTheTreeFitsEveryWidth(t *testing.T) {
	t.Parallel()
	chain := []string{"left", "rate", "amount", "share"}
	var before []map[string]bool
	for width := 40; width <= 120; width++ {
		f := newTreeFixture(t, "deploy", treeSetup{width: width})
		copyImage(f)
		rows := strings.Split(strings.TrimSuffix(f.screen.String(), "\n"), "\n")
		if len(rows) != 8 {
			t.Fatalf("on %d columns, %d rows, want 8:\n%s", width, len(rows), f.screen.String())
		}
		var kinds []map[string]bool
		for i, row := range rows {
			k, drawn := workKinds(row, width)
			kinds = append(kinds, k)
			for j := 0; j+1 < len(chain); j++ {
				if k[chain[j]] && !drawn[chain[j+1]] {
					t.Errorf("on %d columns, row %d keeps the %s but gives up the %s: %s", width, i+1, chain[j], chain[j+1], row)
				}
			}
			if before != nil && len(k) < len(before[i]) {
				t.Errorf("on %d columns, row %d draws less of its work than on %d: %s", width, i+1, width-1, row)
			}
		}
		before = kinds
	}
}
