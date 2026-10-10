// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// eofReader gives all of its data in one read, with io.EOF, as some readers
// do at their end.
type eofReader struct{ data string }

func (r *eofReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, io.EOF
}

// CountWriter advances the span it is given by what each write took, a
// short write included, and gives back what the writer gave, unchanged.
func TestCountWriterAdvancesTheSpanByWhatEachWriteTook(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		w      io.Writer
		writes []string
		// wantN is what each write returns.
		wantN []int
		// wantErr says whether one of the writes fails.
		wantErr bool
		want    []work
	}{
		{"every write in full", &bytes.Buffer{}, []string{"0123456789", "abcdefghij", "klmnopqrst"},
			[]int{10, 10, 10}, false,
			[]work{{10, 100, progress.Bytes}, {20, 100, progress.Bytes}, {30, 100, progress.Bytes}}},
		{"an empty write advances nothing", &bytes.Buffer{}, []string{"", "abcd"},
			[]int{0, 4}, false, []work{{4, 100, progress.Bytes}}},
		{"a short write advances by what it took", failing{}, []string{"0123456789"},
			[]int{5}, true, []work{{5, 100, progress.Bytes}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := newClock()
			ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
			ctx, call := progress.Start(ctx, progress.KindCall, "copy", progress.Work(progress.Bytes, 100))
			w := progress.CountWriter(ctx, tc.w)
			var failed bool
			for i, p := range tc.writes {
				clock.Add(100 * time.Millisecond)
				n, err := w.Write([]byte(p))
				if n != tc.wantN[i] {
					t.Errorf("write %d returned %d, want %d", i, n, tc.wantN[i])
				}
				failed = failed || err != nil
			}
			if failed != tc.wantErr {
				t.Errorf("a write failed: %t, want %t", failed, tc.wantErr)
			}
			call.End(nil)
			if got := advances(capture); !equalWork(got, tc.want) {
				t.Errorf("advances %+v, want %+v", got, tc.want)
			}
		})
	}
}

// CountReader advances the span it is given by what each read gave, a read
// that also returns io.EOF included.
func TestCountReaderAdvancesTheSpanByWhatEachReadGave(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		r    io.Reader
		// size is the size of each read.
		size int
		want []work
	}{
		{"reads in pieces", strings.NewReader("0123456789"), 4,
			[]work{{4, 100, progress.Bytes}, {8, 100, progress.Bytes}, {10, 100, progress.Bytes}}},
		{"a read that gives bytes and io.EOF", &eofReader{"abc"}, 8,
			[]work{{3, 100, progress.Bytes}}},
		{"a read that gives nothing", strings.NewReader(""), 8, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := newClock()
			ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
			ctx, call := progress.Start(ctx, progress.KindCall, "read", progress.Work(progress.Bytes, 100))
			r := progress.CountReader(ctx, tc.r)
			buf := make([]byte, tc.size)
			for {
				clock.Add(100 * time.Millisecond)
				if _, err := r.Read(buf); errors.Is(err, io.EOF) {
					break
				} else if err != nil {
					t.Fatalf("read: %v", err)
				}
			}
			call.End(nil)
			if got := advances(capture); !equalWork(got, tc.want) {
				t.Errorf("advances %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The reader and the writer hand back the bytes and the error of what they
// wrap, unchanged.
func TestCountReturnsWhatItWrapsReturned(t *testing.T) {
	t.Parallel()
	ctx, _, _ := watched(t, progress.BusOptions{})
	ctx, call := progress.Start(ctx, progress.KindCall, "copy", progress.Work(progress.Bytes, 100))
	buf := make([]byte, 8)
	n, err := progress.CountReader(ctx, &eofReader{"abc"}).Read(buf)
	if n != 3 || !errors.Is(err, io.EOF) || string(buf[:n]) != "abc" {
		t.Errorf("read gave %d bytes %q and %v, want 3 bytes abc and io.EOF", n, buf[:n], err)
	}
	n, err = progress.CountWriter(ctx, failing{}).Write([]byte("0123"))
	if n != 2 || err == nil {
		t.Errorf("write gave %d bytes and %v, want 2 bytes and an error", n, err)
	}
	call.End(nil)
}

// A copy advances the innermost span only, the call the copy runs under; the
// step above it rolls the call's work up, and is not advanced itself.
func TestCountAdvancesTheInnermostSpanOnly(t *testing.T) {
	t.Parallel()
	clock := newClock()
	ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
	ctx, step := progress.Start(ctx, progress.KindStep, "copying the image", progress.Work(progress.Bytes, 100))
	ctx, call := progress.Start(ctx, progress.KindCall, "copy", progress.Work(progress.Bytes, 100))
	w := progress.CountWriter(ctx, &bytes.Buffer{})
	clock.Add(100 * time.Millisecond)
	if _, err := w.Write([]byte("0123456789")); err != nil {
		t.Fatalf("write: %v", err)
	}
	call.End(nil)
	step.End(nil)
	for _, e := range capture.Events() {
		if e.Type == progress.TypeAdvance && e.Kind != progress.KindCall {
			t.Errorf("an advance of a %s, want the call's only", e.Kind)
		}
	}
	if got, want := advances(capture), []work{{10, 100, progress.Bytes}}; !equalWork(got, want) {
		t.Errorf("advances %+v, want %+v", got, want)
	}
}

// A span that is queued or has ended advances nothing: a target counts what
// is written once it runs, and what is written after it ends counts for
// nothing.
func TestCountCountsOnlyWhileTheSpanRuns(t *testing.T) {
	t.Parallel()
	clock := newClock()
	ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
	ctx, target := progress.Start(ctx, progress.KindTarget, "exe0001", progress.Queued(),
		progress.Work(progress.Bytes, 100))
	w := progress.CountWriter(ctx, &bytes.Buffer{})
	if _, err := w.Write([]byte("0123")); err != nil {
		t.Fatalf("write: %v", err)
	}
	target.Run()
	clock.Add(100 * time.Millisecond)
	if _, err := w.Write([]byte("0123456")); err != nil {
		t.Fatalf("write: %v", err)
	}
	target.End(nil)
	clock.Add(100 * time.Millisecond)
	if _, err := w.Write([]byte("01")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, want := advances(capture), []work{{7, 100, progress.Bytes}}; !equalWork(got, want) {
		t.Errorf("advances %+v, want %+v", got, want)
	}
	if got, want := workOf(last(t, capture, progress.TypeEnd)), (work{7, 100, progress.Bytes}); got != want {
		t.Errorf("the end carries %+v, want %+v", got, want)
	}
}

// Without a span in ctx, CountWriter and CountReader return the writer and
// the reader they were given: with no Bus at all, with a Bus but no span, and
// with a span that belongs to another Bus than the one ctx carries.
func TestCountReturnsTheWriterAndReaderItselfWithoutASpan(t *testing.T) {
	t.Parallel()
	withBus, bus, _ := watched(t, progress.BusOptions{})
	otherBus, _, _ := watched(t, progress.BusOptions{})
	foreignCtx, _ := progress.Start(otherBus, progress.KindCall, "download")
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"no bus", context.Background()},
		{"a bus but no span", withBus},
		{"a span of another bus", progress.WithBus(foreignCtx, bus)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := &bytes.Buffer{}
			if got := progress.CountWriter(tc.ctx, w); got != io.Writer(w) {
				t.Errorf("CountWriter returned %T, want the writer itself", got)
			}
			r := strings.NewReader("abc")
			if got := progress.CountReader(tc.ctx, r); got != io.Reader(r) {
				t.Errorf("CountReader returned %T, want the reader itself", got)
			}
		})
	}
}

// Without a span, CountWriter and CountReader allocate nothing.
func TestCountWithoutASpanAllocatesNothing(t *testing.T) {
	ctx := context.Background()
	w := &bytes.Buffer{}
	r := strings.NewReader("abc")
	allocs := testing.AllocsPerRun(100, func() {
		_ = progress.CountWriter(ctx, w)
		_ = progress.CountReader(ctx, r)
	})
	if allocs != 0 {
		t.Errorf("CountWriter and CountReader without a span allocate %v times, want 0", allocs)
	}
}

// A copy through CountWriter ends with the bytes it moved: the End carries
// the newest amount, whatever the copy's own buffers did.
func TestACopyThroughCountEndsWithItsBytes(t *testing.T) {
	t.Parallel()
	clock := newClock()
	ctx, _, capture := watched(t, progress.BusOptions{Now: clock.Now})
	ctx, call := progress.Start(ctx, progress.KindCall, "copy", progress.Work(progress.Bytes, 26))
	var dst bytes.Buffer
	n, err := io.Copy(progress.CountWriter(ctx, &dst), strings.NewReader("abcdefghijklmnopqrstuvwxyz"))
	call.End(err)
	if err != nil || n != 26 {
		t.Fatalf("copy moved %d bytes, %v, want 26", n, err)
	}
	if got, want := workOf(last(t, capture, progress.TypeEnd)), (work{26, 26, progress.Bytes}); got != want {
		t.Errorf("the end carries %+v, want %+v", got, want)
	}
}
