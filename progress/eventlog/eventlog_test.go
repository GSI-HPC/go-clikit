// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog_test

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/eventlog"
)

// golden returns the lines of the golden log, with each span id written
// as "#n" made the n it stands for, as 16 hexadecimal digits.
func golden(t testing.TB) []string {
	t.Helper()
	b, err := os.ReadFile("../testdata/log-v1.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	ids := regexp.MustCompile(`"#(\d+)"`)
	s := ids.ReplaceAllStringFunc(string(b), func(m string) string {
		n, _ := strconv.Atoi(m[2 : len(m)-1])
		return fmt.Sprintf("%q", fmt.Sprintf("%016x", n))
	})
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// rewrite writes lines through a progress.Log again, every advance among
// them, and returns what it wrote.
func rewrite(t testing.TB, lines []eventlog.Line) string {
	t.Helper()
	var out bytes.Buffer
	var log *progress.Log
	for _, l := range lines {
		if l.Begin {
			if log != nil {
				t.Fatalf("a second first line: %+v", l)
			}
			log = progress.NewLog(&out, progress.LogOptions{RunID: l.RunID, Program: l.Program, Version: l.Version,
				AdvanceEvery: -1, FlushEvery: -1})
			log.Begin(progress.RunInfo{Trace: l.Trace})
			continue
		}
		log.Handle(l.Event)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// The golden log decodes to what, written again, is the golden log: every
// key and every value it lists is read back. The log writes every advance
// again, so what it left out the first time is all it does not say.
func TestDecodeReadsTheGoldenLog(t *testing.T) {
	lines := golden(t)
	var decoded []eventlog.Line
	leftOut := regexp.MustCompile(`,"leftOut":(\d+)`)
	var want strings.Builder
	for i, s := range lines {
		l, err := eventlog.Decode([]byte(s))
		if err != nil {
			t.Fatalf("line %d: %v", i+1, err)
		}
		if l.RunID != "0123456789abcdef" || l.Trace.ID.String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("line %d: run %q, trace %v", i+1, l.RunID, l.Trace.ID)
		}
		if m := leftOut.FindStringSubmatch(s); m != nil {
			if n, _ := strconv.Atoi(m[1]); l.LeftOut != n {
				t.Errorf("line %d: LeftOut = %d, want %d", i+1, l.LeftOut, n)
			}
		} else if l.LeftOut != 0 {
			t.Errorf("line %d: LeftOut = %d, want none", i+1, l.LeftOut)
		}
		decoded = append(decoded, l)
		want.WriteString(leftOut.ReplaceAllString(s, "") + "\n")
	}
	first := decoded[0]
	if !first.Begin || first.Trace.Parent.String() != "00f067aa0ba902b7" || first.Trace.Flags != 1 ||
		first.Trace.State != "rojo=00f067aa0ba902b7" || first.Version != "v1.2.3" {
		t.Errorf("the first line = %+v", first)
	}
	if got := rewrite(t, decoded); got != want.String() {
		t.Errorf("written again, the golden log is\n%s\nwant\n%s", got, want.String())
	}
}

// A line of another version is refused with a *VersionError, which is
// ErrVersion, so that a reader can say why it shows nothing; a line that
// is not one of a log is no VersionError.
func TestDecodeTellsAnotherVersion(t *testing.T) {
	for _, tc := range []struct {
		line string
		got  int
	}{
		{`{"v":2,"type":"trace","run":"r"}`, 2},
		{`{"v":-1,"run":"r","type":"start"}`, -1},
		{`{"type":"trace","run":"r"}`, 0},
		{`not a line`, 0},
	} {
		_, err := eventlog.Decode([]byte(tc.line))
		var ve *eventlog.VersionError
		if as := errors.As(err, &ve); as != errors.Is(err, eventlog.ErrVersion) || as != (tc.got != 0) || as && ve.Got != tc.got {
			t.Errorf("Decode(%s) = %v, want a VersionError of %d, ErrVersion, or neither for 0", tc.line, err, tc.got)
		}
	}
}

func TestDecodeRefuses(t *testing.T) {
	for _, tc := range []struct {
		name, line, err string
	}{
		{"not JSON", `{"v":1,`, "not a line of an event log"},
		{"not an object", `[1]`, "names no version"},
		{"another version", `{"v":2,"type":"trace","run":"r"}`, "version 2"},
		{"no version", `{"type":"trace","run":"r"}`, "names no version"},
		{"no run", `{"v":1,"type":"trace"}`, "without a run"},
		{"a type not known", `{"v":1,"run":"r","type":"fork","time":"2026-09-25T12:00:00Z","span":"0000000000000001"}`, `"fork"`},
		{"no type", `{"v":1,"run":"r","time":"2026-09-25T12:00:00Z","span":"0000000000000001"}`, `type ""`},
		{"no time", `{"v":1,"run":"r","type":"start","span":"0000000000000001"}`, "without a time"},
		{"no span", `{"v":1,"run":"r","type":"end","time":"2026-09-25T12:00:00Z"}`, "the span of an event"},
		{"a short span", `{"v":1,"run":"r","type":"end","time":"2026-09-25T12:00:00Z","span":"01"}`, `"01" is not 16`},
		{"a span of zeros", `{"v":1,"run":"r","type":"end","time":"2026-09-25T12:00:00Z","span":"0000000000000000"}`, "not all zero"},
		{"a span not hexadecimal", `{"v":1,"run":"r","type":"end","time":"2026-09-25T12:00:00Z","span":"000000000000000g"}`, "the span of an event"},
		{"a span with a sign", `{"v":1,"run":"r","type":"end","time":"2026-09-25T12:00:00Z","span":"+000000000000001"}`, "the span of an event"},
		{"a bad parent", `{"v":1,"run":"r","type":"end","time":"2026-09-25T12:00:00Z","span":"0000000000000001","parent":"x"}`, "the parent of an event"},
		{"a bad span of a suspend", `{"v":1,"run":"r","type":"suspend","time":"2026-09-25T12:00:00Z","span":"x"}`, "the span of an event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := eventlog.Decode([]byte(tc.line))
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("Decode(%s) = %v, want an error with %q", tc.line, err, tc.err)
			}
		})
	}
}

// A reader of version 1 leaves out what it does not know, and the rest of
// the line stands.
func TestDecodeLeavesOutWhatItDoesNotKnow(t *testing.T) {
	l, err := eventlog.Decode([]byte(`{"v":1,"run":"r","trace":"x","seq":3,"time":"2026-09-25T12:00:00+02:00","type":"end",` +
		`"span":"0000000000000002","parent":"0000000000000001","kind":"pod","name":"exe1","flags":["fold","sticky"],` +
		`"state":"gone","status":"lost","class":"cosmic","stream":"stdlog","unit":"parsecs","total":"many","node":"exe1","future":{"a":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	e := l.Event
	want := progress.Event{Seq: 3, Time: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC), Type: progress.TypeEnd,
		Span: 2, Parent: 1, Name: "exe1", Flags: progress.FlagFold, Fields: progress.Fields{Node: "exe1"}}
	if !reflect.DeepEqual(e, want) {
		t.Errorf("Decode = %+v, want %+v", e, want)
	}
	if l.Trace != (progress.TraceContext{}) {
		t.Errorf("the trace %q is %v, want none", "x", l.Trace)
	}
}

func TestDecodeSuspendWithoutSpan(t *testing.T) {
	l, err := eventlog.Decode([]byte(`{"v":1,"run":"r","seq":1,"time":"2026-09-25T12:00:00Z","type":"resume"}`))
	if err != nil || l.Event.Type != progress.TypeResume || l.Event.Span != 0 {
		t.Errorf("Decode = %+v, %v", l, err)
	}
}

// What another writer put in a log reaches a reader as safe as what a Bus
// puts there: escaped, cut to its bound, and no count below zero.
func TestDecodeSanitises(t *testing.T) {
	long := strings.Repeat("x", 2*progress.MaxErr)
	l, err := eventlog.Decode([]byte(`{"v":1,"run":"r\u001b","time":"2026-09-25T12:00:00Z","type":"end","span":"0000000000000001",` +
		`"name":"a\u001b[31mb","node":"` + long + `","host":"` + long + `","message":"one\ntwo","err":"` + long + `",` +
		`"total":-1,"limit":-2,"httpStatus":-3,"amount":-4,"size":-5,"dropped":-6,"leftOut":-7,"timeout":-1}`))
	if err != nil {
		t.Fatal(err)
	}
	e := l.Event
	for _, c := range []struct {
		what, got, want string
	}{
		{"the run", l.RunID, `r\x1b`},
		{"the name", e.Name, `a\x1b[31mb`},
		{"the node", e.Node, long},
		{"the host", e.Host, long[:progress.MaxField]},
		{"the message", e.Message, `one\ntwo`},
		{"the error", e.Err, long[:progress.MaxErr]},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.what, c.got, c.want)
		}
	}
	if e.Total != 0 || e.Limit != 0 || e.HTTPStatus != 0 || e.Amount != 0 || e.Size != 0 || e.Dropped != 0 ||
		l.LeftOut != 0 || e.Timeout != 0 {
		t.Errorf("counts below zero were kept: %+v, leftOut %d", e, l.LeftOut)
	}
}

// An exit status is a whole number; anything else is no exit status, not
// exit 0.
func TestDecodeExit(t *testing.T) {
	for _, tc := range []struct {
		exit string
		want int
		has  bool
	}{
		{`"exit":0`, 0, true},
		{`"exit":2`, 2, true},
		{`"other":1`, 0, false},
		{`"exit":null`, 0, false},
		{`"exit":"x"`, 0, false},
		{`"exit":1.5`, 0, false},
		{`"exit":true`, 0, false},
		{`"exit":{}`, 0, false},
	} {
		l, err := eventlog.Decode([]byte(`{"v":1,"run":"r","time":"2026-09-25T12:00:00Z","type":"end","span":"0000000000000001",` + tc.exit + `}`))
		if err != nil {
			t.Errorf("%s: %v", tc.exit, err)
			continue
		}
		if e := l.Event; e.Exit != tc.want || e.HasExit != tc.has {
			t.Errorf("%s: Exit, HasExit = %d, %t; want %d, %t", tc.exit, e.Exit, e.HasExit, tc.want, tc.has)
		}
	}
}

func TestDecodeTimeout(t *testing.T) {
	for _, tc := range []struct {
		seconds string
		want    time.Duration
	}{
		{"1.5", 1500 * time.Millisecond},
		{"0", 0},
		{"1e300", math.MaxInt64},
		{"9223372036.854775807", math.MaxInt64},
	} {
		l, err := eventlog.Decode([]byte(`{"v":1,"run":"r","time":"2026-09-25T12:00:00Z","type":"start","span":"0000000000000001","timeout":` + tc.seconds + `}`))
		if err != nil || l.Event.Timeout != tc.want {
			t.Errorf("a timeout of %s = %v, %v; want %v", tc.seconds, l.Event.Timeout, err, tc.want)
		}
	}
}

// The first line of a run keeps where its trace came from only as
// progress.ParseTraceContext would have taken it.
func TestDecodeTheFirstLine(t *testing.T) {
	const trace = "4bf92f3577b34da6a3ce929d0e0e4736"
	for _, tc := range []struct {
		name, line string
		want       eventlog.Line
		// traced says the line has the trace id.
		traced bool
	}{
		{"no parent", `{"v":1,"type":"trace","run":"r","trace":"` + trace + `","program":"prog\u0007"}`,
			eventlog.Line{RunID: "r", Begin: true, Program: `prog\x07`, Trace: progress.TraceContext{}}, true},
		{"a parent", `{"v":1,"type":"trace","run":"r","trace":"` + trace + `","parent":"00f067aa0ba902b7","traceFlags":"01","traceState":"a=b"}`,
			eventlog.Line{RunID: "r", Begin: true, Trace: progress.TraceContext{Parent: 0x00f067aa0ba902b7, Flags: 1, State: "a=b"}}, true},
		{"a tracestate not kept", `{"v":1,"type":"trace","run":"r","trace":"` + trace + `","parent":"00f067aa0ba902b7","traceFlags":"01","traceState":"a=\u001b"}`,
			eventlog.Line{RunID: "r", Begin: true, Trace: progress.TraceContext{Parent: 0x00f067aa0ba902b7, Flags: 1}}, true},
		{"a parent without flags", `{"v":1,"type":"trace","run":"r","trace":"` + trace + `","parent":"00f067aa0ba902b7"}`,
			eventlog.Line{RunID: "r", Begin: true, Trace: progress.TraceContext{}}, true},
		{"a short trace", `{"v":1,"type":"trace","run":"r","trace":"4bf9"}`,
			eventlog.Line{RunID: "r", Begin: true}, false},
		{"a trace not hexadecimal", `{"v":1,"type":"trace","run":"r","trace":"` + strings.Repeat("z", 32) + `"}`,
			eventlog.Line{RunID: "r", Begin: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.traced {
				tc.want.Trace.ID = mustTrace(t, trace)
			}
			got, err := eventlog.Decode([]byte(tc.line))
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Decode = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

// mustTrace returns the trace id s writes.
func mustTrace(t testing.TB, s string) progress.TraceID {
	t.Helper()
	tc, ok := progress.ParseTraceContext("00-"+s+"-00f067aa0ba902b7-01", "")
	if !ok {
		t.Fatalf("no trace id: %q", s)
	}
	return tc.ID
}

// What Decode reads, written again by a Log and read again, is what it
// read: the reader and the writer agree on every line either takes.
func FuzzDecode(f *testing.F) {
	for _, s := range golden(f) {
		f.Add([]byte(s))
	}
	f.Add([]byte(`{"v":1,"run":"r","time":"2026-09-25T12:00:00Z","type":"end","span":"0000000000000001","name":"a\u009bb","timeout":1e30}`))
	f.Add([]byte(`{"v":1,"run":"r","time":"2026-09-25T12:00:00Z","type":"end","span":"0000000000000001","exit":"x"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		l, err := eventlog.Decode(b)
		if err != nil {
			return
		}
		begin := eventlog.Line{RunID: l.RunID, Begin: true, Trace: l.Trace}
		if l.Begin {
			begin = l
		}
		lines := []eventlog.Line{begin}
		if !l.Begin {
			l.LeftOut = 0
			lines = append(lines, l)
		}
		written := strings.Split(strings.TrimSuffix(rewrite(t, lines), "\n"), "\n")
		again, err := eventlog.Decode([]byte(written[len(written)-1]))
		if err != nil {
			t.Fatalf("Decode(%s), written as %s: %v", b, written[len(written)-1], err)
		}
		if !l.Begin {
			// An event's line says only the trace id of its run.
			l.Trace = progress.TraceContext{ID: l.Trace.ID}
		}
		if !reflect.DeepEqual(again, l) {
			t.Errorf("Decode(%s) = %+v, but written again and read, %+v", b, l, again)
		}
	})
}
