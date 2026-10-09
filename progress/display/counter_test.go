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
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
	"github.com/GSI-HPC/go-nodeset"
)

// screen is a terminal a test reads back.
type screen struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// String returns what was written, with each erasing of the line shown as
// "<erase>" and each line drawn after it on a line of its own.
func (s *screen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.ReplaceAll(s.b.String(), "\r\x1b[2K", "\n<erase>")
}

// clock is a clock a test moves by hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// startsOnce checks that a display, which newDisplay makes with the clock
// it is given and which draws every so often from Start, reads the clock
// once a frame: a second Start begins no second goroutine, which would
// draw twice as often and never stop, and a Start after Close begins
// none, which would draw on for good.
func startsOnce(t *testing.T, every time.Duration, newDisplay func(now func() time.Time) interface {
	Start()
	Close()
}) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		t.Helper()
		var reads atomic.Int64
		d := newDisplay(func() time.Time {
			reads.Add(1)
			return time.Now()
		})
		reads.Store(0)
		d.Start()
		d.Start()
		time.Sleep(time.Second + every/2)
		synctest.Wait()
		if got, want := reads.Load(), int64(time.Second/every); got != want {
			t.Errorf("started twice, the display drew %d frames in a second, want %d", got, want)
		}
		d.Close()
		reads.Store(0)
		d.Start()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if got := reads.Load(); got != 0 {
			t.Errorf("started after Close, the display drew %d frames, want none", got)
		}
	})
}

// fixture is a counter on a screen, fed by a Bus whose events are checked
// when the test ends, for a command that started when the counter was
// made.
type fixture struct {
	ctx     context.Context
	screen  *screen
	term    *display.Terminal
	counter *display.Counter
	clock   *clock
	command *progress.Span
}

func newFixture(t *testing.T, command string, size func() (int, int, error)) *fixture {
	t.Helper()
	f := &fixture{screen: &screen{}, clock: &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}}
	f.term = display.NewTerminal(f.screen, display.TerminalOptions{Size: size})
	f.counter = display.NewCounter(f.term, display.CounterOptions{Now: f.clock.Now})
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture, f.counter}, Now: f.clock.Now})
	t.Cleanup(func() {
		bus.Close()
		f.counter.Close()
		progresstest.Check(t, capture.Events())
	})
	f.ctx, f.command = progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, command)
	return f
}

// draw moves the clock on by d and draws a frame.
func (f *fixture) draw(d time.Duration) {
	f.clock.Add(d)
	f.counter.Draw()
}

// frames returns the lines the counter drew, in order.
func (f *fixture) frames() []string {
	var out []string
	for _, line := range strings.Split(f.screen.String(), "\n<erase>") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func check(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("frames:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A command done within a second never shows a counter.
func TestTheCounterWaitsASecond(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "node hw", nil)
	f.draw(0)
	f.draw(999 * time.Millisecond)
	if got := f.screen.String(); got != "" {
		t.Fatalf("the counter was drawn within its first second: %q", got)
	}
	f.draw(time.Millisecond)
	check(t, f.frames(), "node hw · 0:01.0")
}

// A counter given no clock reads the real one, and draws from its own
// ticker: nothing in its first second, and then a line at each tick, whose
// time, in tenths of a second, reads otherwise than the one before.
func TestTheCounterReadsTheRealClock(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		s := &screen{}
		counter := display.NewCounter(display.NewTerminal(s, display.TerminalOptions{}), display.CounterOptions{})
		counter.Start()
		time.Sleep(999 * time.Millisecond)
		synctest.Wait()
		if got := s.String(); got != "" {
			t.Fatalf("the counter was drawn within its first second: %q", got)
		}
		// Two ticks, at 1.0s and 1.1s, a line each.
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		counter.Close()
		if got, want := s.String(), "\n<erase>0:01.0\n<erase>0:01.1\n<erase>"; got != want {
			t.Errorf("screen:\n%q\nwant:\n%q", got, want)
		}
	})
}

// A line that reads as the one drawn before is not written again: one
// drawn at the same instant, or later within the same tenth of a second,
// which the time is cut to, writes nothing. The next tenth is drawn.
func TestTheCounterDoesNotDrawTheSameLineTwice(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "node hw", nil)
	f.draw(time.Second)
	written := f.screen.String()
	for _, again := range []struct {
		what  string
		after time.Duration
	}{
		{"at the same instant", 0},
		{"within the same tenth", 99 * time.Millisecond},
	} {
		f.draw(again.after)
		if got := f.screen.String(); got != written {
			t.Errorf("a line drawn again %s wrote %q", again.what, strings.TrimPrefix(got, written))
		}
	}
	f.draw(time.Millisecond)
	check(t, f.frames(), "node hw · 0:01.0", "node hw · 0:01.1")
}

// A fan-out is counted as its targets end, however they end, with those
// that fail, run and wait said as well.
func TestTheCounterCountsAFanOut(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "exec", nil)
	f.draw(time.Second)
	nodes := []string{"exe1", "exe2", "exe3", "exe4"}
	fanout.Map(f.ctx, nodes, fanout.MapOptions[string]{Step: "run", Limit: 1}, func(_ context.Context, node string) (struct{}, error) {
		f.draw(time.Second)
		if node == "exe2" {
			return struct{}{}, errors.New("exe2: command exited 1")
		}
		return struct{}{}, nil
	})
	f.draw(time.Second)
	check(t, f.frames(),
		"exec · 0:01.0",
		"run · 0/4 · 1 running · 3 queued · 0:02.0",
		"run · 1/4 · 1 running · 2 queued · 0:03.0",
		"run · 2/4 · 1 failed · 1 running · 1 queued · 0:04.0",
		"run · 3/4 · 1 failed · 1 running · 0:05.0",
		"exec · 0:06.0",
	)
}

// A wide fan-out reads the way the manual shows it.
func TestTheCounterOfAWideFanOut(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "provision reinstall", func() (int, int, error) { return 120, 40, nil })
	ctx, step := progress.Start(f.ctx, progress.KindStep, "reset the machines",
		progress.WithFlags(progress.Fold), progress.Total(480), progress.Limit(8))
	targets := make([]*progress.Span, 480)
	for i := range targets {
		node := fmt.Sprintf("exe%d", i+1)
		_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	end := func(i int) {
		var err error
		if i == 40 {
			err = errors.New("no answer")
		}
		targets[i].End(err)
	}
	// Eight at a time: each target that ends makes room for the next.
	for i := range 320 {
		if i >= 8 {
			end(i - 8)
		}
		targets[i].Run()
	}
	f.draw(4*time.Minute + 12*time.Second)
	for i := 312; i < 480; i++ {
		if i < 320 {
			end(i)
			continue
		}
		targets[i].Run()
		end(i)
	}
	step.End(errors.New("1 of 480 failed"))
	check(t, f.frames(), "reset the machines · 312/480 · 1 failed · 8 running · 160 queued · 4:12.0")
}

// A power-on in batches is counted as a whole, with the batch under way,
// the pause between two, and the batches left out after one failed. The
// test runs in a testing/synctest bubble, so that a frame is drawn during
// the pause.
func TestTheCounterCountsAPowerOnInBatches(t *testing.T) {
	t.Parallel()
	synctest.Test(t, testTheCounterCountsAPowerOnInBatches)
}

func testTheCounterCountsAPowerOnInBatches(t *testing.T) {
	f := newFixture(t, "bmc power on", nil)
	nodes, err := nodeset.Parse("exe[1-6]")
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Add(time.Second)
	o := duringPauses(fanout.BatchOptions{
		Step:  "power on",
		Size:  2,
		Pause: 30 * time.Second,
	}, func(d time.Duration) {
		f.draw(time.Second)
		f.clock.Add(d)
	})
	fanout.Batches(f.ctx, nodes, o, func(ctx context.Context, batch *nodeset.NodeSet) error {
		f.draw(time.Second)
		names := batch.Expand()
		spans := make([]*progress.Span, len(names))
		for i, node := range names {
			_, spans[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
		}
		var failed error
		for i, node := range names {
			spans[i].Run()
			f.draw(time.Second)
			if node == "exe4" {
				failed = errors.New("exe4: no answer")
			}
			spans[i].End(failed)
		}
		return failed
	})
	f.draw(time.Second)
	check(t, f.frames(),
		"power on · batch 1/3 · 0/6 · 4 queued · 0:02.0",
		"power on · batch 1/3 · 0/6 · 1 running · 5 queued · 0:03.0",
		"power on · batch 1/3 · 1/6 · 1 running · 4 queued · 0:04.0",
		"power on · batch 1/3 · 2/6 · 4 queued · waiting · 0:05.0",
		"power on · batch 2/3 · 2/6 · 2 queued · 0:36.0",
		"power on · batch 2/3 · 2/6 · 1 running · 3 queued · 0:37.0",
		"power on · batch 2/3 · 3/6 · 1 running · 2 queued · 0:38.0",
		"bmc power on · 0:39.0",
	)
}

// Two steps under way side by side each have a segment, and the one that
// ends first leaves the line.
func TestTheCounterShowsTwoStepsSideBySide(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "provision status", func() (int, int, error) { return 160, 40, nil })
	start := func(name string, n int) (*progress.Span, []*progress.Span) {
		ctx, step := progress.Start(f.ctx, progress.KindStep, name, progress.WithFlags(progress.Fold), progress.Total(n))
		targets := make([]*progress.Span, n)
		for i := range targets {
			node := fmt.Sprintf("exe%d", i+1)
			_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
		}
		return step, targets
	}
	power, bmcs := start("read the power state", 3)
	ssh, nodes := start("run", 3)
	bmcs[0].Run()
	nodes[0].Run()
	nodes[1].Run()
	f.draw(2 * time.Second)
	bmcs[0].End(nil)
	bmcs[1].Run()
	nodes[0].End(errors.New("exe1: connection refused"))
	f.draw(time.Second)
	for _, bmc := range bmcs[1:] {
		bmc.Run()
		bmc.End(nil)
	}
	power.End(nil)
	f.draw(time.Second)
	for _, node := range nodes[1:] {
		node.Run()
		node.End(nil)
	}
	ssh.End(nil)
	check(t, f.frames(),
		"read the power state · 0/3 · 1 running · 2 queued | run · 0/3 · 2 running · 1 queued · 0:02.0",
		"read the power state · 1/3 · 1 running · 1 queued | run · 1/3 · 1 failed · 1 running · 1 queued · 0:03.0",
		"run · 1/3 · 1 failed · 1 running · 1 queued · 0:04.0",
	)
}

// A step that counts nothing is named while it runs, unless it is hidden.
func TestTheCounterNamesTheStepUnderWay(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "provision reinstall", nil)
	_, hidden := progress.Start(f.ctx, progress.KindStep, "check the boot paths", progress.WithFlags(progress.Hidden))
	f.draw(time.Second)
	hidden.End(nil)
	_, step := progress.Start(f.ctx, progress.KindStep, "configuring the network boot")
	f.draw(time.Second)
	step.End(nil)
	check(t, f.frames(), "provision reinstall · 0:01.0", "configuring the network boot · 0:02.0")
}

// A step with no name, as a pool given none reports its targets under, is
// named by the nearest step above it that has a name, through a call, or
// by the command.
func TestTheCounterNamesAStepWithNoNameByTheSpanAboveIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "exec", nil)
	f.draw(time.Second)
	fanout.Map(f.ctx, []string{"exe1", "exe2"}, fanout.MapOptions[string]{Limit: 1},
		func(context.Context, string) (struct{}, error) {
			f.draw(time.Second)
			return struct{}{}, nil
		})
	disarm, disarming := progress.Start(f.ctx, progress.KindStep, "disarming")
	ssh, call := progress.Start(disarm, progress.KindCall, "ssh")
	unnamed, step := progress.Start(ssh, progress.KindStep, "")
	f.draw(time.Second)
	fanout.Map(unnamed, []string{"exe1"}, fanout.MapOptions[string]{},
		func(context.Context, string) (struct{}, error) {
			f.draw(time.Second)
			return struct{}{}, nil
		})
	step.End(nil)
	call.End(nil)
	disarming.End(nil)
	check(t, f.frames(),
		"exec · 0:01.0",
		"exec · 0/2 · 1 running · 1 queued · 0:02.0",
		"exec · 1/2 · 1 running · 0:03.0",
		"disarming · 0:04.0",
		"disarming · 0/1 · 1 running · 0:05.0",
	)
}

// A step with no name that no span above names says only how far it has
// got.
func TestTheCounterOfAStepWithNothingToNameIt(t *testing.T) {
	t.Parallel()
	s := &screen{}
	c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
	counter := display.NewCounter(display.NewTerminal(s, display.TerminalOptions{}), display.CounterOptions{Now: c.Now})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{counter}, Now: c.Now})
	fanout.Map(progress.WithBus(context.Background(), bus), []string{"exe1"}, fanout.MapOptions[string]{},
		func(context.Context, string) (struct{}, error) {
			c.Add(time.Second)
			counter.Draw()
			return struct{}{}, nil
		})
	bus.Close()
	counter.Close()
	if got, want := s.String(), "\n<erase>0/1 · 1 running · 0:01.0\n<erase>"; got != want {
		t.Errorf("screen %q, want %q", got, want)
	}
}

// Whatever the command writes takes the line off first, and it is drawn
// again only once what was written ended a line: a question waiting for its
// answer is never drawn over.
func TestWritesTakeTheCounterOff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "bmc power cycle", nil)
	errOut := f.term.Writer(f.screen)
	f.draw(time.Second)
	_, _ = io.WriteString(errOut, "waiting 30s before the next batch\n")
	f.draw(time.Second)
	_, _ = io.WriteString(errOut, "About to power cycle 2 hosts\n")
	_, _ = io.WriteString(errOut, "Continue? [y/N] ")
	f.draw(time.Second)
	f.draw(time.Second)
	_, _ = io.WriteString(errOut, "y\n")
	f.draw(time.Second)
	want := `
<erase>bmc power cycle · 0:01.0
<erase>waiting 30s before the next batch

<erase>bmc power cycle · 0:02.0
<erase>About to power cycle 2 hosts
Continue? [y/N] y

<erase>bmc power cycle · 0:05.0`
	if got := f.screen.String(); got != want {
		t.Errorf("screen:\n%s\nwant:\n%s", got, want)
	}
}

// Suspend takes the line off before it returns, and the line stays off,
// whatever is written meanwhile, until the question has been answered.
func TestSuspendTakesTheCounterOffUntilResumed(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "bmc status", nil)
	errOut := f.term.Writer(f.screen)
	f.draw(time.Second)
	resume := progress.Suspend(f.ctx)
	if got := f.screen.String(); !strings.HasSuffix(got, "0:01.0\n<erase>") {
		t.Fatalf("Suspend returned with the line still drawn: %q", got)
	}
	_, _ = io.WriteString(errOut, "Password for admin@bmc: ")
	f.draw(time.Second)
	_, _ = io.WriteString(errOut, "\n")
	f.draw(time.Second)
	resume()
	f.draw(time.Second)
	want := `
<erase>bmc status · 0:01.0
<erase>Password for admin@bmc: 

<erase>bmc status · 0:04.0`
	if got := f.screen.String(); got != want {
		t.Errorf("screen:\n%s\nwant:\n%s", got, want)
	}
}

// The line never wraps, which would leave a row behind each time it is
// drawn: it is cut to the width of the terminal, or to 80 columns when the
// terminal does not say.
func TestTheLineFitsTheTerminal(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 100)
	for _, tc := range []struct {
		name, command string
		size          func() (int, int, error)
		want          string
	}{
		{"a narrow terminal", "provision reinstall", func() (int, int, error) { return 21, 24, nil }, "provision reinstall "},
		{"a terminal that does not say", long, func() (int, int, error) { return 0, 0, errors.New("not a terminal") }, long[:79]},
		{"no size at all", long, nil, long[:79]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.command, tc.size)
			f.draw(time.Second)
			check(t, f.frames(), tc.want)
		})
	}
}

// Close takes the line off for good, stops the drawing Start began, and
// leaves the writers working.
func TestCloseTakesTheCounterOff(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "exec", nil)
	errOut := f.term.Writer(f.screen)
	f.counter.Start()
	f.draw(time.Second)
	f.counter.Close()
	f.counter.Close()
	f.draw(time.Second)
	_, _ = io.WriteString(errOut, "prog: interrupted\n")
	if got, want := f.screen.String(), "\n<erase>exec · 0:01.0\n<erase>prog: interrupted\n"; got != want {
		t.Errorf("screen:\n%q\nwant:\n%q", got, want)
	}
}

// A second Start, and a Start after Close, do nothing.
func TestTheCounterStartsOnce(t *testing.T) {
	t.Parallel()
	startsOnce(t, 100*time.Millisecond, func(now func() time.Time) interface {
		Start()
		Close()
	} {
		return display.NewCounter(display.NewTerminal(&screen{}, display.TerminalOptions{}), display.CounterOptions{Now: now})
	})
}

// A display that panics while it draws, on its own goroutine, draws no
// more and says why with the stack, but the process goes on, and Close
// returns, as a Bus goes on without a sink that panics.
func TestADisplayThatPanicsWhileItDrawsStops(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"counter", "tree"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var log screen
			o := display.TerminalOptions{
				Size:     func() (int, int, error) { panic("the size of the terminal") },
				PanicLog: &log,
			}
			prefix := ""
			if name == "tree" {
				o.Program, prefix = "sind", "sind: "
			}
			term := display.NewTerminal(&screen{}, o)
			c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
			var d interface {
				Start()
				Close()
			}
			if name == "tree" {
				d = display.NewTree(term, display.TreeOptions{Now: c.Now})
			} else {
				d = display.NewCounter(term, display.CounterOptions{Now: c.Now})
			}
			c.Add(2 * time.Second)
			d.Start()
			deadline := time.Now().Add(5 * time.Second)
			for !strings.Contains(log.String(), "the progress display stopped: the size of the terminal") {
				if time.Now().After(deadline) {
					t.Fatalf("no word of the panic within 5s: %q", log.String())
				}
				time.Sleep(10 * time.Millisecond)
			}
			d.Close()
			if !strings.HasPrefix(log.String(), prefix+"the progress display stopped") {
				t.Errorf("the panic is not written as the program's: %q", log.String())
			}
			if !strings.Contains(log.String(), "goroutine") {
				t.Errorf("the stack is not written: %q", log.String())
			}
		})
	}
}

// A job in the background of its terminal draws nothing, since the shell's
// prompt is on the row a frame would erase, and draws again once it is in
// the foreground, below whatever the shell wrote meanwhile.
func TestNothingIsDrawnInTheBackground(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	inFront := false
	s := &screen{}
	term := display.NewTerminal(s, display.TerminalOptions{Foreground: func() bool {
		mu.Lock()
		defer mu.Unlock()
		return inFront
	}})
	c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
	counter := display.NewCounter(term, display.CounterOptions{Now: c.Now})
	c.Add(2 * time.Second)
	counter.Draw()
	if got := s.String(); got != "" {
		t.Errorf("drawn in the background: %q", got)
	}
	mu.Lock()
	inFront = true
	mu.Unlock()
	counter.Draw()
	if got := s.String(); got == "" {
		t.Error("nothing drawn in the foreground")
	}
	counter.Close()
}

// transportError is the error of a host that could not be reached, which
// says its class itself, as the errors of a program's transport do.
type transportError struct{ msg string }

func (e transportError) Error() string               { return e.msg }
func (transportError) ProgressClass() progress.Class { return progress.ClassTransport }

// unreachable returns a transportError that says what format and args do.
func unreachable(format string, args ...any) error {
	return transportError{fmt.Sprintf(format, args...)}
}
