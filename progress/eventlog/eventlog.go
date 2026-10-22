// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package eventlog reads the event log that progress.Log writes, version
// 1 of the format doc/event-log.md describes: what a view of the runs a
// program made reads, and what a server reads back from a command it ran
// as a child, to send on the child's progress.
//
// Decode turns one line of a log back into what was written: the first
// line of a run, or one of its events as a progress.Event, with every
// text sanitised as a Bus sanitises it, so that a log something else
// wrote cannot act on a terminal or a page either.
//
// A reader of version 1 leaves out the keys and the values it does not
// know, as the format asks, so that a log a later release of the kit
// writes, with a key or a value added, still reads: a key Decode does not
// know is not read, a value of a kind, a state, a status, a class, a
// unit, a stream or a flag it does not know is left out, and a line of a
// type it does not know is not decoded at all.
package eventlog

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// Line is one line of an event log, decoded: the first line of a run,
// which says what the run is, or one of its events.
type Line struct {
	// Run names the run the line belongs to, the same on every line of
	// the run: the lines of several runs may be in one log.
	Run string
	// Trace is the trace the run's spans belong to. On the first line of
	// a run it says too where the trace came from, when another program
	// handed it on; on an event, only its Trace is set.
	Trace progress.TraceContext
	// Begin says the line is the first of its run; Program and Version
	// are then the program that wrote it and its version, and Event is
	// zero.
	Begin            bool
	Program, Version string
	// Event is the event the line holds, as the Bus sent it to the log,
	// but for the text of a line of output, which the log never holds.
	Event progress.Event
	// LeftOut counts the advances of the span the log did not write: on
	// an advance, those since the span's previous advance it wrote, and
	// on an end, all of the span's.
	LeftOut int
}

// line is what Decode reads of a line: every key the log writes.
type line struct {
	V          int      `json:"v"`
	Type       string   `json:"type"`
	Run        string   `json:"run"`
	Trace      string   `json:"trace"`
	Parent     string   `json:"parent"`
	TraceFlags string   `json:"traceFlags"`
	TraceState string   `json:"traceState"`
	Program    string   `json:"program"`
	Version    string   `json:"version"`
	Seq        uint64   `json:"seq"`
	Time       string   `json:"time"`
	Span       string   `json:"span"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Flags      []string `json:"flags"`
	State      string   `json:"state"`

	Node       string  `json:"node"`
	Host       string  `json:"host"`
	Role       string  `json:"role"`
	Total      int     `json:"total"`
	Limit      int     `json:"limit"`
	Batch      string  `json:"batch"`
	Message    string  `json:"message"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	HTTPStatus int     `json:"httpStatus"`
	Cache      string  `json:"cache"`
	Source     string  `json:"source"`
	Timeout    float64 `json:"timeout"`
	Exit       *int    `json:"exit"`
	Amount     int64   `json:"amount"`
	Size       int64   `json:"size"`
	Unit       string  `json:"unit"`

	Status  string `json:"status"`
	Class   string `json:"class"`
	Err     string `json:"err"`
	Stream  string `json:"stream"`
	Dropped int    `json:"dropped"`
	LeftOut int    `json:"leftOut"`
}

// Decode reads one line of an event log, with or without its newline. It
// fails on a line that is not a JSON object, one of another version of
// the format than progress.LogVersion, one without a run, one of a type
// it does not know, an event without a time, and an event of a span
// without a span id, or with an id, of the span or of its parent, that is
// not 16 hexadecimal digits, not all zero. A key whose value is of the
// wrong type is left out, and the rest of the line read. The texts of the
// errors are no interface: a reader that skips the lines it cannot read
// needs none.
//
// The texts of an event are sanitised and cut to their bounds as the Bus
// does it (see progress.Sanitize), and so are the run, the program and
// the version; a tracestate that progress.ParseTraceContext would not
// keep is left out, and so is a count, a size or a timeout below zero.
func Decode(b []byte) (Line, error) {
	var l line
	if err := json.Unmarshal(b, &l); err != nil {
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			return Line{}, fmt.Errorf("not a line of an event log: %w", err)
		}
	}
	if l.V != progress.LogVersion {
		return Line{}, fmt.Errorf("a line of version %d of the event log, not %d", l.V, progress.LogVersion)
	}
	if l.Run == "" {
		return Line{}, errors.New("a line of the event log without a run")
	}
	out := Line{Run: progress.Sanitize(l.Run, progress.MaxField)}
	out.Trace.Trace, _ = traceID(l.Trace)
	if l.Type == "trace" {
		out.Begin = true
		out.Program = progress.Sanitize(l.Program, progress.MaxField)
		out.Version = progress.Sanitize(l.Version, progress.MaxField)
		if tc, ok := progress.ParseTraceContext("00-"+l.Trace+"-"+l.Parent+"-"+l.TraceFlags, l.TraceState); ok {
			out.Trace = tc
		}
		return out, nil
	}
	e, err := l.event()
	if err != nil {
		return Line{}, err
	}
	out.Event, out.LeftOut = e, max(l.LeftOut, 0)
	return out, nil
}

// event decodes the event of a line that is not the first of its run.
func (l *line) event() (progress.Event, error) {
	typ, ok := types[l.Type]
	if !ok {
		return progress.Event{}, fmt.Errorf("an event of the type %q, which this reader does not know", l.Type)
	}
	t, err := time.Parse(time.RFC3339Nano, l.Time)
	if err != nil {
		return progress.Event{}, fmt.Errorf("an event without a time: %w", err)
	}
	e := progress.Event{Seq: l.Seq, Time: t.UTC(), Type: typ}
	if l.Span != "" || typ != progress.TypeSuspend && typ != progress.TypeResume {
		if e.Span, err = spanID(l.Span); err != nil {
			return progress.Event{}, fmt.Errorf("the span of an event: %w", err)
		}
	}
	if l.Parent != "" {
		if e.Parent, err = spanID(l.Parent); err != nil {
			return progress.Event{}, fmt.Errorf("the parent of an event: %w", err)
		}
	}
	e.Kind = kinds[l.Kind]
	e.State = states[l.State]
	e.Status = statuses[l.Status]
	e.Class = classes[l.Class]
	e.Stream = streams[l.Stream]
	for _, name := range l.Flags {
		e.Flags |= flags[name]
	}
	e.Name = progress.Sanitize(l.Name, progress.MaxField)
	e.Err = progress.Sanitize(l.Err, progress.MaxErr)
	e.Dropped = max(l.Dropped, 0)
	e.Fields = progress.Fields{
		// Node is a node set a display folds, so it is escaped but never
		// cut, as the Bus does it.
		Node:       progress.Sanitize(l.Node, 0),
		Host:       progress.Sanitize(l.Host, progress.MaxField),
		Role:       progress.Sanitize(l.Role, progress.MaxField),
		Total:      max(l.Total, 0),
		Limit:      max(l.Limit, 0),
		Batch:      progress.Sanitize(l.Batch, progress.MaxField),
		Message:    progress.Sanitize(l.Message, progress.MaxField),
		Method:     progress.Sanitize(l.Method, progress.MaxField),
		Path:       progress.Sanitize(l.Path, progress.MaxField),
		HTTPStatus: max(l.HTTPStatus, 0),
		Cache:      progress.Sanitize(l.Cache, progress.MaxField),
		Source:     progress.Sanitize(l.Source, progress.MaxField),
		Timeout:    seconds(l.Timeout),
		Exit:       l.Exit,
		Amount:     max(l.Amount, 0),
		Size:       max(l.Size, 0),
	}
	e.Unit = units[l.Unit]
	return e, nil
}

// seconds is a timeout of the log as a duration: none below zero, and at
// most the longest a duration holds.
func seconds(s float64) time.Duration {
	switch {
	case s <= 0:
		return 0
	case s >= math.MaxInt64/float64(time.Second):
		return math.MaxInt64
	}
	return time.Duration(s * float64(time.Second))
}

// spanID reads a span id as the log writes it, 16 hexadecimal digits not
// all zero.
func spanID(s string) (progress.SpanID, error) {
	id, err := strconv.ParseUint(s, 16, 64)
	if len(s) != 16 || err != nil || id == 0 {
		return 0, fmt.Errorf("%q is not 16 hexadecimal digits, not all zero", s)
	}
	return progress.SpanID(id), nil
}

// traceID reads a trace id as the log writes it, 32 hexadecimal digits.
func traceID(s string) (progress.TraceID, bool) {
	var id progress.TraceID
	if len(s) != 2*len(id) {
		return id, false
	}
	if _, err := hex.Decode(id[:], []byte(s)); err != nil {
		return progress.TraceID{}, false
	}
	return id, true
}

// named maps the name the log writes for each of values, its String, to
// the value; a name it does not hold is the zero value, which is none.
func named[T fmt.Stringer](values ...T) map[string]T {
	m := make(map[string]T, len(values))
	for _, v := range values {
		m[v.String()] = v
	}
	return m
}

// The values the log names.
var (
	types = named(progress.TypeStart, progress.TypeRun, progress.TypeUpdate, progress.TypeLine,
		progress.TypeEnd, progress.TypeSuspend, progress.TypeResume, progress.TypeAdvance)
	kinds = named(progress.KindCommand, progress.KindStep, progress.KindBatch,
		progress.KindTarget, progress.KindCall, progress.KindWait)
	states   = named(progress.StateQueued, progress.StateRunning, progress.StateEnded)
	statuses = named(progress.StatusOK, progress.StatusFailed, progress.StatusCanceled, progress.StatusSkipped)
	classes  = named(progress.ClassTarget, progress.ClassTransport, progress.ClassTimeout,
		progress.ClassAuth, progress.ClassPin, progress.ClassUsage, progress.ClassCanceled)
	streams = named(progress.Stdout, progress.Stderr)
	units   = named(progress.Items, progress.Bytes, progress.Percent)
	flags   = named(progress.Hidden, progress.Fold, progress.ShowLines, progress.DryRun)
)
