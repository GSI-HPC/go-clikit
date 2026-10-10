// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"sync/atomic"
	"time"
)

// advanceEvery is the least time between two advances of a span, by the
// Bus's clock: a display draws at most ten times a second, so it misses
// nothing it would draw, and at a fan-out of 16 that is at most 160 events
// a second, as for lines.
const advanceEvery = 100 * time.Millisecond

// epoch is what nanos counts from.
var epoch = time.Unix(0, 0)

// nanos returns t as nanoseconds since the Unix epoch, held to the bounds
// of a Duration rather than undefined beyond them.
func nanos(t time.Time) int64 { return int64(t.Sub(epoch)) }

// work is the amount of a span's work. Advance and SetAmount move it
// without the Bus's lock, which is taken only to send it.
type work struct {
	// amount is the amount as Advance and SetAmount left it, which may be
	// newer than the one last sent, Fields.Amount.
	amount atomic.Int64
	// live says the span is running, so that Advance moves the amount.
	live atomic.Bool
	// next is when the next advance may be sent, by the Bus's clock, as
	// nanos counts it.
	next atomic.Int64
	// armed says a timer will send the amount once its place comes, and
	// timer is that timer, for the End to stop.
	armed atomic.Bool
	timer atomic.Pointer[time.Timer]

	// sent is when the last advance was sent, if hasSent; bus.mu guards
	// both.
	sent    time.Time
	hasSent bool
}

// Advance adds n to the amount of a span's work. A span that was given no
// Work counts Items. It does nothing to a span that is not running, and
// costs an atomic add and a read of BusOptions.Now, outside the Bus's lock,
// and the lock but once each 100 ms: the Bus sends the amount at most that
// often, the newest never lost, and with the span's End in any case.
func (s *Span) Advance(n int64) {
	if s == nil || !s.work.live.Load() {
		return
	}
	s.work.amount.Add(n)
	s.offerWork()
}

// SetAmount sets the amount of a span's work, for work that says how far
// it has got rather than how much more it did, such as a remote task that
// reports a percentage, or a download that starts over. An amount below 0
// is sent as 0. It does nothing to a span that is not running, and costs
// what Advance costs.
func (s *Span) SetAmount(n int64) {
	if s == nil || !s.work.live.Load() {
		return
	}
	s.work.amount.Store(n)
	s.offerWork()
}

// offerWork sends the amount when its place has come, and has a timer send
// it once its place comes otherwise.
func (s *Span) offerWork() {
	b := s.bus
	now := nanos(b.now())
	if wait := s.work.next.Load() - now; wait > 0 {
		s.arm(time.Duration(wait))
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s.sendWork()
}

// arm has a timer send the amount once wait has passed, unless one is
// armed already. A wait longer than advanceEvery, which only a clock that
// jumps back gives, is cut to it.
func (s *Span) arm(wait time.Duration) {
	if s.work.armed.Load() || !s.work.armed.CompareAndSwap(false, true) {
		return
	}
	s.work.timer.Store(time.AfterFunc(min(wait, advanceEvery), s.late))
}

// late sends the amount for the timer arm started, or arms another when
// the Bus's clock says its place has not come yet.
func (s *Span) late() {
	b := s.bus
	b.mu.Lock()
	defer b.mu.Unlock()
	s.work.timer.Store(nil)
	s.work.armed.Store(false)
	s.sendWork()
}

// sendWork sends the amount as a TypeAdvance, when it changed and 100 ms
// have passed since the last by the Bus's clock, and has it sent once they
// have otherwise. b.mu is held.
func (s *Span) sendWork() {
	w := &s.work
	if s.state != StateRunning {
		return
	}
	amount := max(w.amount.Load(), 0)
	if amount == s.fields.Amount {
		return
	}
	b := s.bus
	now := b.now()
	if wait := advanceEvery - now.Sub(w.sent); w.hasSent && wait > 0 {
		s.arm(wait)
		return
	}
	if s.fields.Unit == 0 {
		s.fields.Unit = Items
	}
	s.fields.Amount = amount
	w.sent, w.hasSent = now, true
	w.next.Store(nanos(now.Add(advanceEvery)))
	b.emitAt(s.event(TypeAdvance), now)
}

// endWork stops the amount moving, as the span ends, and takes the newest
// amount into its fields, so that its End carries it. b.mu is held.
func (s *Span) endWork() {
	w := &s.work
	w.live.Store(false)
	if t := w.timer.Swap(nil); t != nil {
		t.Stop()
	}
	if amount := max(w.amount.Load(), 0); amount != s.fields.Amount {
		if s.fields.Unit == 0 {
			s.fields.Unit = Items
		}
		s.fields.Amount = amount
	}
}
