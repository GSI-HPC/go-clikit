// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// The tree handles every event and draws every frame under its lock, which
// the Bus waits for, and every worker that reports an event waits for the
// Bus, so what the tree spends on a large step slows the work itself. Each
// part runs the same work at two sizes, takes the best of five runs of
// each, logs how many times as long the larger took, and fails when that
// is several times what the sizes alone explain: work quadratic in the
// targets misses by an order of magnitude. The race detector slows some
// code more than other code, so the test skips under it; CI runs it alone,
// without the race detector, as make costs does.
func TestTheCostOfALargeStep(t *testing.T) {
	if raceDetector {
		t.Skip("the costs are measured without the race detector")
	}
	t.Run("ending the targets costs what they number", testEndingTheTargetsOfAStepCostsWhatTheyNumber)
	t.Run("frames do not fold a large set each time", testFramesDoNotFoldALargeSetEachTime)
	t.Run("failures of their own cost what they number", testFailuresOfTheirOwnCostWhatTheyNumber)
	t.Run("batches that end cost what their targets number", testBatchesThatEndCostWhatTheirTargetsNumber)
	t.Run("a frame draws the running targets that fit", testAFrameDrawsTheRunningTargetsThatFit)
}

// within logs how many times as long large took as small, which go test
// shows with -v, and fails when that is more than limit; what says what
// the two measured and how much more work the larger is.
func within(t *testing.T, what string, small, large time.Duration, limit float64) {
	t.Helper()
	times := float64(large) / float64(max(small, 1))
	report := fmt.Sprintf("%s: %.1f times as long, %v against %v; more than %g times fails",
		what, times, large.Round(time.Microsecond), small.Round(time.Microsecond), limit)
	if times > limit {
		t.Error(report)
		return
	}
	t.Log(report)
}

// fanOutCost is the best of five runs of a step over n targets, each
// started queued, run and ended in turn, with a frame drawn every
// frameEvery targets, through a tree on a terminal of 100 by 30.
func fanOutCost(t *testing.T, n, frameEvery int, end func(i int) error) time.Duration {
	t.Helper()
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("exe%05d", i+1)
	}
	best := time.Duration(1 << 62)
	for range 5 {
		screen := &progresstest.Screen{Width: 100}
		term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return 100, 30, nil }})
		c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
		tree := display.NewTree(term, display.TreeOptions{Now: c.Now})
		bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{tree}, Now: c.Now})
		runtime.GC()
		start := time.Now()
		ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "exec")
		ctx, step := progress.Start(ctx, progress.KindStep, "run", progress.WithFlags(progress.Fold),
			progress.Total(n), progress.Limit(n))
		spans := make([]*progress.Span, n)
		for i, name := range names {
			_, spans[i] = progress.Start(ctx, progress.KindTarget, name, progress.Queued(), progress.Node(name))
		}
		for i, span := range spans {
			span.Run()
			if frameEvery > 0 && i%frameEvery == 0 {
				c.Add(100 * time.Millisecond)
				tree.Draw()
			}
		}
		for i, span := range spans {
			span.End(end(i))
			if frameEvery > 0 && i%frameEvery == 0 {
				c.Add(100 * time.Millisecond)
				tree.Draw()
			}
		}
		step.End(nil)
		command.End(nil)
		best = min(best, time.Since(start))
		bus.Close()
		tree.Close()
	}
	return best
}

// The tree took each target that ended out of the list of its step's
// targets with a pass over the others: a step of 20,000 targets took more
// than half a second over its events, with the Bus waiting, where 2,000
// took 17ms. Thirty times as many targets have to cost about thirty times
// as much.
func testEndingTheTargetsOfAStepCostsWhatTheyNumber(t *testing.T) {
	ok := func(int) error { return nil }
	small, large := fanOutCost(t, 1000, 0, ok), fanOutCost(t, 30000, 0, ok)
	within(t, "ending 30,000 targets against 1,000, 30 times the work", small, large, 90)
}

// Each frame folded the targets that ended well again whenever one more
// had ended, ten times a second, under the lock the Bus waits for: 3ms a
// frame at 10,000 targets. A large set is folded at most once a second, so
// the frames of the second after it was cost about the same over 30,000
// targets as over 300.
func testFramesDoNotFoldALargeSetEachTime(t *testing.T) {
	cost := func(n int) time.Duration {
		best := time.Duration(1 << 62)
		for range 5 {
			screen := &progresstest.Screen{Width: 100}
			term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return 100, 30, nil }})
			c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
			tree := display.NewTree(term, display.TreeOptions{Now: c.Now})
			bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{tree}, Now: c.Now})
			ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "exec")
			ctx, step := progress.Start(ctx, progress.KindStep, "run", progress.WithFlags(progress.Fold),
				progress.Total(n+9), progress.Limit(n+9))
			spans := make([]*progress.Span, n+9)
			for i := range spans {
				name := fmt.Sprintf("exe%05d", i+1)
				_, spans[i] = progress.Start(ctx, progress.KindTarget, name, progress.Queued(), progress.Node(name))
			}
			for _, span := range spans[:n] {
				span.Run()
				span.End(nil)
			}
			c.Add(time.Second)
			tree.Draw()
			runtime.GC()
			start := time.Now()
			for _, span := range spans[n:] {
				span.Run()
				span.End(nil)
				c.Add(100 * time.Millisecond)
				tree.Draw()
			}
			best = min(best, time.Since(start))
			step.End(nil)
			command.End(nil)
			bus.Close()
			tree.Close()
		}
		return best
	}
	small, large := cost(300), cost(30000)
	within(t, "frames over 30,000 targets that ended against over 300, the same work", small, large, 10)
}

// The tree found the row of a failure by comparing its text with that of
// every row before it, so a fan-out whose targets each failed their own way
// cost time quadratic in them. Thirty times as many such failures have to
// cost about thirty times as much.
func testFailuresOfTheirOwnCostWhatTheyNumber(t *testing.T) {
	own := func(i int) error { return fmt.Errorf("checksum %d does not match", i) }
	small, large := fanOutCost(t, 1000, 0, own), fanOutCost(t, 30000, 0, own)
	within(t, "30,000 failures of their own against 1,000, 30 times the work", small, large, 90)
}

// Each batch that ended folded its targets into its step's with a union,
// which copies the step's set: a power on of 30,000 nodes in batches of a
// hundred copied it 300 times. Thirty times as many nodes, in batches of
// the same size, have to cost about thirty times as much.
func testBatchesThatEndCostWhatTheirTargetsNumber(t *testing.T) {
	const size = 100
	cost := func(n int) time.Duration {
		best := time.Duration(1 << 62)
		for range 5 {
			screen := &progresstest.Screen{Width: 100}
			term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return 100, 30, nil }})
			c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
			tree := display.NewTree(term, display.TreeOptions{Now: c.Now})
			bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{tree}, Now: c.Now})
			runtime.GC()
			start := time.Now()
			ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "bmc power on")
			ctx, step := progress.Start(ctx, progress.KindStep, "power on", progress.WithFlags(progress.Fold), progress.Total(n))
			for b := 0; b < n/size; b++ {
				batchCtx, batch := progress.Start(ctx, progress.KindBatch, fmt.Sprintf("%d/%d", b+1, n/size),
					progress.Total(size))
				spans := make([]*progress.Span, size)
				for i := range spans {
					name := fmt.Sprintf("exe%05d", b*size+i+1)
					_, spans[i] = progress.Start(batchCtx, progress.KindTarget, name, progress.Queued(), progress.Node(name))
				}
				for _, span := range spans {
					span.Run()
					span.End(nil)
				}
				batch.End(nil)
			}
			step.End(nil)
			command.End(nil)
			best = min(best, time.Since(start))
			bus.Close()
			tree.Close()
		}
		return best
	}
	small, large := cost(1000), cost(30000)
	within(t, "30,000 targets in batches against 1,000, 30 times the work", small, large, 90)
}

// A frame sorted every running target and drew a row for each, of which
// it kept about a third of the terminal's rows: 2.9ms a frame with 10,000
// targets running, as an IPMI step marks every node, over thirty times
// what a frame took over as many queued, which are only counted. It draws
// the rows that fit, and picks out only the targets that get them.
func testAFrameDrawsTheRunningTargetsThatFit(t *testing.T) {
	const n = 30000
	cost := func(run bool) time.Duration {
		screen := &progresstest.Screen{Width: 100}
		term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return 100, 30, nil }})
		c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
		tree := display.NewTree(term, display.TreeOptions{Now: c.Now})
		bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{tree}, Now: c.Now})
		defer tree.Close()
		defer bus.Close()
		ctx, _ := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "bmc power status")
		ctx, _ = progress.Start(ctx, progress.KindStep, "ipmipower", progress.WithFlags(progress.Fold),
			progress.Total(n), progress.Limit(n))
		for i := range n {
			name := fmt.Sprintf("exe%05d", i+1)
			_, span := progress.Start(ctx, progress.KindTarget, name, progress.Queued(), progress.Node(name))
			if run {
				// A few seconds apart, as a pool starts them.
				if i%1000 == 0 {
					c.Add(time.Second)
				}
				span.Run()
			}
		}
		c.Add(time.Second)
		best := time.Duration(1 << 62)
		for range 5 {
			runtime.GC()
			start := time.Now()
			for range 5 {
				c.Add(100 * time.Millisecond)
				tree.Draw()
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	queued, running := cost(false), cost(true)
	within(t, "frames over 30,000 targets running against as many queued, a few rows more", queued, running, 15)
}
