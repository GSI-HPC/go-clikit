// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package mcpprogress_test

import (
	"context"
	"fmt"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/mcpprogress"
)

// A server runs a call that came with a progress token under a Bus of its
// own, and closes the Notifier after the Bus, before it answers the call.
// Send would put the token in the notification and send it on the call's
// session; here it keeps the last.
func Example() {
	var last string
	n := mcpprogress.New(mcpprogress.Options{
		Program: "prog",
		Send: func(progress, total float64, message string) error {
			last = fmt.Sprintf("%v/%v %s", progress, total, message)
			return nil
		},
	})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{n}, Program: "prog"})
	ctx := progress.WithBus(context.Background(), bus)

	fanout.Map(ctx, []string{"exe1", "exe2", "exe3"}, fanout.MapOptions[string]{Step: "probe the nodes"},
		func(context.Context, string) (struct{}, error) { return struct{}{}, nil })

	bus.Close()
	n.Close()
	fmt.Println(last)
	// Output:
	// 3/3 probe the nodes: 3/3 done
}
