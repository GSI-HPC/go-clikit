// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progresstest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// reset is the library code under test: it reports a step with a target
// for each node.
func reset(ctx context.Context, nodes []string) error {
	ctx, step := progress.Start(ctx, progress.KindStep, "reset the machines",
		progress.WithFlags(progress.Fold), progress.Total(len(nodes)))
	targets := make([]*progress.Span, len(nodes))
	for i, node := range nodes {
		_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	var err error
	for i, target := range targets {
		target.Run()
		if nodes[i] == "exe3" {
			err = errors.New("exe3: no answer")
			target.End(err)
			continue
		}
		target.End(nil)
	}
	step.End(err)
	return err
}

// A test watches the work it runs: Watch checks every promise the events
// make, and the tree they draw reads the same however the work was
// scheduled.
func ExampleWatch() {
	_ = func(t *testing.T) {
		ctx, tree := progresstest.Watch(t.Context(), t)
		_ = reset(ctx, []string{"exe1", "exe2", "exe3"})
		want := `step reset the machines total=3 [fold]: failed (target): exe3: no answer
  target exe3: failed (target): {}: no answer
  target exe[1-2]: ok
`
		if got := tree(); got != want {
			t.Errorf("the work reported\n%s\nwant\n%s", got, want)
		}
	}
}

// A Capture keeps the events of a Bus; its Tree draws them, targets that
// read the same folded into one line.
func ExampleCapture() {
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture}})
	_ = reset(progress.WithBus(context.Background(), bus), []string{"exe1", "exe2", "exe3", "exe10"})
	bus.Close()
	fmt.Print(capture.Tree())
	// Output:
	// step reset the machines total=4 [fold]: failed (target): exe3: no answer
	//   target exe3: failed (target): {}: no answer
	//   target exe[1-2,10]: ok
}

// A Screen shows what a display wrote the way a terminal would: here a
// line drawn, erased and drawn again.
func ExampleScreen() {
	screen := &progresstest.Screen{}
	fmt.Fprint(screen, "uptime · 1/3 · 0:01")
	fmt.Fprint(screen, "\r\x1b[2Kuptime · 3/3 · 0:02\n")
	fmt.Fprint(screen, "\x1b[?25l")
	fmt.Println(screen.String())
	// Output:
	// uptime · 3/3 · 0:02
	// ^[[?25l
}
