// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
	"github.com/GSI-HPC/go-nodeset"
)

// Map works on every item, a bounded number at a time, and returns what
// each came to in the order of the items, and the error its step ended
// with. A Capture shows what it reported: a Fold step with a target for
// each item.
func ExampleMap() {
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture}})
	ctx := progress.WithBus(context.Background(), bus)

	nodes := []string{"exe01", "exe02", "exe03", "exe04"}
	outcomes, err := fanout.Map(ctx, nodes, fanout.MapOptions[string]{Step: "uptime", Limit: 2},
		func(_ context.Context, node string) (string, error) {
			if node == "exe03" {
				return "", fmt.Errorf("%s: no answer", node)
			}
			return strings.ToUpper(node), nil
		})
	for i, o := range outcomes {
		fmt.Println(nodes[i], o.Value, o.Err)
	}
	fmt.Println(err)
	bus.Close()
	fmt.Print(capture.Tree())
	// Output:
	// exe01 EXE01 <nil>
	// exe02 EXE02 <nil>
	// exe03  exe03: no answer
	// exe04 EXE04 <nil>
	// 1 of 4 failed: exe03
	// step uptime total=4 limit=2 [fold]: failed (target): 1 of 4 failed: exe03
	//   target exe03: failed (target): {}: no answer
	//   target exe[01-02,04]: ok
}

// Describe says what a display names an item by: the node its target is
// named after, the host the work goes to and the item's role. An Item
// without a Node names the item as fmt.Sprint prints it.
func ExampleMapOptions_describe() {
	type machine struct {
		name, role string
	}
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture}})
	ctx := progress.WithBus(context.Background(), bus)

	machines := []machine{{"exe01", "compute"}, {"mds01", "storage"}}
	fanout.Map(ctx, machines, fanout.MapOptions[machine]{
		Step: "reset",
		Describe: func(m machine) fanout.Item {
			return fanout.Item{Node: m.name, Host: m.name + "-bmc", Role: m.role}
		},
	}, func(context.Context, machine) (struct{}, error) { return struct{}{}, nil })
	bus.Close()
	for _, ev := range capture.Events() {
		if ev.Kind == progress.KindTarget && ev.Type == progress.TypeStart {
			fmt.Println(ev.Name, ev.Host, ev.Role)
		}
	}
	// Output:
	// exe01 exe01-bmc compute
	// mds01 mds01-bmc storage
}

// An Item with a Unit and a Size starts its target bounded, so that a queued
// target shows its size before it runs. Each worker counts the bytes it
// copies through progress.CountWriter, and the targets' amounts reach the
// Capture's tree as they copy.
func ExampleMapOptions_work() {
	type image struct {
		node string
		size int64
	}
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture}})
	ctx := progress.WithBus(context.Background(), bus)

	images := []image{{"exe01", 4 << 20}, {"exe02", 2 << 20}, {"exe03", 1 << 20}}
	outcomes, err := fanout.Map(ctx, images, fanout.MapOptions[image]{
		Step:  "copy",
		Limit: 2,
		Describe: func(im image) fanout.Item {
			return fanout.Item{Node: im.node, Unit: progress.Bytes, Size: im.size}
		},
	}, func(ctx context.Context, im image) (int64, error) {
		return io.Copy(progress.CountWriter(ctx, io.Discard), bytes.NewReader(make([]byte, im.size)))
	})
	for i, o := range outcomes {
		fmt.Println(images[i].node, o.Value, o.Err)
	}
	fmt.Println(err)
	bus.Close()
	fmt.Print(capture.Tree())
	// Output:
	// exe01 4194304 <nil>
	// exe02 2097152 <nil>
	// exe03 1048576 <nil>
	// <nil>
	// step copy total=3 limit=2 [fold]: ok
	//   target exe01 amount=4194304 size=4194304 unit=bytes: ok
	//   target exe02 amount=2097152 size=2097152 unit=bytes: ok
	//   target exe03 amount=1048576 size=1048576 unit=bytes: ok
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
		fmt.Println(b.Nodes, b.Started, b.Err)
	}
	// Output:
	// batch 1 of 3: exe[01-04]
	// batch 2 of 3: exe[05-07]
	// exe[01-04] true <nil>
	// exe[05-07] true exe06: no answer
	// exe[08-10] false not tried: an earlier batch failed
}

// Failure sums up the items that failed, as the error a step ends with. A
// program's MapOptions.Summarize builds on it, to give the error an exit
// code of its own.
func ExampleFailure() {
	err := fanout.Failure("hosts", fanout.Summary{
		Total: 480,
		Failed: []fanout.Failed{
			{Name: "exe0007", Err: errors.New("exe0007: no answer")},
			{Name: "exe0008", Err: context.Canceled},
		},
	})
	fmt.Println(err)
	fmt.Println(errors.Is(err, context.Canceled), progress.Classify(err, nil))
	// Output:
	// 2 of 480 hosts failed: exe[0007-0008]
	// true target
}

// A skip, an error of progress.Skip, leaves an item out on purpose: its
// target ends skipped, and it is not counted among those that failed.
func ExampleMap_skip() {
	outcomes, _ := fanout.Map(context.Background(), []string{"exe01", "exe02"}, fanout.MapOptions[string]{Step: "push"},
		func(_ context.Context, node string) (struct{}, error) {
			if node == "exe02" {
				return struct{}{}, progress.Skip("nothing to push")
			}
			return struct{}{}, nil
		})
	for _, o := range outcomes {
		fmt.Println(o.Err, errors.Is(o.Err, progress.ErrSkipped))
	}
	// Output:
	// <nil> false
	// nothing to push true
}
