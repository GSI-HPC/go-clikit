// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"fmt"
	"time"
)

const (
	// rateWindow is how far back the rate and the time left look, and
	// stallAfter how long the amount must not grow for the work to be
	// stalled.
	rateWindow = 5 * time.Second
	stallAfter = 5 * time.Second
	// leftAfter is how long a span must have run before it has a time left.
	leftAfter = 2 * time.Second
	// sampleEvery is the least time between a sample a span keeps of its
	// work and the one before the one before it, which bounds them to
	// some hundred a span. A sample merges the changes of less than that
	// time, which can raise a rate by no more than sampleEvery over
	// rateWindow, 2%.
	sampleEvery = 100 * time.Millisecond
	// almostDone is rule 2's share at most while a target it counts has not
	// ended, so that 100% means the count is complete. It stays clearly
	// below 1, so that a display cutting it to a whole percentage reads 99%.
	almostDone = 0.999
	// maxLeft bounds the time left, some 146 years, so that a share that
	// barely grows gives a time left a Duration holds.
	maxLeft = float64(1 << 62)
)

// Meter keeps the work of a Bus's spans from its events, rolled up the
// tree, the way a display shows how far the work has got.
//
// A span's rolled-up amount is its own and that of every span below it in
// the same unit, those that ended included; a span that reports no work of
// its own takes the unit of the first span below it that does. Its share
// done comes from the first of three rules that applies, which Bound
// names:
//
//  1. BySize: a span with a size of its own is its amount over that size.
//  2. ByTargets: a step with Fold, or a batch whose Total is known, once at
//     least one target below it has been bounded, is as far as the targets
//     that ended, however they ended, and the share done of each target
//     running, over its Total. A running target that is unbounded adds
//     nothing until it ends. The share stays short of done until Tally's
//     count reaches its Total, so that a step whose running targets have
//     all reached their sizes does not read as done.
//  3. ByBelow: any other span is its amount over the sum of the sizes of
//     the spans below it with work in its unit, those that ended included,
//     as long as every one of them has a size. A span with a size of its
//     own counts for everything below it: what is below it adds neither
//     its size nor its want of one. Work in Percent counts with a size of
//     100, so a span over several such calls is as far as the mean of
//     their shares.
//
// Without any of them a span is Unbounded. A share is held to 0 to 1, so
// that an amount that passes its size reads as done.
//
// A Meter is given every event of a Bus in order, as a sink is, and keeps
// a few seconds of each open span's work, for its rate and its time left.
// It forgets a span once it has ended, but its work still counts in the
// spans above. A Meter is not safe for concurrent use; a sink feeds it from
// Handle, under the Bus lock. The zero Meter is ready to use.
type Meter struct {
	spans map[SpanID]*metered
	// tally counts the targets that rule 2 needs.
	tally Tally
}

// Reading is the work of a span, rolled up. Fields may be added in a minor
// release.
type Reading struct {
	// Unit is the span's own, or that of the first span below it that
	// reported work.
	Unit Unit
	// Amount is the span's own amount and that of the spans below in Unit.
	Amount int64
	// Size is the bound of Amount, under BySize and ByBelow; 0 otherwise.
	Size int64
	// Bound says where Fraction comes from; Unbounded has none.
	Bound Bound
	// Fraction is the share of the work done, from 0 to 1.
	Fraction float64
	// Rate is how much Amount grew a second over the last five seconds, or
	// over the time the span has run when that is shorter. In the Reading
	// Add returns as a span ends, it is the mean over the whole time the
	// span ran.
	Rate float64
	// Left is the time the rest of the work should take, at the rate the
	// share grew over the last five seconds, once the span has run for two
	// seconds and while its share grows; 0 is not known.
	Left time.Duration
	// Stalled is how long Amount has not grown, once that is five seconds,
	// while the span runs; 0 otherwise.
	Stalled time.Duration
}

// Bound says which rule gives a span its share done.
type Bound uint8

const (
	// Unbounded is a span no rule gives a share.
	Unbounded Bound = iota
	// BySize is rule 1: the span's own Size.
	BySize
	// ByTargets is rule 2: the targets it counts.
	ByTargets
	// ByBelow is rule 3: the sizes of the work below it.
	ByBelow
)

// String returns "unbounded", "size", "targets" or "below", or "bound(n)"
// for a value this package does not define.
func (b Bound) String() string {
	switch b {
	case Unbounded:
		return "unbounded"
	case BySize:
		return "size"
	case ByTargets:
		return "targets"
	case ByBelow:
		return "below"
	}
	return fmt.Sprintf("bound(%d)", uint8(b))
}

// metered is what a Meter keeps of a span.
type metered struct {
	parent *metered
	kind   Kind
	state  State
	// tallied is the span in the Meter's Tally, which counts its targets
	// when it counts them; it is kept after the span ends.
	tallied *tallied

	// unit, amount and size are the span's own work, the amount held to
	// 0 or more and the size of work in Percent 100; a zero unit is none.
	unit         Unit
	amount, size int64
	// took is the unit of the first span below that reported work.
	took Unit
	// below sums the work below, by unit: the own amount of every span
	// below, and what bounds it, which each span directly below passes up.
	below []unitSum

	// bound and fraction are the span's share done after the last event.
	bound    Bound
	fraction float64
	// boundedBelow says a target below has been bounded, which rule 2
	// waits for.
	boundedBelow bool
	// running counts the targets running below, and shares adds up what
	// each of them adds to rule 2, its share done when it is bounded.
	running int
	shares  float64
	// added is what a running target adds to the shares of every span
	// above it that counts targets.
	added float64

	// ran is when the span started running, and grew when its amount last
	// grew.
	ran, grew time.Time
	// samples are the amount and share of the last five seconds or so,
	// oldest first, the newest the latest; a sample is at least
	// sampleEvery after the one before the one before it. There is always
	// one, from the start of the span.
	samples []sample
}

// unitSum is the work below a span in one unit.
type unitSum struct {
	unit Unit
	// amount adds up the own amount of every span below.
	amount int64
	// size and unsized are what bounds that amount: the sizes of the
	// topmost spans below with a size of their own, and how many spans
	// with work but no size there are with none of those above them.
	size    int64
	unsized int
}

// sample is a span's amount and share at a time.
type sample struct {
	t        time.Time
	amount   int64
	fraction float64
}

// Add counts e in. When e ends a span with work, own or below, Add returns
// what its work came to.
func (m *Meter) Add(e Event) (ended Reading, ok bool) {
	m.tally.Add(e)
	if e.Type == TypeStart {
		m.start(e)
		return Reading{}, false
	}
	s := m.spans[e.Span]
	if s == nil {
		return Reading{}, false
	}
	switch e.Type {
	case TypeRun:
		if s.state == StateQueued {
			s.run(e.Time)
		}
	case TypeUpdate, TypeAdvance:
		// Their work is taken below, as that of a run is.
	case TypeEnd:
		return m.end(s, e)
	default:
		return Reading{}, false
	}
	s.own(e.Fields)
	s.settle(e.Time)
	return Reading{}, false
}

// Read returns the work of an open span as it stands at now, when it has
// any.
func (m *Meter) Read(span SpanID, now time.Time) (Reading, bool) {
	s := m.spans[span]
	if s == nil || s.workUnit() == 0 {
		return Reading{}, false
	}
	r := s.reading()
	if s.state != StateRunning {
		return r, true
	}
	from := now.Add(-rateWindow)
	if from.Before(s.ran) {
		from = s.ran
	}
	if took := now.Sub(from); took > 0 {
		then := s.at(from)
		r.Rate = max(float64(r.Amount-then.amount)/took.Seconds(), 0)
		if grew := r.Fraction - then.fraction; grew > 0 && now.Sub(s.ran) >= leftAfter {
			r.Left = time.Duration(min(float64(took)*(1-r.Fraction)/grew, maxLeft))
		}
	}
	if still := now.Sub(s.grew); still >= stallAfter {
		r.Stalled = still
	}
	return r, true
}

// start counts in a span that starts, unless it started before.
func (m *Meter) start(e Event) {
	if m.spans == nil {
		m.spans = map[SpanID]*metered{}
	}
	if _, again := m.spans[e.Span]; again {
		return
	}
	s := &metered{kind: e.Kind, state: StateQueued, tallied: m.tally.spans[e.Span]}
	if e.Parent != 0 {
		s.parent = m.spans[e.Parent]
	}
	m.spans[e.Span] = s
	if e.State != StateQueued {
		s.run(e.Time)
	}
	s.own(e.Fields)
	s.settle(e.Time)
}

// end counts in the end of s, and forgets it.
func (m *Meter) end(s *metered, e Event) (Reading, bool) {
	delete(m.spans, e.Span)
	was := s.state
	s.state = StateEnded
	if was == StateRunning && s.kind == KindTarget {
		s.counting(func(p *metered) { p.running-- })
	}
	s.own(e.Fields)
	s.settle(e.Time)
	if s.workUnit() == 0 {
		return Reading{}, false
	}
	r := s.reading()
	if was != StateQueued {
		if took := e.Time.Sub(s.ran); took > 0 {
			r.Rate = float64(r.Amount) / took.Seconds()
		}
	}
	return r, true
}

// run marks s running from t.
func (s *metered) run(t time.Time) {
	s.state = StateRunning
	s.ran, s.grew = t, t
	if s.kind == KindTarget {
		s.counting(func(p *metered) { p.running++ })
	}
}

// own takes the span's own work from f, and moves it in the sums of the
// spans above.
func (s *metered) own(f Fields) {
	unit, amount, size := f.Unit, max(f.Amount, 0), max(f.Size, 0)
	if unit == Percent {
		size = 100
	}
	if unit == s.unit && amount == s.amount && size == s.size {
		return
	}
	// The bounds s passes up in its old unit and in its new one, before
	// the change.
	was := s.unit
	wasSize, wasUnsized := s.bounds(was)
	unitSize, unitUnsized := s.bounds(unit)
	if was != 0 {
		s.above(func(p *metered) { p.sum(was).amount -= s.amount })
	}
	s.unit, s.amount, s.size = unit, amount, size
	if unit != 0 {
		s.above(func(p *metered) {
			p.sum(unit).amount += amount
			if p.took == 0 {
				p.took = unit
			}
		})
	}
	if was != unit {
		size, unsized := s.bounds(was)
		s.pass(was, size-wasSize, unsized-wasUnsized)
		wasSize, wasUnsized = unitSize, unitUnsized
	}
	size, unsized := s.bounds(unit)
	s.pass(unit, size-wasSize, unsized-wasUnsized)
}

// bounds returns what bounds the work of s and of the spans below it in
// unit, as s passes it up: its own size when it has one, else the sizes
// and the spans without one that the spans directly below pass up, and s
// among those when it has work in unit but no size. A zero unit has none.
func (s *metered) bounds(unit Unit) (size int64, unsized int) {
	if unit == 0 {
		return 0, 0
	}
	if s.unit == unit && s.size > 0 {
		return s.size, 0
	}
	if b := s.find(unit); b != nil {
		size, unsized = b.size, b.unsized
	}
	if s.unit == unit {
		unsized++
	}
	return size, unsized
}

// pass moves what bounds the work below the parent of s in unit by size
// and unsized, and on up the tree as far as it changes what a span passes
// up: no further than the first span with a size of its own in unit.
func (s *metered) pass(unit Unit, size int64, unsized int) {
	for p := s.parent; p != nil && (size != 0 || unsized != 0); p = p.parent {
		wasSize, wasUnsized := p.bounds(unit)
		b := p.sum(unit)
		b.size += size
		b.unsized += unsized
		nowSize, nowUnsized := p.bounds(unit)
		size, unsized = nowSize-wasSize, nowUnsized-wasUnsized
	}
}

// find returns what s sums of the work below in unit; nil when there is
// none yet.
func (s *metered) find(unit Unit) *unitSum {
	for i := range s.below {
		if s.below[i].unit == unit {
			return &s.below[i]
		}
	}
	return nil
}

// sum returns what s sums of the work below in unit, adding it when there
// is none yet.
func (s *metered) sum(unit Unit) *unitSum {
	if b := s.find(unit); b != nil {
		return b
	}
	s.below = append(s.below, unitSum{unit: unit})
	return &s.below[len(s.below)-1]
}

// settle works out again the share of s and of every span above it, the
// spans below first, and samples their work at t.
func (s *metered) settle(t time.Time) {
	for n := s; n != nil; n = n.parent {
		n.share()
		if n.kind == KindTarget {
			n.offer()
		}
		n.sample(t)
	}
}

// share works out the share of s by the first rule that applies.
func (s *metered) share() {
	s.bound, s.fraction = Unbounded, 0
	unit := s.workUnit()
	switch {
	case unit == 0:
	case s.unit != 0 && s.size > 0:
		s.bound, s.fraction = BySize, ratio(float64(s.rolled()), float64(s.size))
	case s.tallied.counts && s.boundedBelow && s.tallied.count.Total > 0:
		if s.running == 0 {
			s.shares = 0
		}
		done := float64(s.tallied.count.Done) + s.shares
		s.bound, s.fraction = ByTargets, ratio(done, float64(s.tallied.count.Total))
		if s.tallied.count.Done < s.tallied.count.Total {
			s.fraction = min(s.fraction, almostDone)
		}
	default:
		if b := s.sum(unit); b.unsized == 0 && b.size > 0 {
			s.bound, s.fraction = ByBelow, ratio(float64(s.rolled()), float64(b.size))
		}
	}
}

// offer passes the share of a target to the spans above that count it:
// what it adds to their rule 2 while it runs, and that it is bounded.
func (s *metered) offer() {
	add := 0.0
	if s.state == StateRunning {
		add = s.fraction
	}
	bounded := s.bound != Unbounded
	if add == s.added && !bounded {
		return
	}
	more := add - s.added
	s.added = add
	s.counting(func(p *metered) {
		p.shares = max(p.shares+more, 0)
		p.boundedBelow = p.boundedBelow || bounded
	})
}

// sample keeps the work of s at t when it changed, and when its amount
// grew, the time it did.
func (s *metered) sample(t time.Time) {
	amount, n := s.rolled(), len(s.samples)
	if n > 0 {
		last := s.samples[n-1]
		if amount > last.amount {
			s.grew = t
		}
		if amount == last.amount && s.fraction == last.fraction {
			return
		}
	}
	next := sample{t: t, amount: amount, fraction: s.fraction}
	if n >= 2 && t.Sub(s.samples[n-2].t) < sampleEvery {
		s.samples[n-1] = next
	} else {
		s.samples = append(s.samples, next)
	}
	old := 0
	for old+1 < len(s.samples) && !s.samples[old+1].t.After(t.Add(-rateWindow)) {
		old++
	}
	if old > 0 {
		s.samples = append(s.samples[:0], s.samples[old:]...)
	}
}

// at returns the sample of s that stood at t: the newest not after it, or
// the oldest kept.
func (s *metered) at(t time.Time) sample {
	then := s.samples[0]
	for _, x := range s.samples[1:] {
		if x.t.After(t) {
			break
		}
		then = x
	}
	return then
}

// reading returns the work of s as its last event left it.
func (s *metered) reading() Reading {
	r := Reading{Unit: s.workUnit(), Amount: s.rolled(), Bound: s.bound, Fraction: s.fraction}
	switch s.bound {
	case BySize:
		r.Size = s.size
	case ByBelow:
		r.Size = s.sum(r.Unit).size
	}
	return r
}

// workUnit returns the unit of the work of s: its own, or the one it took
// from below; 0 when it has none.
func (s *metered) workUnit() Unit {
	if s.unit != 0 {
		return s.unit
	}
	return s.took
}

// rolled returns the amount of s and of the spans below it in its unit.
func (s *metered) rolled() int64 {
	unit := s.workUnit()
	if unit == 0 {
		return 0
	}
	amount := s.sum(unit).amount
	if s.unit == unit {
		amount += s.amount
	}
	return amount
}

// above calls f with every span above s.
func (s *metered) above(f func(*metered)) {
	for p := s.parent; p != nil; p = p.parent {
		f(p)
	}
}

// counting calls f with every span above s that counts targets.
func (s *metered) counting(f func(*metered)) {
	s.above(func(p *metered) {
		if p.tallied.counts {
			f(p)
		}
	})
}

// ratio returns done over all, held to 0 to 1.
func ratio(done, all float64) float64 { return min(max(done/all, 0), 1) }
