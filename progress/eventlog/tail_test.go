// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/eventlog"
)

// record is a Feed that keeps what it was given.
type record struct {
	lines  []string
	resets int
}

func (r *record) Take(b []byte) { r.lines = append(r.lines, string(b)) }
func (r *record) Reset()        { r.lines, r.resets = nil, r.resets+1 }

// take returns the lines given since the last call.
func (r *record) take() []string {
	lines := r.lines
	r.lines = nil
	return lines
}

// appendTo appends s to the file at path, creating it.
func appendTo(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// poll polls tail and fails the test on an error, or when what it
// reports is not want.
func poll(t *testing.T, tail *eventlog.Tail, want bool) {
	t.Helper()
	got, err := tail.Poll()
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if got != want {
		t.Fatalf("Poll() = %v, want %v", got, want)
	}
}

// equal fails the test when the lines got are not want.
func equal(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s: got %q, want %q", what, got, want)
	}
}

// A log written a byte at a time is read a line at a time: no line is
// lost, cut or read twice, and Poll reports the bytes that end a line.
func TestTailByteByByte(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	var feed record
	tail := eventlog.NewTail(path, &feed)
	poll(t, tail, false)
	lines := golden(t)
	data := strings.Join(lines, "\n") + "\n"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for i := range len(data) {
		if _, err := f.WriteString(data[i : i+1]); err != nil {
			t.Fatal(err)
		}
		poll(t, tail, data[i] == '\n')
	}
	equal(t, "the lines", feed.lines, lines)
	poll(t, tail, false)
}

// Blank lines and the spaces around a line are no part of the log.
func TestTailBlankLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	appendTo(t, path, "\n  a \r\n\n\t\nb\n")
	var feed record
	poll(t, eventlog.NewTail(path, &feed), true)
	equal(t, "the lines", feed.lines, []string{"a", "b"})
}

func TestTailReadsTheRotatedCopyFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	// Only the copy is there: its lines come on the first Poll, and a
	// last line cut off in it is no line yet.
	appendTo(t, path+".1", "old 1\nold 2\nold 3 cut")
	var feed record
	tail := eventlog.NewTail(path, &feed)
	poll(t, tail, true)
	equal(t, "the copy", feed.take(), []string{"old 1", "old 2"})
	poll(t, tail, false)
	// The log appears, and a command still writing to the copy ends its
	// line there.
	appendTo(t, path, "new 1\n")
	appendTo(t, path+".1", "\n")
	poll(t, tail, true)
	equal(t, "the copy, then the log", feed.take(), []string{"old 3 cut", "new 1"})
	if feed.resets != 0 {
		t.Errorf("%d resets, want none", feed.resets)
	}
}

// A log moved to its copy is read to its end there, and the new log from
// its start: each line once, and no reset.
func TestTailRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	appendTo(t, path, "one 1\n")
	var feed record
	tail := eventlog.NewTail(path, &feed)
	poll(t, tail, true)
	equal(t, "before", feed.take(), []string{"one 1"})
	appendTo(t, path, "one 2 ha")
	poll(t, tail, false)

	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	// While the new log is not there yet, the copy is followed.
	appendTo(t, path+".1", "lf\n")
	poll(t, tail, true)
	equal(t, "rotated, no log yet", feed.take(), []string{"one 2 half"})
	appendTo(t, path, "two 1\n")
	appendTo(t, path+".1", "one 3\n")
	poll(t, tail, true)
	equal(t, "rotated", feed.take(), []string{"one 3", "two 1"})
	poll(t, tail, false)

	// Rotated again: the copy is the second log now, read on from where
	// it was; the first is gone.
	appendTo(t, path, "two 2\n")
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	appendTo(t, path, "three 1\n")
	poll(t, tail, true)
	equal(t, "rotated again", feed.take(), []string{"two 2", "three 1"})
	if feed.resets != 0 {
		t.Errorf("%d resets, want none", feed.resets)
	}
}

// A copy that is no longer the file the Tail followed is left alone.
func TestTailCopyReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	appendTo(t, path+".1", "old 1\n")
	appendTo(t, path, "new 1\n")
	var feed record
	tail := eventlog.NewTail(path, &feed)
	poll(t, tail, true)
	equal(t, "before", feed.take(), []string{"old 1", "new 1"})
	// Made apart first, so that it cannot take the inode of the copy.
	appendTo(t, path+".x", "other 1\n")
	if err := os.Rename(path+".x", path+".1"); err != nil {
		t.Fatal(err)
	}
	poll(t, tail, false)
	appendTo(t, path+".1", "other 2\n")
	appendTo(t, path, "new 2\n")
	poll(t, tail, true)
	equal(t, "after", feed.take(), []string{"new 2"})
}

func TestTailStartsOver(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, path string)
	}{
		{"cut short", func(t *testing.T, path string) {
			if err := os.Truncate(path, 0); err != nil {
				t.Fatal(err)
			}
			appendTo(t, path, "b\n")
		}},
		// The same file, as long as before: only its start tells.
		{"written again in place", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("b 1\nb 2\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			appendTo(t, path, "b\n")
		}},
		{"replaced by another file", func(t *testing.T, path string) {
			appendTo(t, path+".x", "b\n")
			if err := os.Rename(path+".x", path); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "log.jsonl")
			appendTo(t, path+".1", "old\n")
			appendTo(t, path, "a 1\na 2\n")
			var feed record
			tail := eventlog.NewTail(path, &feed)
			poll(t, tail, true)
			tc.change(t, path)
			poll(t, tail, true)
			if feed.resets != 1 {
				t.Errorf("%d resets, want 1", feed.resets)
			}
			want := []string{"old", "b"}
			if tc.name == "written again in place" {
				want = []string{"old", "b 1", "b 2", "b"}
			}
			equal(t, "read again", feed.take(), want)
		})
	}
}

// A log that is gone is read no more, and what was read stands, until a
// log is there again.
func TestTailLogRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	appendTo(t, path, "a\n")
	var feed record
	tail := eventlog.NewTail(path, &feed)
	poll(t, tail, true)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	poll(t, tail, false)
	equal(t, "gone", feed.lines, []string{"a"})
}

func TestTailLongLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	var feed record
	tail := eventlog.NewTail(path, &feed)
	// A line longer than the reader's buffer is still a line, and so is
	// one of MaxLine bytes with its newline.
	big := strings.Repeat("x", 200<<10)
	most := strings.Repeat("m", eventlog.MaxLine-1)
	appendTo(t, path, big+"\n"+most+"\n")
	poll(t, tail, true)
	equal(t, "long lines", feed.take(), []string{big, most})

	// A line over the limit is left out, whole, written over several
	// polls, and the line after it is read.
	huge := strings.Repeat("y", eventlog.MaxLine)
	appendTo(t, path, "start ")
	poll(t, tail, false)
	appendTo(t, path, huge)
	poll(t, tail, false)
	appendTo(t, path, huge)
	poll(t, tail, false)
	appendTo(t, path, "end\nafter\n")
	poll(t, tail, true)
	equal(t, "after the longest", feed.take(), []string{"after"})
}

func TestTailErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	appendTo(t, file, "x\n")
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "log.jsonl.1"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path string
	}{
		// A directory opens, but cannot be read.
		{"a directory", t.TempDir()},
		// A path below a file is not a missing log.
		{"below a file", filepath.Join(file, "log.jsonl")},
		{"a copy that is a directory", filepath.Join(dir, "log.jsonl")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var feed record
			changed, err := eventlog.NewTail(tc.path, &feed).Poll()
			if err == nil || !strings.HasPrefix(err.Error(), "reading the event log: ") || changed {
				t.Errorf("Poll() = %v, %v; want an error", changed, err)
			}
		})
	}
}

// What was read before an error is given, and the next Poll goes on.
func TestTailAfterAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.jsonl")
	appendTo(t, path, "a\n")
	if err := os.Mkdir(path+".1", 0o700); err != nil {
		t.Fatal(err)
	}
	var feed record
	tail := eventlog.NewTail(path, &feed)
	if _, err := tail.Poll(); err == nil {
		t.Fatal("no error for a copy that is a directory")
	}
	if err := os.Remove(path + ".1"); err != nil {
		t.Fatal(err)
	}
	poll(t, tail, true)
	equal(t, "the log", feed.take(), []string{"a"})
}

// sink keeps the runs it is told of and the events it is given.
type sink struct {
	runs   []progress.RunInfo
	events []progress.Event
}

func (s *sink) Begin(r progress.RunInfo) { s.runs = append(s.runs, r) }
func (s *sink) Handle(e progress.Event)  { s.events = append(s.events, e) }

// handler is a sink that is no RunSink.
type handler struct{ events []progress.Event }

func (h *handler) Handle(e progress.Event) { h.events = append(h.events, e) }

// A Forwarder gives a sink every event of its run once, in order, the log
// read again after a reset too, and tells a RunSink of the run first, with
// the version the log names; a line it cannot read is skipped, and
// counted.
func TestNewForwarder(t *testing.T) {
	lines := golden(t)
	var want []progress.Event
	for _, s := range lines[1:] {
		l, err := eventlog.Decode([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, l.Event)
	}
	var s sink
	f := eventlog.NewForwarder(&s, mustTrace(t, "4bf92f3577b34da6a3ce929d0e0e4736"))
	for range 2 {
		f.Reset()
		f.Take([]byte("not a line"))
		f.Take([]byte(`{"v":2,"run":"0123456789abcdef","type":"start"}`))
		for _, l := range lines {
			f.Take([]byte(l))
		}
	}
	if len(s.runs) != 1 || s.runs[0].Trace.Parent.String() != "00f067aa0ba902b7" || s.runs[0].Version != "v1.2.3" {
		t.Errorf("the runs = %+v, want the golden log's", s.runs)
	}
	if !reflect.DeepEqual(s.events, want) {
		t.Errorf("the events = %+v\nwant %+v", s.events, want)
	}
	if f.Skipped() != 4 || f.Dropped() != 0 {
		t.Errorf("Skipped() = %d, Dropped() = %d; want 4 and 0", f.Skipped(), f.Dropped())
	}
}

// begin returns the first line of a run of trace, and event the line of
// its event seq.
func begin(run, trace string) string {
	return `{"v":1,"type":"trace","run":"` + run + `","trace":"` + trace + `"}`
}

func event(run, trace string, seq int) string {
	return fmt.Sprintf(`{"v":1,"run":%q,"trace":%q,"seq":%d,"time":"2026-09-25T12:00:00Z","type":"start","span":"%016x","kind":"call","name":"c","state":"running"}`,
		run, trace, seq, seq)
}

// A Forwarder gives the sink one run, the first that begins in its trace,
// as a sink is promised: the lines of the other runs, and those of its own
// run before the run's first line, are dropped, and counted. A zero trace,
// and Forward, take the first run that begins.
func TestAForwarderGivesOneRun(t *testing.T) {
	const ours, theirs = "4bf92f3577b34da6a3ce929d0e0e4736", "0af7651916cd43dd8448eb211c80319c"
	log := []string{
		event("aaaa", ours, 1),
		begin("bbbb", theirs), event("bbbb", theirs, 1),
		begin("aaaa", ours), event("aaaa", ours, 1), event("aaaa", ours, 2),
		begin("cccc", ours), event("cccc", ours, 1),
		begin("aaaa", ours), event("bbbb", theirs, 2), event("aaaa", ours, 3),
	}
	for _, tc := range []struct {
		name    string
		feed    func(progress.Sink) eventlog.Feed
		run     string
		seqs    []uint64
		dropped int
	}{
		{"its trace", func(s progress.Sink) eventlog.Feed { return eventlog.NewForwarder(s, mustTrace(t, ours)) },
			"aaaa", []uint64{1, 2, 3}, 6},
		{"a zero trace", func(s progress.Sink) eventlog.Feed { return eventlog.NewForwarder(s, progress.TraceID{}) },
			"bbbb", []uint64{1, 2}, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s sink
			f := tc.feed(&s)
			for _, l := range log {
				f.Take([]byte(l))
			}
			var seqs []uint64
			for _, e := range s.events {
				seqs = append(seqs, e.Seq)
			}
			if len(s.runs) != 1 || !slices.Equal(seqs, tc.seqs) {
				t.Errorf("the runs = %+v, the events %v; want one run, and %v", s.runs, seqs, tc.seqs)
			}
			if got := f.(*eventlog.Forwarder).Dropped(); got != tc.dropped {
				t.Errorf("Dropped() = %d, want %d", got, tc.dropped)
			}
		})
	}

	// A sink that is no RunSink is given the events alone.
	var h handler
	f := eventlog.NewForwarder(&h, mustTrace(t, ours))
	for _, l := range log {
		f.Take([]byte(l))
	}
	if len(h.events) != 3 {
		t.Errorf("the events = %+v, want the run's three", h.events)
	}
}
