// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanouttest_test

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout/fanouttest"
)

// pool runs calls calls, width of them at once, the way a fan-out bounded
// at width does.
func pool(width, calls int, call func()) {
	sem := make(chan struct{}, width)
	var wg sync.WaitGroup
	for range calls {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			call()
		})
	}
	wg.Wait()
}

// Holding calls until one more than the limit are under way is what makes a
// count worth something: a pool that keeps to its limit is seen with exactly
// that many in flight, and one that runs a call too many is caught with it.
// The patience runs on the fake clock of testing/synctest, which passes
// only once every call that can have joined a round has.
func TestInFlightTellsALimitKeptFromOneExceeded(t *testing.T) {
	t.Parallel()

	const limit = 2
	tests := []struct {
		name  string
		width int
	}{
		{"a pool that keeps to the limit", limit},
		{"a pool that runs one call too many", limit + 1},
		{"a pool that runs one call at a time", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				calls := &fanouttest.InFlight{Hold: limit + 1, Patience: 20 * time.Millisecond}
				pool(tc.width, 6, func() { calls.Enter()() })
				if got := calls.Peak(); got != tc.width {
					t.Errorf("Peak = %d, want %d", got, tc.width)
				}
				if got := calls.Started(); got != 6 {
					t.Errorf("Started = %d, want 6", got)
				}
			})
		})
	}
}

// Calls that are all under way together are let go at once rather than
// after the wait, and a call that too few others can join waits for the
// patience, DefaultPatience when none is set or one that is not positive.
func TestInFlightHoldsACallForAsLongAsItsPatience(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		calls    *fanouttest.InFlight
		width    int
		wantWait time.Duration
	}{
		{"a full round", &fanouttest.InFlight{Hold: 4, Patience: time.Minute}, 4, 0},
		{"a call alone", &fanouttest.InFlight{Hold: 2, Patience: time.Second}, 1, time.Second},
		{"no patience", &fanouttest.InFlight{Hold: 2}, 1, fanouttest.DefaultPatience},
		{"a negative patience", &fanouttest.InFlight{Hold: 2, Patience: -time.Second}, 1, fanouttest.DefaultPatience},
		{"nothing to hold", &fanouttest.InFlight{Patience: time.Minute}, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				pool(tc.width, tc.width, func() { tc.calls.Enter()() })
				if got := time.Since(start); got != tc.wantWait {
					t.Errorf("the calls were held for %s, want %s", got, tc.wantWait)
				}
				if got := tc.calls.Peak(); got != tc.width {
					t.Errorf("Peak = %d, want %d", got, tc.width)
				}
			})
		})
	}
}

// A call is under way from Enter until it leaves, however often it says so:
// the calls after the second leave count with the one that has not left.
func TestInFlightCountsACallOutOnce(t *testing.T) {
	t.Parallel()

	var calls fanouttest.InFlight
	leave := calls.Enter()
	calls.Enter()
	leave()
	leave()
	calls.Enter()
	calls.Enter()
	if got := calls.Peak(); got != 3 {
		t.Errorf("Peak = %d, want 3: leaving twice counted the call out twice", got)
	}
	if got := calls.Started(); got != 4 {
		t.Errorf("Started = %d, want 4", got)
	}
}
