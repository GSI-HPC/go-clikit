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
	"testing/iotest"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// A traceparent is taken only as W3C writes it, and anything else is no
// trace context at all; the tracestate is kept as it is when it can be.
func TestParseTraceContext(t *testing.T) {
	t.Parallel()

	const (
		trace  = "4bf92f3577b34da6a3ce929d0e0e4736"
		parent = "00f067aa0ba902b7"
		valid  = "00-" + trace + "-" + parent + "-01"
		state  = "rojo=00f067aa0ba902b7,congo=t61rcWkgMzE"
	)
	tests := []struct {
		name         string
		header, st   string
		ok           bool
		flags        byte
		wantState    string
		wantTrace    string
		wantParentID string
	}{
		{"sampled", valid, "", true, 0x01, "", trace, parent},
		{"not sampled", "00-" + trace + "-" + parent + "-00", "", true, 0x00, "", trace, parent},
		{"flags not yet defined are kept", "00-" + trace + "-" + parent + "-ff", "", true, 0xff, "", trace, parent},
		{"with a tracestate", valid, state, true, 0x01, state, trace, parent},
		{"a later version, with more after it", "01-" + trace + "-" + parent + "-01-what-comes-next", "", true, 0x01, "", trace, parent},
		{"a later version, just as long", "cc-" + trace + "-" + parent + "-01", "", true, 0x01, "", trace, parent},

		{"empty", "", "", false, 0, "", "", ""},
		{"version ff", "ff-" + trace + "-" + parent + "-01", "", false, 0, "", "", ""},
		{"version 00 with more after it", valid + "-00", "", false, 0, "", "", ""},
		{"a later version with no dash after the flags", "01-" + trace + "-" + parent + "-01x", "", false, 0, "", "", ""},
		{"a trace id of zeros", "00-" + strings.Repeat("0", 32) + "-" + parent + "-01", "", false, 0, "", "", ""},
		{"a span id of zeros", "00-" + trace + "-" + strings.Repeat("0", 16) + "-01", "", false, 0, "", "", ""},
		{"upper case", "00-" + strings.ToUpper(trace) + "-" + parent + "-01", "", false, 0, "", "", ""},
		{"upper case flags", "00-" + trace + "-" + parent + "-0A", "", false, 0, "", "", ""},
		{"a digit that is not hexadecimal", "0g-" + trace + "-" + parent + "-01", "", false, 0, "", "", ""},
		{"too short", "00-" + trace[1:] + "-" + parent + "-01", "", false, 0, "", "", ""},
		{"another separator", "00_" + trace + "_" + parent + "_01", "", false, 0, "", "", ""},
		{"a space in front", " " + valid, "", false, 0, "", "", ""},
		{"a newline after it", valid + "\n", "", false, 0, "", "", ""},
		{"a tracestate without a valid traceparent", "00-" + trace + "-" + parent, state, false, 0, "", "", ""},

		{"a tracestate with a control character is left out", valid, "rojo=1\x1b[2J", true, 0x01, "", trace, parent},
		{"a tracestate with a tab around a comma is kept", valid, "rojo=1,\tcongo=2", true, 0x01, "rojo=1,\tcongo=2", trace, parent},
		{"a tracestate that is not ASCII is left out", valid, "rojo=é", true, 0x01, "", trace, parent},
		{"a tracestate too long is left out", valid, "rojo=" + strings.Repeat("a", 508), true, 0x01, "", trace, parent},
		{"a tracestate of 512 bytes is kept", valid, "rojo=" + strings.Repeat("a", 507), true, 0x01, "rojo=" + strings.Repeat("a", 507), trace, parent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := progress.ParseTraceContext(tc.header, tc.st)
			if ok != tc.ok {
				t.Fatalf("ParseTraceContext(%q) ok = %v, want %v", tc.header, ok, tc.ok)
			}
			if !ok {
				if got != (progress.TraceContext{}) {
					t.Errorf("an invalid traceparent gave %+v, want nothing", got)
				}
				return
			}
			if got.Trace.String() != tc.wantTrace || got.Parent.String() != tc.wantParentID {
				t.Errorf("trace %s, parent %s; want %s and %s", got.Trace, got.Parent, tc.wantTrace, tc.wantParentID)
			}
			if got.Flags != tc.flags || got.State != tc.wantState {
				t.Errorf("flags %02x and state %q, want %02x and %q", got.Flags, got.State, tc.flags, tc.wantState)
			}
		})
	}
}

// Runs that continue one trace, as the commands of one CI step do from its
// TRACEPARENT, share its trace id and none of their span ids, and each
// says where the trace came from. A parent without a trace is none.
func TestRunsThatContinueOneTraceShareNoSpanID(t *testing.T) {
	t.Parallel()

	tc, ok := progress.ParseTraceContext("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "rojo=1")
	if !ok {
		t.Fatal("the traceparent was refused")
	}
	seen := map[progress.SpanID]int{}
	for run := range 2 {
		capture := &progresstest.Capture{}
		bus := progress.NewBus(progress.BusOptions{
			Sinks: []progress.Sink{capture}, Trace: tc,
		})
		if got := bus.TraceContext(); got != tc {
			t.Errorf("run %d continues %+v, want %+v", run+1, got, tc)
		}
		ctx := progress.WithBus(context.Background(), bus)
		ctx, cmd := progress.Start(ctx, progress.KindCommand, "exec")
		for range 1000 {
			_, s := progress.Start(ctx, progress.KindCall, "ssh")
			s.End(nil)
		}
		cmd.End(nil)
		bus.Close()
		for _, e := range capture.Events() {
			if e.Type != progress.TypeStart {
				continue
			}
			if other, ok := seen[e.Span]; ok {
				t.Fatalf("span id %s is run %d's and run %d's", e.Span, other+1, run+1)
			}
			seen[e.Span] = run
		}
	}

	bus := progress.NewBus(progress.BusOptions{Trace: progress.TraceContext{Parent: tc.Parent, Flags: 1, State: "rojo=1"}})
	if got := bus.TraceContext(); got.Trace == (progress.TraceID{}) || got.Trace == tc.Trace || got.Parent != 0 || got.State != "" {
		t.Errorf("a parent without a trace gave %+v, want a trace of its own and nothing else", got)
	}
}

// A new trace context takes its ids from the source it is given, in order,
// and is sampled, with no tracestate; its traceparent is the W3C one for
// them, which ParseTraceContext reads back.
func TestNewTraceContext(t *testing.T) {
	t.Parallel()

	const (
		trace  = "4bf92f3577b34da6a3ce929d0e0e4736"
		parent = "00f067aa0ba902b7"
	)
	raw := []byte{
		0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36,
		0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7,
		0xde, 0xad,
	}
	src := bytes.NewReader(raw)
	tc, err := progress.NewTraceContext(src)
	if err != nil {
		t.Fatalf("NewTraceContext: %v", err)
	}
	if tc.Trace.String() != trace || tc.Parent.String() != parent || tc.Flags != 0x01 || tc.State != "" {
		t.Errorf("got trace %s, parent %s, flags %02x, state %q; want %s, %s, 01 and none", tc.Trace, tc.Parent, tc.Flags, tc.State, trace, parent)
	}
	if src.Len() != 2 {
		t.Errorf("%d bytes left in the source, want 2: exactly 24 are read", src.Len())
	}
	want := "00-" + trace + "-" + parent + "-01"
	if got := tc.Traceparent(); got != want {
		t.Errorf("Traceparent() = %q, want %q", got, want)
	}
	back, ok := progress.ParseTraceContext(tc.Traceparent(), "")
	if !ok || back != tc {
		t.Errorf("ParseTraceContext(%q) = %+v, %v; want %+v", tc.Traceparent(), back, ok, tc)
	}
}

// A source that gives fewer than 24 bytes, fails, or gives an id of zeros
// makes no trace context, and a source that gives its bytes a few at a time
// is read until there are 24.
func TestNewTraceContextSources(t *testing.T) {
	t.Parallel()

	ones := bytes.Repeat([]byte{0xff}, 24)
	tests := []struct {
		name    string
		src     io.Reader
		wantErr error
		wantMsg string
	}{
		{"empty", bytes.NewReader(nil), io.EOF, ""},
		{"one byte short", bytes.NewReader(ones[:23]), io.ErrUnexpectedEOF, ""},
		{"a read that fails", iotest.ErrReader(iotest.ErrTimeout), iotest.ErrTimeout, ""},
		{"all zeros", bytes.NewReader(make([]byte, 24)), nil, "zeros"},
		{"a trace id of zeros", bytes.NewReader(append(make([]byte, 16), ones[:8]...)), nil, "zeros"},
		{"a span id of zeros", bytes.NewReader(append(bytes.Clone(ones[:16]), make([]byte, 8)...)), nil, "zeros"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := progress.NewTraceContext(tc.src)
			if err == nil {
				t.Fatalf("NewTraceContext gave %+v, want an error", got)
			}
			if got != (progress.TraceContext{}) {
				t.Errorf("an error came with %+v, want nothing", got)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("error %v is not %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q does not say %q", err, tc.wantMsg)
			}
		})
	}

	got, err := progress.NewTraceContext(iotest.OneByteReader(bytes.NewReader(ones)))
	if err != nil {
		t.Fatalf("a source of one byte at a time: %v", err)
	}
	if got.Trace.String() != strings.Repeat("ff", 16) || got.Parent.String() != strings.Repeat("ff", 8) {
		t.Errorf("got trace %s and parent %s from all ones", got.Trace, got.Parent)
	}

	// The next read of one source gives the next trace context.
	src := bytes.NewReader(append(bytes.Clone(ones), bytes.Repeat([]byte{0x01}, 24)...))
	first, err1 := progress.NewTraceContext(src)
	second, err2 := progress.NewTraceContext(src)
	if err1 != nil || err2 != nil || first.Trace == second.Trace || first.Parent == second.Parent {
		t.Errorf("two reads gave %+v (%v) and %+v (%v)", first, err1, second, err2)
	}
}

// A traceparent names the trace, the parent and the flags, as
// ParseTraceContext reads them, and not the tracestate; there is none for a
// trace context that lacks a trace or a parent.
func TestTraceparent(t *testing.T) {
	t.Parallel()

	const (
		trace  = "4bf92f3577b34da6a3ce929d0e0e4736"
		parent = "00f067aa0ba902b7"
	)
	full := progress.TraceContext{
		Trace:  progress.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36},
		Parent: 0x00f067aa0ba902b7,
	}
	with := func(flags byte, state string) progress.TraceContext {
		tc := full
		tc.Flags, tc.State = flags, state
		return tc
	}
	tests := []struct {
		name string
		tc   progress.TraceContext
		want string
	}{
		{"sampled", with(0x01, ""), "00-" + trace + "-" + parent + "-01"},
		{"not sampled", with(0x00, ""), "00-" + trace + "-" + parent + "-00"},
		{"flags are two digits in lower case", with(0xab, ""), "00-" + trace + "-" + parent + "-ab"},
		{"a tracestate is left out", with(0x01, "rojo=1"), "00-" + trace + "-" + parent + "-01"},
		{"the zero value", progress.TraceContext{}, ""},
		{"no trace", progress.TraceContext{Parent: full.Parent, Flags: 0x01}, ""},
		{"no parent", progress.TraceContext{Trace: full.Trace, Flags: 0x01}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.tc.Traceparent()
			if got != tc.want {
				t.Fatalf("Traceparent() = %q, want %q", got, tc.want)
			}
			if got == "" {
				return
			}
			back, ok := progress.ParseTraceContext(got, "")
			if !ok || back.Trace != tc.tc.Trace || back.Parent != tc.tc.Parent || back.Flags != tc.tc.Flags {
				t.Errorf("ParseTraceContext(%q) = %+v, %v; want the trace, parent and flags of %+v", got, back, ok, tc.tc)
			}
		})
	}
}
