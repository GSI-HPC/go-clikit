// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-nodeset"
)

// duringPauses returns o with hooks that call during in each pause
// fanout.Batches waits out, as a display's ticker draws then. The test runs
// in a testing/synctest bubble: BeforePause starts a goroutine that waits
// until every other goroutine of the bubble is blocked, which Batches is
// once it waits for the pause's timer, and calls during, before the fake
// clock lets the pause pass. Before waits for the goroutine to be done,
// so that what during did happens before the next batch. The hooks o had
// are called as well, first.
func duringPauses(o fanout.BatchOptions, during func(pause time.Duration)) fanout.BatchOptions {
	var done chan struct{}
	beforePause, before := o.BeforePause, o.Before
	o.BeforePause = func(pause time.Duration) {
		if beforePause != nil {
			beforePause(pause)
		}
		done = make(chan struct{})
		go func() {
			defer close(done)
			synctest.Wait()
			during(pause)
		}()
	}
	o.Before = func(i, n int, batch *nodeset.NodeSet) {
		if done != nil {
			<-done
			done = nil
		}
		if before != nil {
			before(i, n, batch)
		}
	}
	return o
}
