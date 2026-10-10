// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// work is what an event says of its span's work.
type work struct {
	amount, size int64
	unit         progress.Unit
}

func workOf(e progress.Event) work { return work{e.Amount, e.Size, e.Unit} }

// advances returns the work of every TypeAdvance the capture was sent.
func advances(c *progresstest.Capture) []work {
	var out []work
	for _, e := range c.Events() {
		if e.Type == progress.TypeAdvance {
			out = append(out, workOf(e))
		}
	}
	return out
}

// last returns the last event the capture was sent of type typ.
func last(t *testing.T, c *progresstest.Capture, typ progress.Type) progress.Event {
	t.Helper()
	events := c.Events()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == typ {
			return events[i]
		}
	}
	t.Fatalf("no %s among %d events", typ, len(events))
	return progress.Event{}
}

// Work gives a span its unit and size when it starts, and an update gives
// the size once it is known or changes it; the unit once given stays, a
// size of 0 or less is one not known, and the size of work in Percent is
// always 100.
func TestWorkGivesAUnitThatStaysAndASize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		start, update []progress.Option
		want          work
	}{
		{"none", nil, nil, work{}},
		{"given at the start", []progress.Option{progress.Work(progress.Bytes, 2048)}, nil, work{0, 2048, progress.Bytes}},
		{"a size not known", []progress.Option{progress.Work(progress.Items, -1)}, nil, work{0, 0, progress.Items}},
		{"a zero unit counts items", []progress.Option{progress.Work(0, 10)}, nil, work{0, 10, progress.Items}},
		{"percent is out of 100", []progress.Option{progress.Work(progress.Percent, 0)}, nil, work{0, 100, progress.Percent}},
		{"the size given late", nil, []progress.Option{progress.Work(progress.Bytes, 4096)}, work{0, 4096, progress.Bytes}},
		{"the size changes, the unit stays",
			[]progress.Option{progress.Work(progress.Bytes, 4096)},
			[]progress.Option{progress.Work(progress.Items, 8192)}, work{0, 8192, progress.Bytes}},
		{"the size becomes unknown",
			[]progress.Option{progress.Work(progress.Bytes, 4096)},
			[]progress.Option{progress.Work(progress.Bytes, 0)}, work{0, 0, progress.Bytes}},
		{"percent stays out of 100",
			[]progress.Option{progress.Work(progress.Percent, 100)},
			[]progress.Option{progress.Work(progress.Bytes, 7)}, work{0, 100, progress.Percent}},
		{"an update without work keeps it",
			[]progress.Option{progress.Work(progress.Bytes, 4096)},
			[]progress.Option{progress.Message("halfway")}, work{0, 4096, progress.Bytes}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, _, capture := watched(t, progress.BusOptions{})
			_, call := progress.Start(ctx, progress.KindCall, "download", tc.start...)
			if tc.update != nil {
				call.Update(tc.update...)
			}
			call.End(nil)
			if got := workOf(last(t, capture, progress.TypeEnd)); got != tc.want {
				t.Errorf("the span ends with %+v, want %+v", got, tc.want)
			}
		})
	}
}

// End takes Work as it takes every option that sets a field, under the
// same rules.
func TestEndTakesWork(t *testing.T) {
	t.Parallel()
	ctx, _, capture := watched(t, progress.BusOptions{})
	_, call := progress.Start(ctx, progress.KindCall, "download", progress.Work(progress.Bytes, 0))
	call.End(nil, progress.Work(progress.Items, 512))
	if got, want := workOf(last(t, capture, progress.TypeEnd)), (work{0, 512, progress.Bytes}); got != want {
		t.Errorf("the span ends with %+v, want %+v", got, want)
	}
}

// The Bus sends a span's amount at most once each 100 ms by its clock: an
// advance before then is sent once its place comes, and the newest amount
// is never lost, since the End carries it. Every event of the span from the
// first advance carries the amount sent last.
func TestAnAdvanceIsSentAtMostEvery100msByTheBusClock(t *testing.T) {
	t.Parallel()
	clock := newClock()
	ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
	_, call := progress.Start(ctx, progress.KindCall, "download", progress.Work(progress.Bytes, 100))
	call.Advance(10)
	call.Advance(10)
	clock.Add(50 * time.Millisecond)
	call.Advance(10)
	call.Update(progress.Message("halfway"))
	if got, want := workOf(last(t, capture, progress.TypeUpdate)), (work{10, 100, progress.Bytes}); got != want {
		t.Errorf("the update carries %+v, want the amount sent last, %+v", got, want)
	}
	clock.Add(50 * time.Millisecond)
	call.Advance(10)
	call.Advance(5)
	call.End(nil)
	want := []work{{10, 100, progress.Bytes}, {40, 100, progress.Bytes}}
	if got := advances(capture); !equalWork(got, want) {
		t.Errorf("advances %+v, want %+v", got, want)
	}
	end := last(t, capture, progress.TypeEnd)
	if got, want := workOf(end), (work{45, 100, progress.Bytes}); got != want {
		t.Errorf("the end carries %+v, want the newest amount, %+v", got, want)
	}
	var times []time.Time
	for _, e := range capture.Events() {
		if e.Type == progress.TypeAdvance {
			times = append(times, e.Time)
		}
	}
	if d := times[1].Sub(times[0]); d < 100*time.Millisecond {
		t.Errorf("two advances %s apart, want 100ms at least", d)
	}
}

func equalWork(a, b []work) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A span given no Work counts Items, and SetAmount sets the amount rather
// than adding to it, down as well as up; one below 0 is sent as 0, and one
// that does not change the amount sends nothing.
func TestSetAmountSetsTheAmountOfASpanThatCountsItems(t *testing.T) {
	t.Parallel()
	clock := newClock()
	ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
	_, step := progress.Start(ctx, progress.KindStep, "indexing")
	step.SetAmount(70)
	clock.Add(100 * time.Millisecond)
	step.SetAmount(70)
	step.SetAmount(30)
	clock.Add(100 * time.Millisecond)
	step.Advance(-50)
	step.End(nil)
	want := []work{{70, 0, progress.Items}, {30, 0, progress.Items}, {0, 0, progress.Items}}
	if got := advances(capture); !equalWork(got, want) {
		t.Errorf("advances %+v, want %+v", got, want)
	}
}

// Work moves only while its span runs: an advance of a queued span or of
// one that ended does nothing, and neither does one of a nil span.
func TestOnlyARunningSpanAdvances(t *testing.T) {
	t.Parallel()
	clock := newClock()
	ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
	_, target := progress.Start(ctx, progress.KindTarget, "exe0001", progress.Queued(), progress.Work(progress.Bytes, 10))
	target.Advance(5)
	target.SetAmount(5)
	clock.Add(time.Second)
	target.Run()
	target.Advance(1)
	target.End(nil)
	clock.Add(time.Second)
	target.Advance(1)
	target.SetAmount(9)
	var none *progress.Span
	none.Advance(1)
	none.SetAmount(1)
	if got, want := advances(capture), []work{{1, 10, progress.Bytes}}; !equalWork(got, want) {
		t.Errorf("advances %+v, want %+v", got, want)
	}
	if got, want := workOf(last(t, capture, progress.TypeEnd)), (work{1, 10, progress.Bytes}); got != want {
		t.Errorf("the end carries %+v, want %+v", got, want)
	}
}

// An amount that waits for its place is sent by a timer once it comes, by
// the Bus's clock: a timer that fires before then waits again, and one that
// finds the amount sent already sends nothing.
func TestAnAmountThatWaitsIsSentWhenItsPlaceComes(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		clock := newClock()
		ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
		_, call := progress.Start(ctx, progress.KindCall, "download")
		call.Advance(1)
		call.Advance(1)
		// The timer fires twice, and the Bus's clock has not moved.
		time.Sleep(250 * time.Millisecond)
		synctest.Wait()
		if got := advances(capture); len(got) != 1 {
			t.Errorf("%d advances sent before their place came, want 1", len(got))
		}
		clock.Add(100 * time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		if got, want := advances(capture), []work{{1, 0, progress.Items}, {2, 0, progress.Items}}; !equalWork(got, want) {
			t.Errorf("advances %+v, want %+v", got, want)
		}
		// One sent by an advance before its timer fires leaves the timer
		// nothing to send.
		call.Advance(1)
		clock.Add(100 * time.Millisecond)
		call.Advance(1)
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		if got := advances(capture); len(got) != 3 || got[2].amount != 4 {
			t.Errorf("advances %+v, want a third of 4", got)
		}
		call.End(nil)
	})
}

// With the clock the Bus has by default, an advance before its place is
// sent 100 ms after the one before.
func TestAnAdvanceWaitsForItsPlaceByTheTimeOfDay(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, _, capture := watched(t, progress.BusOptions{})
		_, call := progress.Start(ctx, progress.KindCall, "download", progress.Work(progress.Bytes, 0))
		call.Advance(1)
		time.Sleep(10 * time.Millisecond)
		call.Advance(1)
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		events := capture.Events()
		var times []time.Time
		for _, e := range events {
			if e.Type == progress.TypeAdvance {
				times = append(times, e.Time)
			}
		}
		if len(times) != 2 || times[1].Sub(times[0]) != 100*time.Millisecond {
			t.Errorf("advances at %v, want two 100ms apart", times)
		}
		call.End(nil)
	})
}

// A clock that moves back, as one a test sets may, keeps an amount waiting
// no longer than 100 ms at a time, and sends it once the clock has caught
// up.
func TestAClockThatMovesBackDelaysNoAdvanceForLong(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		clock := newClock()
		ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
		_, call := progress.Start(ctx, progress.KindCall, "download")
		call.Advance(1)
		clock.Add(-time.Hour)
		call.Advance(1)
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		if got := advances(capture); len(got) != 1 {
			t.Errorf("%d advances sent before the clock caught up, want 1", len(got))
		}
		clock.Add(time.Hour + 100*time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
		if got := advances(capture); len(got) != 2 {
			t.Errorf("%d advances sent once the clock caught up, want 2", len(got))
		}
		call.End(nil)
	})
}

// A span whose first advance could not be sent before it ended, by a clock
// that reads a time before 1970, ends with the amount, in Items when it was
// given no Work.
func TestAnAmountNeverSentIsOnTheEnd(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		clock := &clock{now: time.Date(1960, 1, 1, 0, 0, 0, 0, time.UTC)}
		ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
		_, call := progress.Start(ctx, progress.KindCall, "download")
		call.Advance(3)
		call.End(nil)
		if got := advances(capture); len(got) != 0 {
			t.Errorf("advances %+v, want none", got)
		}
		if got, want := workOf(last(t, capture, progress.TypeEnd)), (work{3, 0, progress.Items}); got != want {
			t.Errorf("the end carries %+v, want %+v", got, want)
		}
	})
}

// A span that ends because its parent or the Bus does carries its newest
// amount too.
func TestASpanEndedForItCarriesItsAmount(t *testing.T) {
	t.Parallel()
	clock := newClock()
	ctx, bus, capture := watched(t, progress.BusOptions{Now: clock.Now})
	_, call := progress.Start(ctx, progress.KindCall, "download", progress.Work(progress.Bytes, 10))
	call.Advance(2)
	call.Advance(2)
	bus.Close()
	end := last(t, capture, progress.TypeEnd)
	if got, want := workOf(end), (work{4, 10, progress.Bytes}); got != want || end.Status != progress.StatusCanceled {
		t.Errorf("the end carries %+v and %s, want %+v and canceled", got, end.Status, want)
	}
}

// TestWithoutABusAdvanceAllocatesNothing holds the work of a span nobody
// watches to the promise of the rest: Work, Advance and SetAmount cost no
// allocation.
func TestWithoutABusAdvanceAllocatesNothing(t *testing.T) {
	ctx := context.Background()
	allocs := testing.AllocsPerRun(100, func() {
		_, call := progress.Start(ctx, progress.KindCall, "download", progress.Work(progress.Bytes, 2<<30))
		call.Update(progress.Work(progress.Bytes, 4<<30))
		call.Advance(8 << 20)
		call.SetAmount(16 << 20)
		call.End(nil)
	})
	if allocs != 0 {
		t.Errorf("a span's work without a Bus allocates %v times, want 0", allocs)
	}
}

func BenchmarkAdvance(b *testing.B) {
	b.Run("without a bus", func(b *testing.B) {
		_, call := progress.Start(context.Background(), progress.KindCall, "download")
		for b.Loop() {
			call.Advance(4096)
		}
	})
	b.Run("with a bus", func(b *testing.B) {
		bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{discard{}}})
		defer bus.Close()
		_, call := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCall, "download",
			progress.Work(progress.Bytes, 0))
		defer call.End(nil)
		for b.Loop() {
			call.Advance(4096)
		}
	})
	b.Run("with a bus, in parallel", func(b *testing.B) {
		bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{discard{}}})
		defer bus.Close()
		_, call := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCall, "download",
			progress.Work(progress.Bytes, 0))
		defer call.End(nil)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				call.Advance(4096)
			}
		})
	})
}
