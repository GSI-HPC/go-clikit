// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// metering feeds a Meter events by hand, as a Bus sends them, at a clock
// the test moves: every event of a span carries what the span's last one
// did, but for what the test changes.
type metering struct {
	meter progress.Meter
	now   time.Time
	seq   uint64
	last  map[progress.SpanID]progress.Event
}

func newMetering() *metering {
	return &metering{now: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC), last: map[progress.SpanID]progress.Event{}}
}

// send sends the meter an event of typ about span, changed by sets, and
// returns what Add returned.
func (m *metering) send(typ progress.Type, span progress.SpanID, sets ...func(*progress.Event)) (progress.Reading, bool) {
	e := m.last[span]
	m.seq++
	e.Seq, e.Time, e.Type, e.Span = m.seq, m.now, typ, span
	switch typ {
	case progress.TypeStart, progress.TypeRun:
		e.State = progress.StateRunning
	case progress.TypeEnd:
		e.State, e.Status = progress.StateEnded, progress.StatusOK
	}
	for _, set := range sets {
		set(&e)
	}
	m.last[span] = e
	return m.meter.Add(e)
}

// start starts span under parent.
func (m *metering) start(span, parent progress.SpanID, kind progress.Kind, sets ...func(*progress.Event)) {
	m.send(progress.TypeStart, span, append([]func(*progress.Event){func(e *progress.Event) {
		e.Parent, e.Kind = parent, kind
	}}, sets...)...)
}

// advance sends an advance of span to amount.
func (m *metering) advance(span progress.SpanID, amount int64) {
	m.send(progress.TypeAdvance, span, func(e *progress.Event) { e.Amount = amount })
}

// end ends span.
func (m *metering) end(span progress.SpanID) { m.send(progress.TypeEnd, span) }

// read returns the work of span at the meter's clock.
func (m *metering) read(t *testing.T, span progress.SpanID) progress.Reading {
	t.Helper()
	r, ok := m.meter.Read(span, m.now)
	if !ok {
		t.Fatalf("span %d has no work", span)
	}
	return r
}

// owns sets a span's own work.
func owns(u progress.Unit, amount, size int64) func(*progress.Event) {
	return func(e *progress.Event) { e.Unit, e.Amount, e.Size = u, amount, size }
}

// queued starts a span queued.
func queued(e *progress.Event) { e.State = progress.StateQueued }

// counting makes a span a step with Fold that expects total targets.
func counting(total int) func(*progress.Event) {
	return func(e *progress.Event) { e.Flags, e.Total = progress.Fold, total }
}

// totalOf sets a span's Total.
func totalOf(total int) func(*progress.Event) {
	return func(e *progress.Event) { e.Total = total }
}

func TestBoundNamesItsRule(t *testing.T) {
	t.Parallel()
	for b, want := range map[progress.Bound]string{
		progress.Unbounded: "unbounded",
		progress.BySize:    "size",
		progress.ByTargets: "targets",
		progress.ByBelow:   "below",
		progress.Bound(9):  "bound(9)",
	} {
		if got := b.String(); got != want {
			t.Errorf("Bound(%d).String() = %q, want %q", b, got, want)
		}
	}
}

// The spans of the trees below: a command, a step, and what the step does.
const (
	cmd progress.SpanID = iota + 1
	step
	first
	second
	third
	fourth
)

// A span's rolled-up amount is its own and that of every span below it in
// the same unit, those that ended included; a span without work of its own
// takes the unit of the first span below it that reports some, and work in
// another unit adds nothing to it.
func TestTheMeterRollsTheAmountUpTheTree(t *testing.T) {
	t.Parallel()
	m := newMetering()
	m.start(cmd, 0, progress.KindCommand)
	m.start(step, cmd, progress.KindStep)
	if _, ok := m.meter.Read(step, m.now); ok {
		t.Errorf("a step without work has a reading")
	}
	m.start(first, step, progress.KindTarget, owns(progress.Bytes, 0, 1000))
	m.start(second, first, progress.KindCall, owns(progress.Bytes, 0, 0))
	m.start(third, first, progress.KindCall, owns(progress.Items, 7, 0))
	m.start(fourth, step, progress.KindTarget, owns(progress.Bytes, 200, 0))
	m.advance(second, 300)
	m.send(progress.TypeEnd, second, func(e *progress.Event) { e.Amount = 400 })

	for _, tc := range []struct {
		span   progress.SpanID
		unit   progress.Unit
		amount int64
	}{
		{cmd, progress.Bytes, 600},
		{step, progress.Bytes, 600},
		{first, progress.Bytes, 400},
		{third, progress.Items, 7},
		{fourth, progress.Bytes, 200},
	} {
		if r := m.read(t, tc.span); r.Unit != tc.unit || r.Amount != tc.amount {
			t.Errorf("span %d reads %d %s, want %d %s", tc.span, r.Amount, r.Unit, tc.amount, tc.unit)
		}
	}
	if _, ok := m.meter.Read(second, m.now); ok {
		t.Errorf("a span that ended still has a reading")
	}
}

// A span's share done comes from its own size, else from the targets it
// counts once one of them is bounded, else from the sizes of the work
// below it, and it is unbounded without any of them.
func TestTheMeterTakesTheShareFromTheFirstRuleThatApplies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// run sends the events, and returns the span to read.
		run  func(m *metering) progress.SpanID
		want progress.Reading
		ok   bool
	}{
		{"no work", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindCall)
			return step
		}, progress.Reading{}, false},
		{"its own size", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep, owns(progress.Bytes, 0, 1000))
			m.start(first, step, progress.KindCall, owns(progress.Bytes, 250, 0))
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 250, Size: 1000, Bound: progress.BySize, Fraction: 0.25}, true},
		{"an amount past its size is done", func(m *metering) progress.SpanID {
			m.start(first, 0, progress.KindCall, owns(progress.Bytes, 3000, 2000))
			return first
		}, progress.Reading{Unit: progress.Bytes, Amount: 3000, Size: 2000, Bound: progress.BySize, Fraction: 1}, true},
		{"its targets once one is bounded", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep, counting(4))
			m.start(first, step, progress.KindTarget, queued, owns(progress.Bytes, 0, 100))
			m.start(second, step, progress.KindTarget, queued, owns(progress.Bytes, 0, 100))
			m.send(progress.TypeRun, first)
			m.advance(first, 100)
			m.end(first)
			m.send(progress.TypeRun, second)
			m.advance(second, 50)
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 150, Bound: progress.ByTargets, Fraction: 0.375}, true},
		{"its targets at their sizes are short of done", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep, counting(2))
			m.start(first, step, progress.KindTarget, queued, owns(progress.Bytes, 0, 100))
			m.start(second, step, progress.KindTarget, queued, owns(progress.Bytes, 0, 100))
			m.send(progress.TypeRun, first)
			m.send(progress.TypeRun, second)
			m.advance(first, 100)
			m.advance(second, 100)
			m.end(first)
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 200, Bound: progress.ByTargets, Fraction: 0.999}, true},
		{"a running target without a bound adds nothing", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep, counting(2))
			m.start(first, step, progress.KindTarget, owns(progress.Bytes, 50, 100))
			m.start(second, step, progress.KindTarget, owns(progress.Items, 9, 0))
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 50, Bound: progress.ByTargets, Fraction: 0.25}, true},
		{"targets without a bound leave the count to say it", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep, counting(2))
			m.start(first, step, progress.KindTarget, owns(progress.Items, 4, 0))
			m.start(second, step, progress.KindTarget, queued)
			m.end(first)
			return step
		}, progress.Reading{Unit: progress.Items, Amount: 4}, true},
		{"every target ended", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep, counting(2))
			m.start(first, step, progress.KindTarget, owns(progress.Bytes, 0, 3))
			m.start(second, step, progress.KindTarget, owns(progress.Bytes, 0, 3))
			m.advance(first, 1)
			m.advance(second, 2)
			m.end(first)
			m.end(second)
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 3, Bound: progress.ByTargets, Fraction: 1}, true},
		{"a batch whose Total is not known", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindBatch)
			m.start(first, step, progress.KindTarget, owns(progress.Bytes, 50, 100))
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 50, Size: 100, Bound: progress.ByBelow, Fraction: 0.5}, true},
		{"a batch whose Total is known", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindBatch, totalOf(2))
			m.start(first, step, progress.KindTarget, owns(progress.Bytes, 50, 100))
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 50, Bound: progress.ByTargets, Fraction: 0.25}, true},
		{"the mean of the calls in percent", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindCall, owns(progress.Percent, 40, 100))
			m.start(second, step, progress.KindCall, owns(progress.Percent, 80, 100))
			m.end(second)
			return step
		}, progress.Reading{Unit: progress.Percent, Amount: 120, Size: 200, Bound: progress.ByBelow, Fraction: 0.6}, true},
		{"a sized call over a sized call counts once", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindCall, owns(progress.Bytes, 0, 100))
			m.start(second, first, progress.KindCall, owns(progress.Bytes, 0, 100))
			m.advance(second, 100)
			m.end(second)
			m.end(first)
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 100, Size: 100, Bound: progress.ByBelow, Fraction: 1}, true},
		{"a sized target over a sized call counts once", func(m *metering) progress.SpanID {
			m.start(cmd, 0, progress.KindCommand)
			m.start(step, cmd, progress.KindStep)
			m.start(first, step, progress.KindTarget, owns(progress.Bytes, 0, 1000))
			m.start(second, first, progress.KindCall, owns(progress.Bytes, 0, 1000))
			m.advance(second, 1000)
			m.end(second)
			m.end(first)
			return cmd
		}, progress.Reading{Unit: progress.Bytes, Amount: 1000, Size: 1000, Bound: progress.ByBelow, Fraction: 1}, true},
		{"a sized target over an unsized call bounds it", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindTarget, owns(progress.Bytes, 0, 1000))
			m.start(second, first, progress.KindCall, owns(progress.Bytes, 400, 0))
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 400, Size: 1000, Bound: progress.ByBelow, Fraction: 0.4}, true},
		{"a size given up passes up what it covered", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindTarget, owns(progress.Bytes, 10, 100))
			m.start(second, first, progress.KindCall, owns(progress.Bytes, 5, 50))
			m.send(progress.TypeUpdate, first, owns(0, 0, 0))
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 5, Size: 50, Bound: progress.ByBelow, Fraction: 0.1}, true},
		{"a size not known below", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindCall, owns(progress.Bytes, 10, 100))
			m.start(second, step, progress.KindCall, owns(progress.Bytes, 5, 0))
			return step
		}, progress.Reading{Unit: progress.Bytes, Amount: 15}, true},
		{"its own unit before the one below", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep, owns(progress.Items, 3, 0))
			m.start(first, step, progress.KindCall, owns(progress.Bytes, 10, 100))
			return step
		}, progress.Reading{Unit: progress.Items, Amount: 3}, true},
		{"a unit taken from below gives way to its own", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindCall, owns(progress.Bytes, 10, 100))
			m.send(progress.TypeUpdate, step, owns(progress.Items, 5, 10))
			return step
		}, progress.Reading{Unit: progress.Items, Amount: 5, Size: 10, Bound: progress.BySize, Fraction: 0.5}, true},
		{"an amount below 0 is none", func(m *metering) progress.SpanID {
			m.start(first, 0, progress.KindCall, owns(progress.Bytes, -5, 10))
			return first
		}, progress.Reading{Unit: progress.Bytes, Size: 10, Bound: progress.BySize}, true},
		{"a unit that changes takes its amount along", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindCall, owns(progress.Bytes, 10, 100))
			m.send(progress.TypeUpdate, first, owns(progress.Items, 10, 0))
			return step
		}, progress.Reading{Unit: progress.Bytes}, true},
		{"work given up below", func(m *metering) progress.SpanID {
			m.start(step, 0, progress.KindStep)
			m.start(first, step, progress.KindCall, owns(progress.Bytes, 10, 100))
			m.send(progress.TypeUpdate, first, owns(0, 0, 0))
			return step
		}, progress.Reading{Unit: progress.Bytes}, true},
		{"work in percent is out of 100", func(m *metering) progress.SpanID {
			m.start(first, 0, progress.KindCall, owns(progress.Percent, 30, 7))
			return first
		}, progress.Reading{Unit: progress.Percent, Amount: 30, Size: 100, Bound: progress.BySize, Fraction: 0.3}, true},
		{"a span started under one that ended", func(m *metering) progress.SpanID {
			m.start(cmd, 0, progress.KindCommand)
			m.start(step, cmd, progress.KindStep)
			m.end(step)
			m.start(first, step, progress.KindCall, owns(progress.Bytes, 10, 100))
			return cmd
		}, progress.Reading{}, false},
		{"an event after the end", func(m *metering) progress.SpanID {
			m.start(first, 0, progress.KindCall, owns(progress.Bytes, 10, 100))
			m.end(first)
			m.advance(first, 20)
			return first
		}, progress.Reading{}, false},
		{"a second start", func(m *metering) progress.SpanID {
			m.start(first, 0, progress.KindCall, owns(progress.Bytes, 10, 100))
			m.start(first, 0, progress.KindCall, owns(progress.Items, 1, 0))
			return first
		}, progress.Reading{Unit: progress.Bytes, Amount: 10, Size: 100, Bound: progress.BySize, Fraction: 0.1}, true},
		{"a run of a span that runs", func(m *metering) progress.SpanID {
			m.start(first, 0, progress.KindCall, owns(progress.Bytes, 10, 100))
			m.send(progress.TypeRun, first, owns(progress.Bytes, 20, 100))
			return first
		}, progress.Reading{Unit: progress.Bytes, Amount: 20, Size: 100, Bound: progress.BySize, Fraction: 0.2}, true},
		{"events that do not carry work", func(m *metering) progress.SpanID {
			m.start(first, 0, progress.KindCall, owns(progress.Bytes, 10, 100))
			m.send(progress.TypeLine, first, owns(0, 0, 0))
			m.send(progress.TypeSuspend, first, owns(0, 0, 0))
			m.send(progress.TypeRun, fourth)
			return first
		}, progress.Reading{Unit: progress.Bytes, Amount: 10, Size: 100, Bound: progress.BySize, Fraction: 0.1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newMetering()
			span := tc.run(m)
			got, ok := m.meter.Read(span, m.now)
			if got != tc.want || ok != tc.ok {
				t.Errorf("Read = %+v, %v, want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// download starts a step over a call that downloads 2 GiB, 8 MiB each
// 100 ms for n times.
func download(m *metering, n int) {
	m.start(step, 0, progress.KindStep)
	m.start(first, step, progress.KindCall, owns(progress.Bytes, 0, 2<<30))
	for i := range n {
		m.now = m.now.Add(100 * time.Millisecond)
		m.advance(first, int64(i+1)*8<<20)
	}
}

// The rate is how much the amount grew a second over the last five
// seconds, or since the span started running when that is shorter; the
// time left is the share still to do, at the rate the share grew over the
// same time, once the span has run two seconds; and once the amount has not
// grown for five seconds, the work is stalled.
func TestTheMeterReadsTheRateTheTimeLeftAndAStall(t *testing.T) {
	t.Parallel()
	const mib = 1 << 20
	for _, tc := range []struct {
		name     string
		advances int
		// after is how long after the last advance the meter is read.
		after time.Duration
		want  progress.Reading
	}{
		{"as it starts", 0, 0, progress.Reading{Unit: progress.Bytes, Size: 2 << 30, Bound: progress.ByBelow}},
		{"for the time it ran", 10, 0, progress.Reading{Unit: progress.Bytes, Amount: 80 * mib, Size: 2 << 30,
			Bound: progress.ByBelow, Fraction: 80.0 / 2048, Rate: 80 * mib}},
		{"over the last five seconds", 144, 0, progress.Reading{Unit: progress.Bytes, Amount: 1152 * mib, Size: 2 << 30,
			Bound: progress.ByBelow, Fraction: 0.5625, Rate: 80 * mib, Left: 11200 * time.Millisecond}},
		{"as it slows", 144, 2 * time.Second, progress.Reading{Unit: progress.Bytes, Amount: 1152 * mib, Size: 2 << 30,
			Bound: progress.ByBelow, Fraction: 0.5625, Rate: 48 * mib, Left: 18666666666}},
		{"stalled", 144, 5 * time.Second, progress.Reading{Unit: progress.Bytes, Amount: 1152 * mib, Size: 2 << 30,
			Bound: progress.ByBelow, Fraction: 0.5625, Stalled: 5 * time.Second}},
		{"read before it started", 0, -time.Second, progress.Reading{Unit: progress.Bytes, Size: 2 << 30, Bound: progress.ByBelow}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newMetering()
			download(m, tc.advances)
			got, _ := m.meter.Read(step, m.now.Add(tc.after))
			if got != tc.want {
				t.Errorf("Read = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A queued span has work, but no rate, time left or stall; an amount that
// falls has no rate below 0; and a share that barely grows has a time left
// a Duration holds.
func TestTheMeterReadsOnlyWhatARunningSpanHas(t *testing.T) {
	t.Parallel()
	m := newMetering()
	m.start(first, 0, progress.KindTarget, queued, owns(progress.Bytes, 0, 100))
	m.start(second, 0, progress.KindCall, owns(progress.Bytes, 50, 100))
	m.start(third, 0, progress.KindCall, owns(progress.Bytes, 0, 1<<62))
	m.now = m.now.Add(time.Second)
	m.advance(second, 10)
	m.advance(third, 1)
	m.now = m.now.Add(2 * time.Second)

	for _, tc := range []struct {
		span progress.SpanID
		want progress.Reading
	}{
		{first, progress.Reading{Unit: progress.Bytes, Size: 100, Bound: progress.BySize}},
		{second, progress.Reading{Unit: progress.Bytes, Amount: 10, Size: 100, Bound: progress.BySize, Fraction: 0.1}},
		{third, progress.Reading{Unit: progress.Bytes, Amount: 1, Size: 1 << 62, Bound: progress.BySize, Fraction: 1.0 / (1 << 62),
			Rate: 1.0 / 3, Left: 1 << 62}},
	} {
		if got := m.read(t, tc.span); got != tc.want {
			t.Errorf("span %d reads %+v, want %+v", tc.span, got, tc.want)
		}
	}
}

// The reading Add returns as a span with work ends says what its work came
// to, at the mean rate of the time it ran.
func TestTheMeterSaysWhatTheWorkCameTo(t *testing.T) {
	t.Parallel()
	m := newMetering()
	download(m, 256)
	m.end(first)
	got, ok := m.send(progress.TypeEnd, step)
	want := progress.Reading{Unit: progress.Bytes, Amount: 2 << 30, Size: 2 << 30, Bound: progress.ByBelow, Fraction: 1,
		Rate: 80 << 20}
	if !ok || got != want {
		t.Errorf("the step ended with %+v, %v, want %+v", got, ok, want)
	}

	for _, tc := range []struct {
		name string
		// run sends the events up to the end, which it returns.
		run  func(m *metering) (progress.Reading, bool)
		want progress.Reading
		ok   bool
	}{
		{"without work", func(m *metering) (progress.Reading, bool) {
			m.start(first, 0, progress.KindCall)
			m.now = m.now.Add(time.Second)
			return m.send(progress.TypeEnd, first)
		}, progress.Reading{}, false},
		{"left out while queued", func(m *metering) (progress.Reading, bool) {
			m.start(first, 0, progress.KindTarget, queued, owns(progress.Bytes, 0, 100))
			m.now = m.now.Add(time.Second)
			return m.send(progress.TypeEnd, first)
		}, progress.Reading{Unit: progress.Bytes, Size: 100, Bound: progress.BySize}, true},
		{"in no time", func(m *metering) (progress.Reading, bool) {
			m.start(first, 0, progress.KindCall, owns(progress.Items, 0, 0))
			return m.send(progress.TypeEnd, first, owns(progress.Items, 7, 0))
		}, progress.Reading{Unit: progress.Items, Amount: 7}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newMetering()
			got, ok := tc.run(m)
			if got != tc.want || ok != tc.ok {
				t.Errorf("Add = %+v, %v, want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// FuzzMeter feeds a Meter the events of spans that start, run, change
// their work, end and wait in any order, and holds every reading to what
// the events said: a span's rolled-up amount is its own and that of every
// span below it in its unit, those that ended included, a span with work
// of its own has a reading in its unit, and a share is from 0 to 1.
func FuzzMeter(f *testing.F) {
	f.Add([]byte{0, 0, 8, 2, 0, 1, 0, 0x45, 0, 1, 16, 0x24, 1, 1, 0, 0, 4, 20, 0, 0, 2, 1, 60, 0x05, 3, 1, 0, 0, 3, 0, 0, 0})
	f.Add([]byte{0, 0, 0, 0, 0, 0, 3, 0x48, 0, 1, 3, 0x48, 2, 1, 50, 2, 4, 40, 0, 0, 2, 2, 90, 3, 4, 200, 0, 0, 3, 2, 0, 0})
	f.Fuzz(func(t *testing.T, ops []byte) {
		if len(ops) > 4*64 {
			return
		}
		type known struct {
			parent progress.SpanID
			open   bool
			unit   progress.Unit
			amount int64
		}
		m := newMetering()
		spans := map[progress.SpanID]*known{}
		var ids []progress.SpanID
		pick := func(b byte) progress.SpanID {
			if len(ids) == 0 || b%5 == 0 {
				return 0
			}
			return ids[int(b)%len(ids)]
		}
		// sent keeps what an event said of an open span's work.
		sent := func(id progress.SpanID) {
			if k := spans[id]; k != nil && k.open {
				e := m.last[id]
				k.unit, k.amount = e.Unit, max(e.Amount, 0)
			}
		}
		check := func(id progress.SpanID, r progress.Reading) {
			want := int64(0)
			for s, k := range spans {
				if k.unit != r.Unit {
					continue
				}
				for p := s; p != 0; p = spans[p].parent {
					if p == id {
						want += k.amount
						break
					}
				}
			}
			if r.Amount != want {
				t.Fatalf("span %d reads %d %s, but the events say %d", id, r.Amount, r.Unit, want)
			}
			if !(r.Fraction >= 0 && r.Fraction <= 1) || r.Bound == progress.Unbounded && r.Fraction != 0 {
				t.Fatalf("span %d reads a share of %v, %s", id, r.Fraction, r.Bound)
			}
			if r.Rate < 0 || r.Left < 0 || r.Stalled != 0 && r.Stalled < 5*time.Second {
				t.Fatalf("span %d reads %+v", id, r)
			}
			if u := spans[id].unit; u != 0 && r.Unit != u {
				t.Fatalf("span %d reads %s, but its own work is in %s", id, r.Unit, u)
			}
		}
		for ; len(ops) >= 4; ops = ops[4:] {
			op, a, b, c := ops[0], ops[1], ops[2], ops[3]
			setWork := func(e *progress.Event) {
				e.Unit, e.Amount, e.Size = progress.Unit(c>>2%5), int64(int8(b)), int64(c>>5)*10
			}
			switch op % 5 {
			case 0:
				id := progress.SpanID(len(ids) + 1)
				parent := pick(a)
				k := &known{open: true}
				if p := spans[parent]; p != nil && p.open {
					k.parent = parent
				}
				spans[id], ids = k, append(ids, id)
				m.start(id, parent, progress.Kind(1+b%6), setWork, func(e *progress.Event) {
					e.Amount = int64(int8(a))
					e.Total = int(c % 4)
					if b&8 != 0 {
						e.Flags = progress.Fold
					}
					if b&16 != 0 {
						e.State = progress.StateQueued
					}
				})
				sent(id)
			case 1:
				id := pick(a)
				m.send(progress.TypeRun, id)
				sent(id)
			case 2:
				id := pick(a)
				m.send(progress.TypeAdvance, id, setWork)
				sent(id)
			case 3:
				id := pick(a)
				r, ok := m.send(progress.TypeEnd, id)
				sent(id)
				if k := spans[id]; k != nil {
					k.open = false
				}
				if ok {
					check(id, r)
				}
			default:
				m.now = m.now.Add(time.Duration(a) * 50 * time.Millisecond)
			}
			for _, id := range ids {
				k := spans[id]
				if !k.open {
					continue
				}
				r, ok := m.meter.Read(id, m.now)
				if ok {
					check(id, r)
				} else if k.unit != 0 {
					t.Fatalf("span %d has work in %s, but no reading", id, k.unit)
				}
			}
		}
	})
}
