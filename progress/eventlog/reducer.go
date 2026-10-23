// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

const (
	// defaultKeep is how many runs a Reducer keeps unless
	// Options.Keep says otherwise.
	defaultKeep = 100
	// defaultMaxSpans is how many spans of a run a Reducer keeps unless
	// Options.MaxSpans says otherwise.
	defaultMaxSpans = 1 << 17
)

// Options configure a Reducer.
type Options struct {
	// Keep is how many runs the Reducer keeps: the newest, and of the
	// older ones those that have not ended. 0 is 100.
	Keep int
	// MaxSpans is how many spans of a run the Reducer keeps; the spans
	// that start after them are left out, and so are their events. 0 is
	// 131072.
	MaxSpans int
}

// Run is one run of a log as a Reducer folds it: what its first line
// says, and the tree of the spans of its command.
type Run struct {
	// ID names the run, as the log does.
	ID string
	// Trace is the trace the run's spans belong to, and where it came
	// from, as the run's first line says.
	Trace progress.TraceContext
	// Program and Version are the program that wrote the run and its
	// version, as the run's first line says.
	Program, Version string
	// Root is the run's command, the first span of the kind
	// progress.KindCommand, with the spans below it.
	Root Span
	// Last is the time of the run's latest event. A run that has not
	// ended and has not changed for long was most likely killed: it will
	// not end.
	Last time.Time
	// Suspended says the displays of the run are off the terminal, while
	// the command asks a question there.
	Suspended bool
}

// Done reports whether the run's command has ended.
func (r Run) Done() bool { return r.Root.Done() }

// Span is a span as its events left it.
type Span struct {
	// ID is the span, and Parent the span it was started under; zero for
	// the root.
	ID, Parent progress.SpanID
	// Kind, Name, Flags and State are the span's, as its latest event
	// says.
	Kind  progress.Kind
	Name  string
	Flags progress.Flags
	State progress.State
	// Fields are the span's fields, as its latest event says. Exit, when
	// set, points to a value that is never written through.
	progress.Fields
	// Status, Class and Err say how the span ended.
	Status progress.Status
	Class  progress.Class
	Err    string
	// Queued is when the span started, queued or not; Started when its
	// work started, zero while it is queued; Ended when it ended, zero
	// while it has not.
	Queued, Started, Ended time.Time
	// Lines counts the lines of output the span had while a display
	// asked for them, whose text the log never holds, and Dropped those
	// that were not sent.
	Lines, Dropped int
	// Children are the spans started under this one, in the order they
	// started. A span whose parent the log does not hold, such as one
	// left out by Options.MaxSpans, is the root's.
	Children []Span
}

// Done reports whether the span has ended.
func (s Span) Done() bool { return s.State == progress.StateEnded }

// Reducer folds the lines of event logs into runs, each the tree of the
// spans of its command, for a view of what a program did, as it does it.
// The lines of several runs may come mixed, as when commands append to
// one log at once. A Reducer is a Feed, for a Tail. It is not safe for
// concurrent use.
type Reducer struct {
	keep, maxSpans int
	runs           map[string]*run
	// order lists the runs as their first line came, oldest first.
	order []*run
	// gone holds the runs that were dropped, so that a late line of one
	// does not bring it back without its start.
	gone map[string]struct{}
	// changed holds the runs that changed since Changed was last called,
	// each by the place it came in, and next is the place of the next.
	changed map[string]int
	next    int
}

// run is a run as the Reducer builds it: its spans without their
// children.
type run struct {
	id               string
	place            int
	trace            progress.TraceContext
	program, version string
	spans            map[progress.SpanID]*Span
	// started lists the spans in the order they started.
	started   []*Span
	root      *Span
	last      time.Time
	suspended bool
	// snap is the run as Runs returns it, good while dirty is false.
	snap  Run
	dirty bool
}

// NewReducer returns a Reducer that holds no run.
func NewReducer(o Options) *Reducer {
	return &Reducer{
		keep:     cmp.Or(max(o.Keep, 0), defaultKeep),
		maxSpans: cmp.Or(max(o.MaxSpans, 0), defaultMaxSpans),
		runs:     map[string]*run{},
		gone:     map[string]struct{}{},
		changed:  map[string]int{},
	}
}

// Line folds one line of a log in. A line Decode refuses is left out, and
// so is a line of a run the Reducer has dropped, and an event of a span
// that has ended.
func (r *Reducer) Line(b []byte) {
	l, err := Decode(b)
	if err != nil {
		return
	}
	if _, dropped := r.gone[l.RunID]; dropped {
		return
	}
	ru := r.runs[l.RunID]
	if ru == nil {
		ru = r.add(l.RunID)
	}
	ru.dirty = true
	r.changed[ru.id] = ru.place
	if l.Begin {
		ru.trace, ru.program, ru.version = l.Trace, l.Program, l.Version
		return
	}
	if ru.trace.ID == (progress.TraceID{}) {
		ru.trace.ID = l.Trace.ID
	}
	e := l.Event
	if e.Time.After(ru.last) {
		ru.last = e.Time
	}
	switch e.Type {
	case progress.TypeSuspend:
		ru.suspended = true
	case progress.TypeResume:
		ru.suspended = false
	default:
		ru.event(e, r.maxSpans)
	}
}

// Reset forgets every run, as when the log was cut short and is read
// again from its start; Changed reports each of them.
func (r *Reducer) Reset() {
	for _, ru := range r.order {
		r.changed[ru.id] = ru.place
	}
	clear(r.runs)
	clear(r.gone)
	r.order = nil
}

// Changed returns the IDs of the runs that changed, or were dropped,
// since the last call, the oldest run first, and forgets them.
func (r *Reducer) Changed() []string {
	ids := slices.SortedFunc(maps.Keys(r.changed), func(a, b string) int {
		return cmp.Compare(r.changed[a], r.changed[b])
	})
	clear(r.changed)
	return ids
}

// Runs returns the runs that have a command, the newest first. The
// Reducer never changes a Run it returned, but builds a run anew once it
// changes, so a Run may be kept and read from another goroutine; Runs
// returns the same Run again while it has not changed, its children
// shared, so a caller does not change what it was given.
func (r *Reducer) Runs() []Run {
	var out []Run
	for _, ru := range slices.Backward(r.order) {
		if ru.root == nil {
			continue
		}
		if ru.dirty {
			ru.snap, ru.dirty = ru.build(), false
		}
		out = append(out, ru.snap)
	}
	return out
}

// add adds a run, and drops the runs beyond the limit.
func (r *Reducer) add(id string) *run {
	ru := &run{id: id, place: r.next, spans: map[progress.SpanID]*Span{}}
	r.next++
	r.runs[id] = ru
	r.order = append(r.order, ru)
	for len(r.order) > r.keep {
		// The oldest run that has ended goes first, else the oldest.
		i := max(slices.IndexFunc(r.order, func(ru *run) bool { return ru.root != nil && ru.root.Done() }), 0)
		gone := r.order[i]
		r.order = slices.Delete(r.order, i, i+1)
		delete(r.runs, gone.id)
		r.changed[gone.id] = gone.place
		if len(r.gone) >= 16*r.keep {
			clear(r.gone)
		}
		r.gone[gone.id] = struct{}{}
	}
	return ru
}

// event folds an event of a span into the run.
func (ru *run) event(e progress.Event, maxSpans int) {
	sp := ru.spans[e.Span]
	if sp == nil {
		if len(ru.spans) >= maxSpans {
			return
		}
		sp = &Span{ID: e.Span, Parent: e.Parent, Queued: e.Time}
		if e.State != progress.StateQueued {
			sp.Started = e.Time
		}
		ru.spans[sp.ID] = sp
		ru.started = append(ru.started, sp)
		if ru.root == nil && e.Kind == progress.KindCommand {
			ru.root = sp
		}
	}
	if sp.Done() {
		return
	}
	// Every event of a span says all the span says by then.
	sp.Kind, sp.Name, sp.Flags, sp.State, sp.Fields = e.Kind, e.Name, e.Flags, e.State, e.Fields
	switch e.Type {
	case progress.TypeRun:
		if sp.Started.IsZero() {
			sp.Started = e.Time
		}
	case progress.TypeLine:
		sp.Lines++
		sp.Dropped += e.Dropped
	case progress.TypeEnd:
		sp.State, sp.Ended = progress.StateEnded, e.Time
		sp.Status, sp.Class, sp.Err, sp.Dropped = e.Status, e.Class, e.Err, e.Dropped
	}
}

// build makes the Run of the spans the run holds: their tree by parent,
// children in the order they started, and the spans whose parent it does
// not hold below the root.
func (ru *run) build() Run {
	kids := make(map[progress.SpanID][]*Span, len(ru.started))
	for _, sp := range ru.started {
		if sp == ru.root {
			continue
		}
		parent := sp.Parent
		if _, known := ru.spans[parent]; !known || parent == sp.ID {
			parent = ru.root.ID
		}
		kids[parent] = append(kids[parent], sp)
	}
	return Run{ID: ru.id, Trace: ru.trace, Program: ru.program, Version: ru.version,
		Root: tree(ru.root, kids), Last: ru.last, Suspended: ru.suspended && !ru.root.Done()}
}

// tree returns a copy of sp with the spans below it as its children.
func tree(sp *Span, kids map[progress.SpanID][]*Span) Span {
	out := *sp
	out.Children = nil
	for _, c := range kids[sp.ID] {
		out.Children = append(out.Children, tree(c, kids))
	}
	return out
}
