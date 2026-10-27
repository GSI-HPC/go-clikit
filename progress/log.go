// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// LogVersion is the version of the event log's schema, the "v" of every
// line. It changes when what a key or a value means does, not when a key or
// a value is added: a reader leaves out the keys and the values it does not
// know.
const LogVersion = 1

const (
	// logBuffer is the most the event log writes at once, whole lines.
	logBuffer = 64 << 10
	// logBehind is how much of the event log may wait to be written before
	// the log stops, a writer that has stopped taking it.
	logBehind = 8 << 20
	// logCloseWait is how long Close waits for what is left to be written.
	logCloseWait = 5 * time.Second
	// logTime is the time of a line, in UTC and always as long, so that the
	// times of one run sort as they happened.
	logTime = "2006-01-02T15:04:05.000000000Z07:00"
)

// Log is a sink that writes the events of a Bus as JSON, one line each but
// for the advances it samples out (see LogOptions.AdvanceEvery): an event
// log, for a bug report, a test, or a converter to another tracing system.
//
// The first line is the run, the trace its events belong to, where the
// trace came from when another program handed it on, and the program and
// its version that wrote it, as LogOptions name them:
//
//	{"v":1,"type":"trace","run":"5d0c…a1f3","trace":"4bf9…4736","parent":"00f067aa0ba902b7","traceFlags":"01","traceState":"rojo=00f067aa0ba902b7","version":"v0.4.0"}
//
// and every line after it one event, with the run and the trace again, its
// Seq, Time and Type, the span's id and its parent's as 16 hexadecimal
// digits, its Kind, Name, Flags and State, the Fields it has, and how it
// ended:
//
//	{"v":1,"run":"5d0c…a1f3","trace":"4bf9…4736","seq":7,"time":"2026-09-26T12:00:03.000000000Z","type":"end","span":"9f3c…0007","parent":"9f3c…0002","kind":"target","name":"exe0002","state":"ended","node":"exe0002","host":"exe0002.hpc.example.org","status":"failed","class":"transport","err":"…"}
//
// The run tells the lines of one Log from those of others appending to the
// same file, which may share the trace. A Timeout is in seconds. The
// Amount, Size and Unit of a span's work are on every line of the span from
// the first event that sets them, and an advance is written as "advance",
// at most once each LogOptions.AdvanceEvery a span: the others are left out,
// and counted in "leftOut" on the next advance written and on the span's
// end. The newest advance left out is held, and written once its place
// comes, unless an event of the span comes first, so that the amount a
// span stopped at, before a pause in its work, is in the log during the
// pause; its line may follow lines of other spans with a greater seq.
// A key whose value is zero or empty is left out. A line of output is
// written without its text, which never leaves the process: the event says
// only which stream it came from and how many lines before it were not
// sent; and since the log is no LineSink, and so asks for no lines, there
// are line events only while a display asks for them.
//
// The lines are written by a goroutine of the log's own, never under the
// Bus's lock, in pieces of whole lines of at most 64 KiB, a longer line on
// its own, so that the lines of runs appending to one local file at once,
// each opened with os.O_APPEND, do not cut into each other. That holds only
// where a write is taken whole: a pipe takes one whole only up to PIPE_BUF
// bytes, 4 KiB on Linux, and a file on NFS not at all when several
// machines append to it, so runs that write there each need a file of
// their own, or one writer that takes their lines whole. The lines are
// written once 64 KiB wait, as a step, a batch or the command ends, and
// LogOptions.FlushEvery, a second by default, after a line came at the
// latest, so that a log followed as it grows, or one of a run killed, falls
// little behind. A write that fails
// stops the log, and so does a writer that falls 8 MiB behind, a pipe
// whose reader has stopped: the log never holds the work up. A log that
// falls behind still writes the lines it had taken, and leaves out those
// that come after. Close writes what is left, waiting five seconds at
// most, and says why the log stops short when it does; Done says when the
// writer is no longer written to.
type Log struct {
	w   io.Writer
	o   LogOptions
	run string

	mu      sync.Mutex
	line    bytes.Buffer
	enc     *json.Encoder
	trace   string
	pending []byte
	// sampled holds, for each span with an advance the log saw, what it
	// samples by; an entry goes at the span's end.
	sampled map[SpanID]*sampling
	err     error
	closed  bool
	// abandoned says Close stopped waiting: the goroutine that writes
	// starts no write after it.
	abandoned bool

	// after starts the timer that writes an advance held, time.AfterFunc
	// but in a test.
	after func(time.Duration, func()) *time.Timer

	// wake asks for the lines kept to be written now; kept says that the
	// first of them came, which has them written LogOptions.FlushEvery
	// later.
	wake, kept    chan struct{}
	stop, stopped chan struct{}
}

const (
	// DefaultAdvanceEvery is the least time between two advances of a span
	// that a Log writes, unless LogOptions.AdvanceEvery says otherwise.
	DefaultAdvanceEvery = time.Second
	// DefaultFlushEvery is how long a line waits to be written at most,
	// unless LogOptions.FlushEvery says otherwise.
	DefaultFlushEvery = time.Second
)

// errBehind stops a log whose writer has fallen too far behind.
var errBehind = errors.New("the lines were not written as fast as the work went on, and the rest were left out")

// LogOptions configure a Log.
type LogOptions struct {
	// Program names the program that writes the log, which its first line
	// names as "program"; empty takes it from the Bus, BusOptions.Program,
	// and leaves the key out when that is empty too.
	Program string
	// Version is the version of the program that writes the log, which its
	// first line names.
	Version string
	// RunID names the run on every line, as eventlog.Run.ID reads it
	// back; empty draws 16 hexadecimal digits at random.
	RunID string
	// AdvanceEvery is the least time between two advances of a span that
	// the log writes, by the events' times; the advances in between are
	// left out, and counted in "leftOut", but for the newest, which is
	// written once AdvanceEvery has passed, unless an event of the span
	// comes first. 0 is DefaultAdvanceEvery; a
	// negative value writes every advance the Bus sends.
	AdvanceEvery time.Duration
	// FlushEvery is how long a line waits at most before the log writes
	// it, with the lines that came after it: what a reader that follows
	// the log as it grows, such as a live view of the run, lags behind. 0
	// is DefaultFlushEvery; a negative value writes each line as it comes.
	FlushEvery time.Duration
}

// advanceEvery is the least time between two advances of a span the log
// writes, as LogOptions.AdvanceEvery gives it.
func (o LogOptions) advanceEvery() time.Duration {
	if o.AdvanceEvery == 0 {
		return DefaultAdvanceEvery
	}
	return o.AdvanceEvery
}

// flushEvery is how long a line waits at most before the log writes it, as
// LogOptions.FlushEvery gives it; 0 writes each line as it comes.
func (o LogOptions) flushEvery() time.Duration {
	switch {
	case o.FlushEvery == 0:
		return DefaultFlushEvery
	case o.FlushEvery < 0:
		return 0
	}
	return o.FlushEvery
}

// sampling is what the log keeps of a span's advances to sample them: when
// it last wrote one, and how many it has left out, since then and in all,
// and the newest advance it has not written yet, which its timer writes
// once its place comes, unless an event of the span comes first.
type sampling struct {
	written time.Time
	since   int
	total   int
	held    *logEvent
	heldAt  time.Time
	timer   *time.Timer
}

// NewLog returns a Log that writes to w, and starts the goroutine that
// writes; Close stops it.
func NewLog(w io.Writer, o LogOptions) *Log {
	l := &Log{w: w, run: o.RunID, o: o, after: time.AfterFunc,
		wake: make(chan struct{}, 1), kept: make(chan struct{}, 1),
		stop: make(chan struct{}), stopped: make(chan struct{})}
	if l.run == "" {
		var run [8]byte
		// crypto/rand never fails; it ends the process when it cannot.
		_, _ = rand.Read(run[:])
		l.run = hex.EncodeToString(run[:])
	}
	l.enc = json.NewEncoder(&l.line)
	// What is escaped for HTML is nothing a log needs escaped.
	l.enc.SetEscapeHTML(false)
	go l.writeOut()
	return l
}

// logTrace is the first line of an event log.
type logTrace struct {
	V          int    `json:"v"`
	Type       string `json:"type"`
	Run        string `json:"run"`
	Trace      string `json:"trace"`
	Parent     string `json:"parent,omitempty"`
	TraceFlags string `json:"traceFlags,omitempty"`
	TraceState string `json:"traceState,omitempty"`
	Program    string `json:"program,omitempty"`
	Version    string `json:"version,omitempty"`
}

// logEvent is one event of an event log. Every member of Event is either
// copied into it by Handle or left out on purpose, as Text is.
type logEvent struct {
	V      int      `json:"v"`
	Run    string   `json:"run"`
	Trace  string   `json:"trace"`
	Seq    uint64   `json:"seq"`
	Time   string   `json:"time"`
	Type   string   `json:"type"`
	Span   string   `json:"span,omitempty"`
	Parent string   `json:"parent,omitempty"`
	Kind   string   `json:"kind,omitempty"`
	Name   string   `json:"name,omitempty"`
	Flags  []string `json:"flags,omitempty"`
	State  string   `json:"state,omitempty"`

	Node       string  `json:"node,omitempty"`
	Host       string  `json:"host,omitempty"`
	Role       string  `json:"role,omitempty"`
	Total      int     `json:"total,omitempty"`
	Limit      int     `json:"limit,omitempty"`
	Batch      string  `json:"batch,omitempty"`
	Message    string  `json:"message,omitempty"`
	Method     string  `json:"method,omitempty"`
	Path       string  `json:"path,omitempty"`
	HTTPStatus int     `json:"httpStatus,omitempty"`
	Cache      string  `json:"cache,omitempty"`
	Source     string  `json:"source,omitempty"`
	Timeout    float64 `json:"timeout,omitempty"`
	Exit       *int    `json:"exit,omitempty"`
	Amount     int64   `json:"amount,omitempty"`
	Size       int64   `json:"size,omitempty"`
	Unit       string  `json:"unit,omitempty"`

	Status  string `json:"status,omitempty"`
	Class   string `json:"class,omitempty"`
	Err     string `json:"err,omitempty"`
	Stream  string `json:"stream,omitempty"`
	Dropped int    `json:"dropped,omitempty"`
	// LeftOut counts the advances of a span the log did not write.
	LeftOut int `json:"leftOut,omitempty"`
}

// Begin writes the run the events belong to, the log's first line: its
// trace, the program, LogOptions.Program or else r.Program, and its
// version, LogOptions.Version or else r.Version.
func (l *Log) Begin(r RunInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tc := r.Trace
	l.trace = tc.ID.String()
	line := logTrace{V: LogVersion, Type: "trace", Run: l.run, Trace: l.trace,
		Program: cmp.Or(l.o.Program, r.Program), Version: cmp.Or(l.o.Version, r.Version)}
	if tc.Parent != 0 {
		line.Parent = tc.Parent.String()
		line.TraceFlags = fmt.Sprintf("%02x", tc.Flags)
		line.TraceState = tc.State
	}
	l.write(line)
}

// Handle writes e as a line of its own, but for an advance the log samples
// out (see LogOptions.AdvanceEvery), which it only counts, in "leftOut" on
// the span's next advance it writes and on its end.
func (l *Log) Handle(e Event) {
	line := logEvent{
		V:     LogVersion,
		Seq:   e.Seq,
		Time:  e.Time.UTC().Format(logTime),
		Type:  e.Type.String(),
		Name:  e.Name,
		Flags: e.Flags.names(),

		Node:       e.Node,
		Host:       e.Host,
		Role:       e.Role,
		Total:      e.Total,
		Limit:      e.Limit,
		Batch:      e.Batch,
		Message:    e.Message,
		Method:     e.Method,
		Path:       e.Path,
		HTTPStatus: e.HTTPStatus,
		Cache:      e.Cache,
		Source:     e.Source,
		Timeout:    e.Timeout.Seconds(),
		Amount:     e.Amount,
		Size:       e.Size,

		Err:     e.Err,
		Dropped: e.Dropped,
	}
	if e.Span != 0 {
		line.Span = e.Span.String()
	}
	if e.Parent != 0 {
		line.Parent = e.Parent.String()
	}
	if e.Kind != 0 {
		line.Kind = e.Kind.String()
	}
	if e.State != 0 {
		line.State = e.State.String()
	}
	if e.Status != 0 {
		line.Status = e.Status.String()
	}
	if e.Class != ClassNone {
		line.Class = e.Class.String()
	}
	if e.Stream != 0 {
		line.Stream = e.Stream.String()
	}
	if e.Unit != 0 {
		line.Unit = e.Unit.String()
	}
	if e.HasExit {
		exit := e.Exit
		line.Exit = &exit
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	line.Run, line.Trace = l.run, l.trace
	if !l.sample(e, &line) {
		return
	}
	l.write(line)
	if e.Type == TypeEnd && (e.Kind == KindCommand || e.Kind == KindStep || e.Kind == KindBatch) {
		l.due()
	}
}

// sample decides whether the log writes e, when it is an advance, and puts
// the advances it left out on the line of the next advance it writes, and
// on the line of the span's end. An advance it leaves out it holds, the
// newest of the span, and writes once AdvanceEvery has passed since the
// last it wrote, so that the amount a span stopped at is in the log before
// the span's next event, however long that takes to come. Any event of the
// span that comes first carries a newer amount, and the advance held is
// left out after all. l.mu is held.
func (l *Log) sample(e Event, line *logEvent) bool {
	s := l.sampled[e.Span]
	if s != nil && s.held != nil {
		s.drop()
	}
	switch e.Type {
	case TypeAdvance:
		every := l.o.advanceEvery()
		if every < 0 {
			return true
		}
		if l.sampled == nil {
			l.sampled = make(map[SpanID]*sampling)
		}
		if s == nil {
			l.sampled[e.Span] = &sampling{written: e.Time}
			return true
		}
		if wait := every - e.Time.Sub(s.written); wait > 0 {
			held := *line
			s.held, s.heldAt = &held, e.Time
			s.timer = l.after(wait, func() { l.late(s, &held) })
			return false
		}
		s.written, line.LeftOut, s.since = e.Time, s.since, 0
	case TypeEnd:
		if s != nil {
			line.LeftOut = s.total
			delete(l.sampled, e.Span)
		}
	}
	return true
}

// drop leaves out the advance s holds, which a newer event of the span
// makes old, and stops its timer. The log's mu is held.
func (s *sampling) drop() {
	s.timer.Stop()
	s.held, s.timer = nil, nil
	s.since++
	s.total++
}

// late writes the advance held, when its place has come and no event of
// the span came first. Its line follows those of other spans written in the
// meantime, whose seq may be greater.
func (l *Log) late(s *sampling, held *logEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s.held != held {
		return
	}
	s.write(l)
}

// write writes the advance s holds, with the advances left out before it.
// The log's mu is held.
func (s *sampling) write(l *Log) {
	held := s.held
	s.held, s.timer = nil, nil
	s.written, held.LeftOut, s.since = s.heldAt, s.since, 0
	l.write(*held)
}

// write keeps v as a line, whole, for the goroutine that writes, and has it
// written once 64 KiB wait, or once it has waited LogOptions.FlushEvery.
// l.mu is held.
func (l *Log) write(v any) {
	if l.err != nil || l.closed {
		return
	}
	l.line.Reset()
	if err := l.enc.Encode(v); err != nil {
		l.err = err
		return
	}
	if len(l.pending)+l.line.Len() > logBehind {
		// The lines already kept are still written; this one and those
		// after it are not.
		l.err = errBehind
		return
	}
	first := len(l.pending) == 0
	l.pending = append(l.pending, l.line.Bytes()...)
	switch {
	case len(l.pending) >= logBuffer || l.o.flushEvery() == 0:
		l.due()
	case first:
		select {
		case l.kept <- struct{}{}:
		default:
		}
	}
}

// due has the lines kept written now. l.mu is held.
func (l *Log) due() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// writeOut writes the lines kept as they fall due, until Close: at once
// when due says so, else LogOptions.FlushEvery after the first of them
// came. An idle log waits for nothing.
func (l *Log) writeOut() {
	defer close(l.stopped)
	var flush <-chan time.Time
	for {
		select {
		case <-l.stop:
			l.drain()
			return
		case <-l.kept:
			if flush == nil {
				flush = time.After(l.o.flushEvery())
			}
			continue
		case <-l.wake:
		case <-flush:
		}
		flush = nil
		l.drain()
	}
}

// drain writes the lines kept, in pieces of whole lines of at most 64 KiB,
// a longer line on its own. The first write that fails stops the log.
func (l *Log) drain() {
	l.mu.Lock()
	lines := l.pending
	l.pending = nil
	l.mu.Unlock()
	for len(lines) > 0 {
		n := len(lines)
		if n > logBuffer {
			n = bytes.LastIndexByte(lines[:logBuffer], '\n') + 1
			if n == 0 {
				n = bytes.IndexByte(lines, '\n') + 1
			}
		}
		if l.isAbandoned() {
			return
		}
		if _, err := l.w.Write(lines[:n]); err != nil {
			l.mu.Lock()
			if l.err == nil {
				l.err = err
			}
			l.pending = nil
			l.mu.Unlock()
			return
		}
		lines = lines[n:]
	}
}

// isAbandoned reports whether Close has stopped waiting for the writes.
func (l *Log) isAbandoned() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.abandoned
}

// Close writes what is left of the log, once the Bus is closed, waiting
// five seconds at most, and returns the first error the log met: nothing
// after it was written. Closing a closed Log only says so again.
//
// The writer is not closed; that is for whoever opened it to do, once
// Close has returned nil. When the time runs out, no write is started
// after Close returns, but one already under way cannot be stopped: Close
// returns an error that says so, and Done is closed once that write has
// returned, which a writer that is not safe to close during a write, such
// as a bufio.Writer or a gzip.Writer, waits for.
func (l *Log) Close() error {
	l.mu.Lock()
	if !l.closed {
		// The advances still held are of spans that never ended, a run
		// cut short: their amounts are the last there are.
		for _, s := range l.sampled {
			if s.held != nil {
				s.timer.Stop()
				s.write(l)
			}
		}
		l.closed = true
		close(l.stop)
	}
	l.mu.Unlock()
	select {
	case <-l.stopped:
	case <-time.After(logCloseWait):
		l.mu.Lock()
		l.abandoned = true
		if l.err == nil {
			l.err = fmt.Errorf("the last lines were not written within %s, and a write to the log may still be under way", logCloseWait)
		}
		l.mu.Unlock()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

// Done returns a channel that is closed once the log has stopped writing
// for good: after Close, once its last write has returned.
func (l *Log) Done() <-chan struct{} { return l.stopped }
