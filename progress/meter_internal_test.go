// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"testing"
	"time"
)

// A Meter keeps a few seconds of each open span's work, however often it
// advances, and forgets a span once it has ended, while what the span did
// still counts above it.
func TestTheMeterKeepsItsMemoryBounded(t *testing.T) {
	t.Parallel()
	var m Meter
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	seq := uint64(0)
	send := func(e Event) {
		seq++
		e.Seq, e.Time = seq, now
		m.Add(e)
	}
	send(Event{Type: TypeStart, Span: 1, Kind: KindStep, State: StateRunning})
	call := Event{Span: 2, Parent: 1, Kind: KindCall, State: StateRunning,
		Fields: Fields{Unit: Bytes, Size: 1 << 40}}
	call.Type = TypeStart
	send(call)
	call.Type = TypeAdvance
	most := 0
	for i := range 6000 {
		now = now.Add(10 * time.Millisecond)
		call.Amount = int64(i+1) * 1000
		send(call)
		most = max(most, len(m.spans[1].samples), len(m.spans[2].samples))
	}
	if limit := int(2*rateWindow/sampleEvery) + 2; most > limit {
		t.Errorf("a span kept %d samples, more than %d", most, limit)
	}
	// The samples it merged cost the rate no more than the share of the
	// window a sample spans.
	if r, _ := m.Read(1, now); r.Rate < 100000 || r.Rate > 100000*(1+float64(sampleEvery)/float64(rateWindow)) {
		t.Errorf("the step's rate is %v, want 100000 or a little more", r.Rate)
	}

	call.Type, call.State = TypeEnd, StateEnded
	send(call)
	if len(m.spans) != 1 || len(m.tally.spans) != 1 {
		t.Errorf("the meter keeps %d spans and its tally %d after the call ended, want 1 each", len(m.spans), len(m.tally.spans))
	}
	if r, _ := m.Read(1, now); r.Amount != 6000*1000 {
		t.Errorf("the step's amount is %d once the call ended, want %d", r.Amount, 6000*1000)
	}
}
