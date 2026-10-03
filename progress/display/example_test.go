// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// exampleClock is a clock the example moves on by hand.
type exampleClock struct{ now time.Time }

// newExampleClock returns a clock at noon.
func newExampleClock() *exampleClock {
	return &exampleClock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
}

func (c *exampleClock) Now() time.Time      { return c.now }
func (c *exampleClock) Add(d time.Duration) { c.now = c.now.Add(d) }

// startTargets announces a target for each node, queued, as a pool does
// before it runs the first.
func startTargets(ctx context.Context, nodes ...string) []*progress.Span {
	targets := make([]*progress.Span, len(nodes))
	for i, node := range nodes {
		_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	return targets
}

// Plain writes a line for each thing worth one, for a CI log; the Summary
// is the line a command prints once it has ended. The noun names the
// targets, which are nodes here.
func ExampleNewPlain() {
	clock := newExampleClock()
	term := display.NewTerminal(os.Stdout, nil)
	plain := display.NewPlain(term, display.PlainOptions{
		Now: clock.Now,
		Noun: func(n int) string {
			if n == 1 {
				return "1 node"
			}
			return fmt.Sprintf("%d nodes", n)
		},
	})
	summary := &display.Summary{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{plain, summary}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "create cluster")

	ctx, step := progress.Start(ctx, progress.KindStep, "starting the nodes",
		progress.WithFlags(progress.Fold), progress.Total(3), progress.Limit(3))
	targets := startTargets(ctx, "worker0", "worker1", "worker2")
	for _, target := range targets {
		target.Run()
	}
	clock.Add(3 * time.Second)
	failed := errors.New("container exited with 1")
	targets[1].End(failed)
	clock.Add(7 * time.Second)
	plain.Draw()
	clock.Add(2 * time.Second)
	targets[0].End(nil)
	targets[2].End(nil)
	step.End(failed)
	command.End(failed)
	bus.Close()
	plain.Close()
	fmt.Println(summary.Line())
	// Output:
	// [0:00] create cluster › starting the nodes: start, 3 nodes
	// [0:03] create cluster › starting the nodes › worker1 failed (target): container exited with 1
	// [0:10] create cluster › starting the nodes: 1/3 done, 1 failed, 2 running
	// [0:12] create cluster › starting the nodes: failed in 12s: 2 ok, 1 failed
	// create cluster: failed in 12s: 2 ok, 1 failed
}

// The Tree draws the work under way at the bottom of a terminal, redrawn
// as it goes on; a Screen shows what a terminal would.
func ExampleNewTree() {
	clock := newExampleClock()
	screen := &progresstest.Screen{Width: 60}
	term := display.NewTerminal(screen, func() (int, int, error) { return 60, 24, nil })
	tree := display.NewTree(term, display.TreeOptions{Now: clock.Now})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{tree}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "power on")

	ctx, step := progress.Start(ctx, progress.KindStep, "powering on",
		progress.WithFlags(progress.Fold), progress.Total(4), progress.Limit(2))
	nodes := []string{"exe01", "exe02", "exe03", "exe04"}
	ctxs := make([]context.Context, len(nodes))
	targets := make([]*progress.Span, len(nodes))
	for i, node := range nodes {
		ctxs[i], targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	for _, target := range targets[:2] {
		target.Run()
		target.End(nil)
	}
	targets[2].Run()
	_, call := progress.Start(ctxs[2], progress.KindCall, "redfish",
		progress.HTTP("POST", "/redfish/v1/Systems/1"), progress.Timeout(30*time.Second))
	clock.Add(2 * time.Second)
	tree.Draw()
	fmt.Print(screen.String())
	fmt.Println("---")

	// Once the work is done, the region is gone and the line of the
	// finished step is left.
	call.End(nil)
	targets[2].End(nil)
	targets[3].Run()
	targets[3].End(nil)
	step.End(nil)
	command.End(nil)
	bus.Close()
	tree.Close()
	fmt.Print(screen.String())
	// Output:
	// power on · 0:02
	//   powering on  2/4 · 1 running · 1 queued
	//     ▸ exe03  2s/30s  POST /redfish/v1/Systems/1
	//     ✓ exe[01-02]
	// ---
	// ✓ powering on  2.0s  4 ok
}

// The Counter draws one line, for a terminal too small for the tree or a
// user who wants no more.
func ExampleNewCounter() {
	clock := newExampleClock()
	screen := &progresstest.Screen{}
	term := display.NewTerminal(screen, nil)
	counter := display.NewCounter(term, display.CounterOptions{Now: clock.Now})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{counter}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "uptime")

	ctx, step := progress.Start(ctx, progress.KindStep, "uptime",
		progress.WithFlags(progress.Fold), progress.Total(3), progress.Limit(8))
	targets := startTargets(ctx, "exe001", "exe002", "exe003")
	targets[0].Run()
	targets[0].End(nil)
	targets[1].Run()
	targets[1].End(errors.New("no answer"))
	targets[2].Run()
	clock.Add(41 * time.Second)
	counter.Draw()
	fmt.Println(screen.String())

	targets[2].End(nil)
	step.End(nil)
	command.End(nil)
	bus.Close()
	counter.Close()
	// Output:
	// uptime · 2/3 · 1 failed · 1 running · 0:41
}
