// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-nodeset"
)

// A batch that ends is added to its step's names in place where the tree
// used to unite the two; the names read the same, padding, names of
// several numbers and names that are no node set included.
func TestMergingABatchReadsAsAUnionDid(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(3, 4))
	name := func() string {
		switch r.IntN(6) {
		case 0:
			return fmt.Sprintf("exe%04d", r.IntN(300))
		case 1:
			return fmt.Sprintf("exe%d", r.IntN(300))
		case 2:
			return fmt.Sprintf("r%dn%d", r.IntN(6), r.IntN(8))
		case 3:
			return fmt.Sprintf("r%02dn%03d-bmc", r.IntN(4), r.IntN(20))
		case 4:
			return fmt.Sprintf("a%db%dc%d", r.IntN(3), r.IntN(3), r.IntN(4))
		}
		return fmt.Sprintf("port %d", r.IntN(10))
	}
	for range 100 {
		var merged, united names
		for range 1 + r.IntN(8) {
			var batch names
			for range r.IntN(60) {
				batch.add(name())
			}
			merged.merge(&batch)
			if batch.set != nil {
				if united.set == nil {
					united.set = nodeset.New()
				}
				united.set = united.set.Union(batch.set)
			}
			united.other = append(united.other, batch.other...)
			united.stale = true
			if got, want := merged.String(), united.String(); got != want {
				t.Fatalf("merged %q, united %q", got, want)
			}
		}
	}
}

// go-nodeset reads back no more than 1,048,576 names at once; a batch of
// more is folded into its step all the same.
func TestABatchTooLargeToReadBackIsMerged(t *testing.T) {
	t.Parallel()
	var batch names
	batch.set = nodeset.MustParse("exe[1-1048576]")
	batch.add("bmc1")
	var step names
	step.add("login1")
	step.merge(&batch)
	if got, want := step.len(), 1048578; got != want {
		t.Errorf("the step holds %d names, want %d", got, want)
	}
	if got, want := step.String(), "bmc1,exe[1-1048576],login1"; got != want {
		t.Errorf("the step reads %q, want %q", got, want)
	}
}

// longest picks out the targets that get a row where the frame used to sort
// them all, by the tenth of a second each started in; the rows are the same,
// in the same order.
func TestLongestPicksWhatSortingFound(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	start := now.Add(-2 * time.Second)
	started := func(s *treeSpan) time.Duration { return s.ran.Sub(start) / (time.Second / 10) }
	r := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		running := make([]*treeSpan, r.IntN(40))
		for i := range running {
			// Few distinct tenths, so that many targets tie, each started
			// at an instant of its own within its tenth.
			ran := now.Add(-time.Duration(r.IntN(1500)) * time.Millisecond)
			running[i] = &treeSpan{id: progress.SpanID(i), ran: ran}
		}
		sorted := slices.Clone(running)
		slices.SortStableFunc(sorted, func(a, b *treeSpan) int {
			return cmp.Compare(started(a), started(b))
		})
		for k := 0; k <= len(running)+1; k++ {
			want := sorted[:min(k, len(sorted))]
			if got := longest(slices.Clone(running), k, start); !slices.Equal(got, want) {
				t.Fatalf("longest of %d, k=%d: %v, want %v", len(running), k, ids(got), ids(want))
			}
		}
	}
}

// longest orders the targets by the tenth of a second each started in,
// counted from the start of the display, whether it sorts them all or picks
// out the first k: one that started a tenth earlier comes first though
// queued later, within a second or across one, and those that started in
// the same tenth keep the order they were queued in, however far apart
// within it they started. The tenths are those of the time since the
// start, as Time.Sub reads it, on the monotonic clock where the times have
// one, and not those of the wall clock: a start between two tenths of the
// wall clock moves them.
func TestLongestOrdersByTenths(t *testing.T) {
	t.Parallel()
	noon := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	ms := func(at ...int) []time.Duration {
		out := make([]time.Duration, len(at))
		for i, n := range at {
			out[i] = time.Duration(n) * time.Millisecond
		}
		return out
	}
	for _, tc := range []struct {
		name string
		// at says when after noon each target, in the order they were
		// queued, started running, and start when the display started.
		at    []time.Duration
		start time.Duration
		k     int
		want  []int
	}{
		{"a tenth earlier within a second", ms(1300, 1200), 0, 2, []int{1, 0}},
		{"a tenth earlier across a second", ms(1000, 999), 0, 2, []int{1, 0}},
		{"started apart within the same tenth", ms(1590, 1510), 0, 2, []int{0, 1}},
		{"a tenth earlier, picked out", ms(1300, 1200, 2400), 0, 1, []int{1}},
		{"started apart within the same tenth, picked out", ms(1590, 1510, 2400), 0, 2, []int{0, 1}},
		{"all sorted", ms(2500, 1510, 1590, 1000, 1501), 0, 5, []int{3, 1, 2, 4, 0}},
		{"the first three picked out", ms(2500, 1510, 1590, 1000, 1501), 0, 3, []int{3, 1, 2}},
		// 0.09s and 0.11s after a start at 0.05s: the same tenth of the
		// wall clock, but not of the time since the start.
		{"a tenth of the time since the start", ms(160, 140), 50 * time.Millisecond, 2, []int{1, 0}},
		{"a tenth of the time since the start, picked out", ms(160, 140, 900), 50 * time.Millisecond, 1, []int{1}},
	} {
		running := make([]*treeSpan, len(tc.at))
		for i, at := range tc.at {
			running[i] = &treeSpan{id: progress.SpanID(i), ran: noon.Add(at)}
		}
		if got := ids(longest(running, tc.k, noon.Add(tc.start))); !slices.Equal(got, tc.want) {
			t.Errorf("%s: longest of %v, k=%d: %v, want %v", tc.name, tc.at, tc.k, got, tc.want)
		}
	}
}

// runTime reads how long a span has run in seconds and tenths, in minutes,
// seconds and tenths from a minute on, and in hours and minutes from an hour
// on. The tenths are cut, not rounded, so that it never runs ahead of the
// time: 999ms is still 0.9s, and 59.95s still short of the minute. A
// negative duration, of a clock that went back, reads as none.
func TestRunTime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Hour, "0.0s"},
		{-time.Nanosecond, "0.0s"},
		{0, "0.0s"},
		{99 * time.Millisecond, "0.0s"},
		{100 * time.Millisecond, "0.1s"},
		{999 * time.Millisecond, "0.9s"},
		{4200 * time.Millisecond, "4.2s"},
		{59950 * time.Millisecond, "59.9s"},
		{time.Minute - time.Nanosecond, "59.9s"},
		{time.Minute, "1m00.0s"},
		{3*time.Minute + 12490*time.Millisecond, "3m12.4s"},
		{3599990 * time.Millisecond, "59m59.9s"},
		{time.Hour, "1h00m"},
		{time.Hour + 2*time.Minute + 59900*time.Millisecond, "1h02m"},
		{25*time.Hour + 59*time.Minute, "25h59m"},
	} {
		if got := runTime(tc.d); got != tc.want {
			t.Errorf("runTime(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// seconds reads a pause's countdown and the bound of a request in whole
// seconds, in minutes and seconds from a minute on, and in hours and minutes
// from an hour on; a negative duration reads as none.
func TestSeconds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{-time.Second, "0s"},
		{999 * time.Millisecond, "0s"},
		{12 * time.Second, "12s"},
		{time.Minute - time.Nanosecond, "59s"},
		{time.Minute, "1m00s"},
		{3*time.Minute + 12900*time.Millisecond, "3m12s"},
		{time.Hour - time.Nanosecond, "59m59s"},
		{time.Hour, "1h00m"},
		{time.Hour + 2*time.Minute + 59*time.Second, "1h02m"},
	} {
		if got := seconds(tc.d); got != tc.want {
			t.Errorf("seconds(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func ids(spans []*treeSpan) []int {
	out := make([]int, len(spans))
	for i, s := range spans {
		out[i] = int(s.id)
	}
	return out
}
