// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// An amount reads in IEC units for bytes, one decimal below 100 and none
// from 100, rounded, and in a larger unit once it would read 1,000; items
// read whole below 10,000, then with SI prefixes alike. Past the largest
// unit the number grows.
func TestAmountText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		u    progress.Unit
		v    float64
		want string
	}{
		{progress.Bytes, 0, "0 B"},
		{progress.Bytes, 512, "512 B"},
		{progress.Bytes, 999, "999 B"},
		{progress.Bytes, 1000, "1.0 KiB"},
		{progress.Bytes, 85.3 * (1 << 20), "85.3 MiB"},
		{progress.Bytes, 99.94 * (1 << 20), "99.9 MiB"},
		{progress.Bytes, 99.96 * (1 << 20), "100 MiB"},
		{progress.Bytes, 312 << 20, "312 MiB"},
		{progress.Bytes, 999.4 * (1 << 20), "999 MiB"},
		{progress.Bytes, 999.6 * (1 << 20), "1.0 GiB"},
		{progress.Bytes, 1.2 * (1 << 30), "1.2 GiB"},
		{progress.Bytes, 3 << 40, "3.0 TiB"},
		{progress.Bytes, 2048 << 40, "2048 TiB"},
		{progress.Items, 9312, "9312"},
		{progress.Items, 9999, "9999"},
		{progress.Items, 10000, "10.0k"},
		{progress.Items, 48200, "48.2k"},
		{progress.Items, 312000, "312k"},
		{progress.Items, 999600, "1.0M"},
		{progress.Items, 1.2e6, "1.2M"},
		{progress.Items, 5e15, "5000T"},
		{progress.Unit(9), 12, "12"},
	} {
		if got := amountText(tc.u, tc.v); got != tc.want {
			t.Errorf("amountText(%v, %v) = %q, want %q", tc.u, tc.v, got, tc.want)
		}
	}
}

// An amount out of a size reads both in the size's unit.
func TestOutOfText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		u            progress.Unit
		amount, size int64
		want         string
	}{
		{progress.Bytes, 512 << 20, 2 << 30, "0.5/2.0 GiB"},
		{progress.Bytes, 2254857830, 2 << 30, "2.1/2.0 GiB"},
		{progress.Bytes, 85 << 20, 312 << 20, "85.0/312 MiB"},
		{progress.Bytes, 100, 512, "100/512 B"},
		{progress.Items, 48200, 50000, "48.2/50.0k"},
		{progress.Items, 312, 480, "312/480"},
		{progress.Items, 0, 0, "0/0"},
	} {
		if got := outOfText(tc.u, tc.amount, tc.size); got != tc.want {
			t.Errorf("outOfText(%v, %d, %d) = %q, want %q", tc.u, tc.amount, tc.size, got, tc.want)
		}
	}
}

// A rate reads in the form of an amount with /s, with one decimal below 10,
// and in a larger unit once it would read 1,000, items as bytes.
func TestRateText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		u    progress.Unit
		v    float64
		want string
	}{
		{progress.Items, 0, "0.0/s"},
		{progress.Items, 0.4, "0.4/s"},
		{progress.Items, 9.94, "9.9/s"},
		{progress.Items, 9.96, "10/s"},
		{progress.Items, 312, "312/s"},
		{progress.Items, 999.4, "999/s"},
		{progress.Items, 999.5, "1.0k/s"},
		{progress.Items, 6600, "6.6k/s"},
		{progress.Bytes, 0.4, "0.4 B/s"},
		{progress.Bytes, 512, "512 B/s"},
		{progress.Bytes, 999.5, "1.0 KiB/s"},
		{progress.Bytes, 85.3 * (1 << 20), "85.3 MiB/s"},
		{progress.Bytes, 797 << 20, "797 MiB/s"},
	} {
		if got := rateText(tc.u, tc.v); got != tc.want {
			t.Errorf("rateText(%v, %v) = %q, want %q", tc.u, tc.v, got, tc.want)
		}
	}
}

// A share reads as a whole percentage, cut, not rounded, so that only work
// done reads 100%; what float arithmetic leaves short of a percentage is
// not cut, but a share short of 1 never reads 100%. The time left is
// rounded up to the second below an hour and to the minute from it, and how
// long work has stalled ticks in tenths.
func TestShareLeftAndStalled(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		f    float64
		want string
	}{
		{0, "0%"}, {0.009, "0%"}, {0.5625, "56%"}, {0.57, "57%"}, {0.29, "29%"}, {0.999, "99%"},
		{0.99999999999, "99%"}, {1, "100%"},
	} {
		if got := shareText(tc.f); got != tc.want {
			t.Errorf("shareText(%v) = %q, want %q", tc.f, got, tc.want)
		}
	}
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{time.Millisecond, "~1s left"},
		{11200 * time.Millisecond, "~12s left"},
		{12 * time.Second, "~12s left"},
		{7*time.Minute + 2500*time.Millisecond, "~7m03s left"},
		{time.Hour + 2*time.Minute, "~1h02m left"},
		{time.Hour + time.Minute + 59*time.Second, "~1h02m left"},
		{time.Hour + 61*time.Second, "~1h02m left"},
		{59*time.Minute + 59500*time.Millisecond, "~1h00m left"},
	} {
		if got := leftText(tc.d); got != tc.want {
			t.Errorf("leftText(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
	if got := stalledText(6190 * time.Millisecond); got != "stalled 6.1s" {
		t.Errorf("stalledText = %q", got)
	}
}

// A row's work gives up, in order, the time left, the rate, the amount and
// the bar; work in Percent draws no amount or rate, and work that is
// unbounded no share, bar or time left. Stalled work says so in place of
// its rate.
func TestWorkText(t *testing.T) {
	t.Parallel()
	bounded := progress.Reading{Unit: progress.Bytes, Amount: 1152 << 20, Size: 2 << 30, Bound: progress.ByBelow,
		Fraction: 0.5625, Rate: 80 << 20, Left: 11200 * time.Millisecond}
	ascii := lookOf(Tide.In(NoColours), true)
	for _, tc := range []struct {
		name   string
		g      look
		r      progress.Reading
		giveUp int
		want   string
	}{
		{"whole", unicodeLook, bounded, 0, "1.1/2.0 GiB · 56% · 80.0 MiB/s · ~12s left"},
		{"no time left", unicodeLook, bounded, 1, "1.1/2.0 GiB · 56% · 80.0 MiB/s"},
		{"no rate", unicodeLook, bounded, 2, "1.1/2.0 GiB · 56%"},
		{"no amount", unicodeLook, bounded, 3, "56%"},
		{"a bar", ascii, bounded, 0, "[###...] 1.1/2.0 GiB - 56% - 80.0 MiB/s - ~12s left"},
		{"the bar last", ascii, bounded, 3, "[###...] 56%"},
		{"no bar", ascii, bounded, 4, "56%"},
		{"percent", ascii, progress.Reading{Unit: progress.Percent, Amount: 40, Size: 100, Bound: progress.BySize,
			Fraction: 0.4, Rate: 2, Left: 32 * time.Second, Stalled: 6 * time.Second}, 0, "[##....] 40% - ~32s left"},
		{"unbounded", ascii, progress.Reading{Unit: progress.Items, Amount: 48200, Rate: 6600}, 0, "48.2k - 6.6k/s"},
		{"unbounded, given up", ascii, progress.Reading{Unit: progress.Items, Amount: 48200, Rate: 6600}, 3, ""},
		{"stalled", unicodeLook, progress.Reading{Unit: progress.Bytes, Amount: 312 << 20, Stalled: 6100 * time.Millisecond},
			0, "312 MiB · stalled 6.1s"},
	} {
		if got := workOf(tc.g, tc.r, workBar).text(tc.g.sep, tc.giveUp); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	// In a theme the amount is muted, and stalled in the colour of a
	// cancellation.
	const muted, canceled = "\x1b[38;5;66m", "\x1b[38;5;103m"
	tide := lookOf(Tide, false)
	stalled := progress.Reading{Unit: progress.Bytes, Amount: 312 << 20, Stalled: 6100 * time.Millisecond}
	if got, want := workOf(tide, stalled, workBar).text(" ", 0), muted+"312 MiB"+reset+" "+canceled+"stalled 6.1s"+reset; got != want {
		t.Errorf("stalled in Tide: %q, want %q", got, want)
	}
}

// What the work of a span came to reads as its amount and its mean rate,
// the amount alone when the span took no time, and nothing in Percent.
func TestEndedText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		r    progress.Reading
		want string
	}{
		{progress.Reading{Unit: progress.Bytes, Amount: 2 << 30, Rate: 80 << 20}, "2.0 GiB at 80.0 MiB/s"},
		{progress.Reading{Unit: progress.Bytes, Amount: 1536}, "1.5 KiB"},
		{progress.Reading{Unit: progress.Percent, Amount: 100, Rate: 4}, ""},
	} {
		if got := endedText(unicodeLook, tc.r); got != tc.want {
			t.Errorf("endedText(%+v) = %q, want %q", tc.r, got, tc.want)
		}
	}
	const muted = "\x1b[38;5;66m"
	if got, want := endedText(lookOf(Tide, false), progress.Reading{Unit: progress.Items, Amount: 43, Rate: 21.5}),
		muted+"43"+reset+" at "+muted+"22/s"+reset; got != want {
		t.Errorf("in Tide: %q, want %q", got, want)
	}
}

// A bar of a share fills with it, cut down to whole cells and never full
// short of done, with the cells of the targets that failed at its end; none
// where the look has no bar.
func TestFractionBar(t *testing.T) {
	t.Parallel()
	shape := lookOf(Tide.In(NoColours), false)
	for _, tc := range []struct {
		name string
		f    float64
		c    progress.Count
		want string
	}{
		{"none", 0, progress.Count{}, "◌◌◌◌◌◌"},
		{"half", 0.5, progress.Count{}, "◉◉◉◌◌◌"},
		{"what floats leave short of a cell", 0.49999999999, progress.Count{}, "◉◉◉◌◌◌"},
		{"all", 1, progress.Count{}, "◉◉◉◉◉◉"},
		{"what floats leave short of all", 1 - 1e-11, progress.Count{}, "◉◉◉◉◉◌"},
		{"a share past the targets done", 0.5, progress.Count{Done: 1, Total: 6}, "◉◉◉◌◌◌"},
		{"failures at its end", 0.5, progress.Count{Done: 2, Failed: 1, Total: 6}, "◉◉◉◌◌◌"},
		{"failures past the share", 0, progress.Count{Done: 2, Failed: 2, Total: 6}, "◉◉◌◌◌◌"},
	} {
		if got := shape.fractionBar(tc.f, tc.c, 6); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := unicodeLook.fractionBar(0.5, progress.Count{}, 6); got != "" {
		t.Errorf("no theme draws a bar: %q", got)
	}
	const blue, orange, slate = "\x1b[38;5;32m", "\x1b[38;5;166m", "\x1b[38;5;66m"
	if got, want := lookOf(Tide, false).fractionBar(0.5, progress.Count{Done: 2, Failed: 1, Total: 6}, 6),
		blue+"◉◉"+reset+orange+"◉"+reset+slate+"◌◌◌"+reset; got != want {
		t.Errorf("in Tide: %q, want %q", got, want)
	}
}
