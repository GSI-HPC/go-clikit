// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
)

// Once the context has ended, Each starts nothing, even when a free place
// and the end are ready at once and select takes the place: it asks the
// context again. Each round is a toss of select's, so the test tosses
// often enough that both sides come up.
func TestEachStartsNothingOnceTheContextHasEnded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 200 {
		fanout.Each(ctx, 1, 1, func(int) { t.Fatal("work started on an ended context") })
	}
}

// A limit below one is DefaultLimit, as it is for Map: each call waits a
// second of the fake clock of testing/synctest, which passes only once
// every call that can start has, so the most under way at once is the
// limit, exactly.
func TestEachTakesTheDefaultLimitBelowOne(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				var now, peak atomic.Int32
				fanout.Each(context.Background(), fanout.DefaultLimit+2, limit, func(int) {
					n := now.Add(1)
					for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); {
						p = peak.Load()
					}
					time.Sleep(time.Second)
					now.Add(-1)
				})
				if got := peak.Load(); got != fanout.DefaultLimit {
					t.Errorf("%d calls ran at once, want %d", got, fanout.DefaultLimit)
				}
			})
		})
	}
}
