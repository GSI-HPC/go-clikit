// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package fanout works on many items at once, a bounded number at a time.
// Each is the one bounded loop, which reports nothing; Map works on items
// and reports them as a step of the progress package with a target each;
// and Batches works on a node set in batches, one after the other, with a
// pause between them, and reports them as a step with a span for each
// batch, under which the work of a batch reports its targets.
//
// Map and Batches keep the promises progresstest.Check holds an emitter
// to: every target or batch is announced queued before the first runs,
// each is ended before its place is given up, and those never started end
// too, canceled when the context left them out, so that a display's count
// reaches its total however the work ends.
//
// Only Map recovers a panic: one in the work for an item, or in what it
// acquires and releases for it, becomes that item's error, a *PanicError.
// Each recovers nothing, so a panic in its work ends the process, as in
// any goroutine, unless the work recovers it, as with Recovered. A panic
// in a batch's run, in Before or in BeforePause goes up the goroutine
// that called Batches, and progress.Bus.Close ends the spans it left open
// canceled.
//
// The package knows no program: its name, its panic log and the rule that
// tells an error's class are the Bus's, which MapOptions may override, and
// the error a step ends with is MapOptions.Summarize's.
package fanout

import (
	"context"
	"sync"
)

// DefaultLimit is how many items a pool works on at once when nothing says:
// a limit below one, given to Each or as MapOptions.Limit.
const DefaultLimit = 16

// Each calls work with every index below n, at most limit at a time, and
// returns once every call has returned; a limit below one is DefaultLimit,
// as it is for Map. When ctx ends, no further call is started, so an
// interrupt stops a fan-out the same way wherever it is; the caller tells
// what was left out by what work did not record.
func Each(ctx context.Context, n, limit int, work func(i int)) {
	if limit < 1 {
		limit = DefaultLimit
	}
	sem := make(chan struct{}, max(1, min(limit, n)))
	var wg sync.WaitGroup
	for i := range n {
		select {
		case <-ctx.Done():
		case sem <- struct{}{}:
			// When a slot and the cancellation are both ready, select
			// picks either, so the context is asked again.
			if ctx.Err() != nil {
				<-sem
			}
		}
		if ctx.Err() != nil {
			break
		}
		wg.Go(func() {
			defer func() { <-sem }()
			work(i)
		})
	}
	wg.Wait()
}
