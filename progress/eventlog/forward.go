// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog

import "github.com/GSI-HPC/go-clikit/progress"

// Forward returns a Feed that gives a sink the events of the lines it
// takes: what a server that runs a command as a child, with the child's
// event log in a file of its own, does to send on the child's progress,
// to an mcpprogress.Notifier for instance.
//
// The Feed decodes each line, and leaves out those Decode refuses. It
// gives the sink each event once, in the order of its run's Seq: an
// event of a run whose Seq is not past the last it gave, as the log read
// again after a Reset has, is left out, so that Reset changes nothing. A
// sink that is a progress.TraceSink is told of each run once, at the first
// line of the run, before the run's events. The events of every run the
// log holds go to the sink; a log of its own for the child holds one.
//
// The sink is called from the goroutine that polls the Tail, never while
// a Bus is locked, and is given the events as Decode gives them: without
// the text of a line of output, and with only the advances the log
// wrote.
func Forward(s progress.Sink) Feed {
	return &forward{sink: s, runs: map[string]*forwarded{}}
}

// forward is the Feed Forward returns.
type forward struct {
	sink progress.Sink
	runs map[string]*forwarded
}

// forwarded is what forward keeps of a run: whether the sink was told of
// it, and the Seq of the last event it was given.
type forwarded struct {
	begun bool
	seq   uint64
	any   bool
}

// Line gives the sink what the line holds, unless it has given it before.
func (f *forward) Line(b []byte) {
	l, err := Decode(b)
	if err != nil {
		return
	}
	r := f.runs[l.Run]
	if r == nil {
		r = &forwarded{}
		f.runs[l.Run] = r
	}
	if l.Begin {
		if ts, ok := f.sink.(progress.TraceSink); ok && !r.begun {
			ts.Begin(progress.Run{Trace: l.Trace, Program: l.Program})
		}
		r.begun = true
		return
	}
	if r.any && l.Event.Seq <= r.seq {
		return
	}
	r.seq, r.any = l.Event.Seq, true
	f.sink.Handle(l.Event)
}

// Reset changes nothing: what is read again is left out by its Seq.
func (f *forward) Reset() {}
