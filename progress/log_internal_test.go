// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// Fields is embedded in Event, and the event log writes both flat, on one
// line: a member of Fields named as one of Event would be shadowed without
// a word, and a key of the log used twice would hide one of its values.
// Neither may happen.
func TestEventAndFieldsShareNoName(t *testing.T) {
	t.Parallel()
	names := map[string]string{}
	for _, typ := range []reflect.Type{reflect.TypeFor[Event](), reflect.TypeFor[Fields]()} {
		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.Anonymous {
				continue
			}
			if other, ok := names[f.Name]; ok {
				t.Errorf("%s.%s has the name of %s.%s", typ.Name(), f.Name, other, f.Name)
			}
			names[f.Name] = typ.Name()
		}
	}

	keys := map[string]string{}
	line := reflect.TypeFor[logEvent]()
	for i := range line.NumField() {
		f := line.Field(i)
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if key == "" || key == "-" {
			t.Errorf("logEvent.%s has no key of its own", f.Name)
			continue
		}
		if other, ok := keys[key]; ok {
			t.Errorf("logEvent.%s and logEvent.%s are both logged as %q", f.Name, other, key)
		}
		keys[key] = f.Name
	}
}

// heldLog is a Log whose timers for the advances it holds are started by
// hand: fire runs the i-th, and waits holds how long each was set to wait.
type heldLog struct {
	*Log
	out   *strings.Builder
	fires []func()
	waits []time.Duration
}

func newHeldLog(t *testing.T) *heldLog {
	t.Helper()
	h := &heldLog{out: &strings.Builder{}}
	h.Log = NewLog(h.out, LogOptions{RunID: "0123456789abcdef", AdvanceEvery: time.Second, FlushEvery: -1})
	h.after = func(d time.Duration, f func()) *time.Timer {
		h.waits = append(h.waits, d)
		h.fires = append(h.fires, f)
		return time.NewTimer(time.Hour)
	}
	return h
}

var heldAt = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

// handle feeds the log an event of span at ms milliseconds.
func (h *heldLog) handle(seq uint64, ms int, typ Type, span SpanID) {
	h.Handle(Event{Seq: seq, Time: heldAt.Add(time.Duration(ms) * time.Millisecond), Type: typ,
		Span: span, Kind: KindCall, Name: "download", Fields: Fields{Amount: int64(seq), Unit: UnitBytes}})
}

// lines closes the log and returns its lines as seq and leftOut pairs.
func (h *heldLog) lines(t *testing.T) [][2]int {
	t.Helper()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	var got [][2]int
	for line := range strings.Lines(h.out.String()) {
		var l struct{ Seq, LeftOut int }
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		got = append(got, [2]int{l.Seq, l.LeftOut})
	}
	return got
}

// An advance the log leaves out is held, and written once AdvanceEvery has
// passed since the last it wrote, after the lines of other spans written
// in the meantime: the amount a span stopped at is in the log before its
// next event, however long that takes to come.
func TestTheLogWritesTheAdvanceItHeldOnceItsPlaceComes(t *testing.T) {
	t.Parallel()
	h := newHeldLog(t)
	h.handle(1, 0, TypeAdvance, 7)
	h.handle(2, 100, TypeAdvance, 7)
	h.handle(3, 150, TypeStart, 8)
	if !slices.Equal(h.waits, []time.Duration{900 * time.Millisecond}) {
		t.Fatalf("the timers wait %v, want 900ms", h.waits)
	}
	h.fires[0]()
	h.handle(4, 1500, TypeAdvance, 7)
	h.handle(5, 1600, TypeEnd, 7)
	if got, want := h.lines(t), [][2]int{{1, 0}, {3, 0}, {2, 0}, {4, 0}, {5, 0}}; !slices.Equal(got, want) {
		t.Errorf("seq and leftOut of the lines = %v, want %v", got, want)
	}
}

// A newer event of the span makes the advance held old: it is left out
// after all, and counted, and its timer writes nothing.
func TestANewerEventOfTheSpanLeavesTheAdvanceHeldOut(t *testing.T) {
	t.Parallel()
	h := newHeldLog(t)
	h.handle(1, 0, TypeAdvance, 7)
	h.handle(2, 100, TypeAdvance, 7)
	h.handle(3, 200, TypeAdvance, 7)
	h.fires[0]()
	h.handle(4, 300, TypeUpdate, 7)
	h.fires[1]()
	h.handle(5, 400, TypeAdvance, 7)
	h.fires[2]()
	h.handle(6, 500, TypeEnd, 7)
	if got, want := h.lines(t), [][2]int{{1, 0}, {4, 0}, {5, 2}, {6, 2}}; !slices.Equal(got, want) {
		t.Errorf("seq and leftOut of the lines = %v, want %v", got, want)
	}
}

// Close writes the advances still held, of spans that never ended, and a
// timer that fires after it writes nothing more.
func TestCloseWritesTheAdvancesStillHeld(t *testing.T) {
	t.Parallel()
	h := newHeldLog(t)
	h.handle(1, 0, TypeAdvance, 7)
	h.handle(2, 100, TypeAdvance, 7)
	h.handle(3, 200, TypeAdvance, 7)
	got := h.lines(t)
	h.fires[1]()
	if want := [][2]int{{1, 0}, {3, 1}}; !slices.Equal(got, want) {
		t.Errorf("seq and leftOut of the lines = %v, want %v", got, want)
	}
	if n := strings.Count(h.out.String(), "\n"); n != 2 {
		t.Errorf("%d lines after the timer fired, want 2", n)
	}
}
