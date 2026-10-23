// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// TraceContext says which trace the spans of a Bus belong to and, when
// another program handed the trace on, where in it they belong: under a
// span of that program's, with the trace flags and the tracestate it gave.
// A Bus records where its trace came from, for an event log, and changes
// nothing else by it: a trace the other program does not sample is shown
// and logged all the same.
// A Bus takes its trace context as BusOptions.Trace.
type TraceContext struct {
	// ID is the trace the spans belong to.
	ID TraceID
	// Parent is the other program's span the work runs under; zero when
	// the trace began with the Bus.
	Parent SpanID
	// Flags are the trace flags the other program gave, whose lowest bit
	// says that it samples the trace.
	Flags byte
	// State is the tracestate the other program gave, the part of the
	// trace context each tracing system keeps for itself, as it was given.
	State string
}

// maxTraceState bounds the tracestate a Bus records: W3C asks that at
// least 512 characters of it be passed on.
const maxTraceState = 512

// ParseTraceContext reads the trace context another program handed on, as
// a traceparent and a tracestate in the W3C Trace Context format, such as
// the TRACEPARENT and TRACESTATE a CI job runs a program with.
//
// A traceparent is four fields of lowercase hexadecimal digits split by
// dashes: the version, the trace id, the parent's span id and the trace
// flags, as in 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01.
// Version 00 is exactly that; a later version may add fields after a
// further dash, which are left out. Version ff, a trace or span id of
// zeros, a letter in upper case and anything else that does not fit make
// the traceparent invalid, and an invalid one is no trace context at all:
// ok is false, and the tracestate is not looked at. The tracestate is kept
// as it was given when it is at most 512 bytes of printable ASCII and
// tabs, and left out otherwise.
func ParseTraceContext(traceparent, tracestate string) (tc TraceContext, ok bool) {
	// "00-" + 32 digits + "-" + 16 digits + "-" + 2 digits.
	const size = 55
	s := traceparent
	if len(s) < size || s[2] != '-' || s[35] != '-' || s[52] != '-' {
		return TraceContext{}, false
	}
	version, ok := lowerHex(s[0:2])
	if !ok || version[0] == 0xff {
		return TraceContext{}, false
	}
	// Version 00 has nothing after the flags; a later one may, after a
	// dash.
	if (version[0] == 0 && len(s) != size) || (len(s) > size && s[size] != '-') {
		return TraceContext{}, false
	}
	trace, okTrace := lowerHex(s[3:35])
	parent, okParent := lowerHex(s[36:52])
	flags, okFlags := lowerHex(s[53:55])
	if !okTrace || !okParent || !okFlags {
		return TraceContext{}, false
	}
	copy(tc.ID[:], trace)
	for _, b := range parent {
		tc.Parent = tc.Parent<<8 | SpanID(b)
	}
	if tc.ID == (TraceID{}) || tc.Parent == 0 {
		return TraceContext{}, false
	}
	tc.Flags = flags[0]
	if validTraceState(tracestate) {
		tc.State = tracestate
	}
	return tc, true
}

// NewTraceContext makes the trace context of a trace that begins here, to
// hand to a program this one starts, in its TRACEPARENT: a new trace id and
// a new parent span id, read from r, 24 bytes in all. A program passes
// crypto/rand.Reader, and a test bytes of its own. The first 16 bytes are
// the trace id and the next 8 the span id, most significant byte first, as
// ParseTraceContext reads one. The sampled flag is set, so Flags is 0x01,
// because the program that makes a trace context records the run of the
// program it hands it to; State is empty.
//
// A read that fails, or ends before 24 bytes, returns its error, wrapped.
// So does an id of zeros, which W3C does not allow and only a source that
// is broken gives: the source is not read again.
func NewTraceContext(r io.Reader) (TraceContext, error) {
	var b [24]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return TraceContext{}, fmt.Errorf("reading a new trace context: %w", err)
	}
	var tc TraceContext
	copy(tc.ID[:], b[:16])
	for _, c := range b[16:] {
		tc.Parent = tc.Parent<<8 | SpanID(c)
	}
	if tc.ID == (TraceID{}) || tc.Parent == 0 {
		return TraceContext{}, errors.New("the source of a new trace context gave an id of zeros")
	}
	tc.Flags = 0x01
	return tc, nil
}

// Traceparent returns the traceparent of tc in the W3C Trace Context
// format, version 00 and lowercase hexadecimal digits: the trace id, the
// parent's span id and the trace flags, as in
// 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01, which
// ParseTraceContext reads back to the same ID, Parent and Flags. A
// program puts it in the TRACEPARENT of a program it starts.
//
// It returns "" for a trace context with no trace or no parent, which no
// traceparent can name. It leaves State out: a program that passes the
// tracestate on does so itself, in TRACESTATE, from tc.State.
func (tc TraceContext) Traceparent() string {
	if tc.ID == (TraceID{}) || tc.Parent == 0 {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-%02x", tc.ID, tc.Parent, tc.Flags)
}

// lowerHex decodes s, which has to be hexadecimal digits in lower case.
func lowerHex(s string) ([]byte, bool) {
	for i := range len(s) {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil, false
		}
	}
	b, err := hex.DecodeString(s)
	return b, err == nil
}

// validTraceState reports whether a tracestate may be recorded as it is:
// printable ASCII, and the tabs W3C allows around the commas between its
// members, which is all it allows in one, and not too long.
func validTraceState(s string) bool {
	if len(s) > maxTraceState {
		return false
	}
	for i := range len(s) {
		if c := s[i]; (c < 0x20 && c != '\t') || c > 0x7e {
			return false
		}
	}
	return true
}
