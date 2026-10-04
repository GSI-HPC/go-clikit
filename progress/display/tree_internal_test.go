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
// them all; the rows are the same, in the same order.
func TestLongestPicksWhatSortingFound(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	r := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		running := make([]*treeSpan, r.IntN(40))
		for i := range running {
			// Few distinct seconds, so that many targets tie.
			ran := now.Add(-time.Duration(r.IntN(5000)) * time.Millisecond)
			running[i] = &treeSpan{id: progress.SpanID(i), ran: ran}
		}
		sorted := slices.Clone(running)
		slices.SortStableFunc(sorted, func(a, b *treeSpan) int {
			return cmp.Compare(now.Sub(b.ran)/time.Second, now.Sub(a.ran)/time.Second)
		})
		for k := 0; k <= len(running)+1; k++ {
			want := sorted[:min(k, len(sorted))]
			if got := longest(slices.Clone(running), k, now); !slices.Equal(got, want) {
				t.Fatalf("longest of %d, k=%d: %v, want %v", len(running), k, ids(got), ids(want))
			}
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
