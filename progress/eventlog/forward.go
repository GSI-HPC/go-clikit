// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog

import "github.com/GSI-HPC/go-clikit/progress"

// Forwarder is a Feed that gives a sink the events of one run of the lines
// it takes: what a server that runs a command as a child, with the child's
// event log in a file of its own, does to send on the child's progress, to
// an mcpprogress.Notifier for instance.
//
// The run is the first that begins, at its first line, in the trace the
// Forwarder was made for: the trace the server handed the child, with
// progress.Span.TraceContext in its TRACEPARENT, which the child's Bus
// continues. A sink is promised one run, its Begin before any event and
// the events in Seq order, and a log may hold other runs, of an earlier
// child in the rotated copy of the log, say: the lines of every other run,
// and the events of the run before its first line, are dropped, and
// counted by Dropped. A line Decode refuses, of a newer version of the log
// among them, is skipped, and counted by Skipped.
//
// The Forwarder gives the sink each event of the run once, in the order of
// its Seq: an event whose Seq is not past the last it gave, as the log
// read again after a Reset has, is left out, so that a Reset changes
// nothing. A sink that is a progress.RunSink is told of the run at its
// first line, before its events, with the trace, the program and the
// version the line names.
//
// The sink is called from the goroutine that polls the Tail, never while
// a Bus is locked, and is given the events as Decode gives them: without
// the text of a line of output, and with only the advances the log wrote.
// A Forwarder is not safe for concurrent use.
type Forwarder struct {
	sink  progress.Sink
	trace progress.TraceID
	// run is the run the Forwarder gives the sink, "" until it begins;
	// seq is the Seq of the last event given, and any says one was.
	run string
	seq uint64
	any bool
	// dropped and skipped are what Dropped and Skipped count.
	dropped, skipped int
}

// NewForwarder returns a Forwarder that gives s the events of the first
// run of trace; a zero trace takes the first run that begins, whatever
// its trace.
func NewForwarder(s progress.Sink, trace progress.TraceID) *Forwarder {
	return &Forwarder{sink: s, trace: trace}
}

// Forward returns a Feed that gives s the events of the first run that
// begins in the lines it takes, as NewForwarder with a zero trace does.
//
// Deprecated: Use NewForwarder, which returns a *Forwarder, as the other
// constructors return their type, and forwards the run of the trace the
// server handed the child, not of whichever began first (decision 36).
func Forward(s progress.Sink) Feed { return NewForwarder(s, progress.TraceID{}) }

// Take gives the sink what the line holds, when it is of the run the
// Forwarder gives, and was not given before.
func (f *Forwarder) Take(b []byte) {
	l, err := Decode(b)
	if err != nil {
		f.skipped++
		return
	}
	if f.run == "" {
		if !l.Begin || f.trace != (progress.TraceID{}) && l.Trace.ID != f.trace {
			f.dropped++
			return
		}
		f.run = l.RunID
		if rs, ok := f.sink.(progress.RunSink); ok {
			rs.Begin(progress.RunInfo{Trace: l.Trace, Program: l.Program, Version: l.Version})
		}
		return
	}
	switch {
	case l.RunID != f.run:
		f.dropped++
	case l.Begin, f.any && l.Event.Seq <= f.seq:
		// The run's first line again, or an event given before.
	default:
		f.seq, f.any = l.Event.Seq, true
		f.sink.Handle(l.Event)
	}
}

// Reset changes nothing: the run stays the one the Forwarder gives, and
// the lines of it read again are left out by their Seq.
func (f *Forwarder) Reset() {}

// Dropped counts the lines the Forwarder dropped as not of its run: those
// of other runs, and those of its run before the run's first line.
func (f *Forwarder) Dropped() int { return f.dropped }

// Skipped counts the lines the Forwarder could not read, those Decode
// refused: a line of a newer version of the log, for one, which a server
// that forwards nothing can name as the reason.
func (f *Forwarder) Skipped() int { return f.skipped }
