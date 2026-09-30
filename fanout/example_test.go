// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
	"github.com/GSI-HPC/go-nodeset"
)

// Map works on every item, a bounded number at a time, and returns what
// each came to in the order of the items. A Capture shows what it
// reported: a Fold step with a target for each item.
func ExampleMap() {
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.Options{Sinks: []progress.Sink{capture}})
	ctx := progress.WithBus(context.Background(), bus)

	nodes := []string{"exe01", "exe02", "exe03", "exe04"}
	outcomes := fanout.Map(ctx, nodes, fanout.Options[string]{Step: "uptime", Limit: 2},
		func(_ context.Context, node string) (string, error) {
			if node == "exe03" {
				return "", fmt.Errorf("%s: no answer", node)
			}
			return strings.ToUpper(node), nil
		})
	for i, o := range outcomes {
		fmt.Println(nodes[i], o.Value, o.Err)
	}
	bus.Close()
	fmt.Print(capture.Tree())
	// Output:
	// exe01 EXE01 <nil>
	// exe02 EXE02 <nil>
	// exe03  exe03: no answer
	// exe04 EXE04 <nil>
	// step uptime total=4 limit=2 [fold]: failed (target): 1 of 4 failed: exe03
	//   target exe03: failed (target): {}: no answer
	//   target exe[01-02,04]: ok
}

// Each is the one bounded loop the pools run on: once ctx ends, no
// further call is started.
func ExampleEach() {
	var sum atomic.Int64
	fanout.Each(context.Background(), 100, 8, func(i int) { sum.Add(int64(i)) })
	fmt.Println(sum.Load())
	// Output:
	// 4950
}

// Batches runs a node set in batches, one after the other, with a pause
// between them; a batch that fails stops the rest.
func ExampleBatches() {
	nodes := nodeset.MustParse("exe[01-10]")
	batches := fanout.Batches(context.Background(), nodes, fanout.BatchOptions{
		Step:  "power on",
		Size:  4,
		Pause: time.Millisecond,
		Before: func(i, n int, batch *nodeset.NodeSet) {
			fmt.Printf("batch %d of %d: %s\n", i+1, n, batch)
		},
	}, func(_ context.Context, batch *nodeset.NodeSet) error {
		if batch.String() == "exe[05-07]" {
			return errors.New("exe06: no answer")
		}
		return nil
	})
	for _, b := range batches {
		fmt.Println(b.Nodes, b.Ran, b.Err)
	}
	// Output:
	// batch 1 of 3: exe[01-04]
	// batch 2 of 3: exe[05-07]
	// exe[01-04] true <nil>
	// exe[05-07] true exe06: no answer
	// exe[08-10] false not tried: an earlier batch failed
}

// Failure sums up the items that failed, as the error a step ends with.
func ExampleFailure() {
	err := fanout.Failure("hosts", 480, []string{"exe0007", "exe0008"},
		[]error{errors.New("exe0007: no answer"), context.Canceled}, false)
	fmt.Println(err)
	fmt.Println(errors.Is(err, context.Canceled), progress.Classify(err, nil))
	// Output:
	// 2 of 480 hosts failed: exe[0007-0008]
	// true target
}

// Skip leaves an item out on purpose: its target ends skipped, and it is
// not counted among those that failed.
func ExampleSkip() {
	outcomes := fanout.Map(context.Background(), []string{"exe01", "exe02"}, fanout.Options[string]{Step: "push"},
		func(_ context.Context, node string) (struct{}, error) {
			if node == "exe02" {
				return struct{}{}, fanout.Skip("nothing to push")
			}
			return struct{}{}, nil
		})
	for _, o := range outcomes {
		fmt.Println(o.Err, fanout.IsSkipped(o.Err))
	}
	// Output:
	// <nil> false
	// nothing to push true
}
