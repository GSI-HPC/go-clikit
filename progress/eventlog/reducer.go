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
	// DefaultKeepRuns is how many runs a Reducer keeps unless
	// ReducerOptions.KeepRuns says otherwise.
	DefaultKeepRuns = 100
	// DefaultMaxSpans is how many spans of a run a Reducer keeps unless
	// ReducerOptions.MaxSpans says otherwise.
	DefaultMaxSpans = 1 << 17
)

// goneRuns is how many times KeepRuns the Reducer remembers of the runs it
// dropped.
const goneRuns = 16

// ReducerOptions configure a Reducer. A value of 0 or less is the default
// the field names.
type ReducerOptions struct {
	// KeepRuns is how many runs the Reducer keeps. Past it, the oldest run
	// that has ended goes first, else the oldest; a run whose command has
	// not started counts as one that has not ended. 0 is DefaultKeepRuns.
	KeepRuns int
	// MaxSpans is how many spans of a run the Reducer keeps; the spans
	// that start after them are left out, and so are their events. 0 is
	// DefaultMaxSpans.
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
	// Fields are the span's fields, as its latest event says.
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
	// left out by ReducerOptions.MaxSpans, is the root's.
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
	// gone holds the runs that were dropped, the last goneRuns times keep
	// of them, so that a late line of one does not bring it back without
	// its start. buried lists them in the order they went: a ring, whose
	// oldest entry is at the index oldest once it is full.
	gone   map[string]struct{}
	buried []string
	oldest int
	// changed holds the runs that changed since Changed was last called,
	// each by the place it came in, and next is the place of the next.
	changed map[string]int
	next    int
	// skipped counts the lines Decode refused.
	skipped int
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
func NewReducer(o ReducerOptions) *Reducer {
	return &Reducer{
		keep:     cmp.Or(max(o.KeepRuns, 0), DefaultKeepRuns),
		maxSpans: cmp.Or(max(o.MaxSpans, 0), DefaultMaxSpans),
		runs:     map[string]*run{},
		gone:     map[string]struct{}{},
		changed:  map[string]int{},
	}
}

// Take folds one line of a log in. A line Decode refuses is left out, and
// counted by Skipped, and so is a line of a run the Reducer has dropped,
// and an event of a span that has ended.
func (r *Reducer) Take(b []byte) {
	l, err := Decode(b)
	if err != nil {
		r.skipped++
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

// Skipped counts the lines the Reducer could not read, those Decode
// refused, since it was made: a view that shows no run can say why, as a
// line of a newer version of the log is one of them. A Reset does not
// change it.
func (r *Reducer) Skipped() int { return r.skipped }

// Reset is the notice that the log was cut short or made again, and is
// read again from its start: the Reducer forgets every run, the runs it
// dropped too, and Changed reports each run it held.
func (r *Reducer) Reset() {
	for _, ru := range r.order {
		r.changed[ru.id] = ru.place
	}
	clear(r.runs)
	clear(r.gone)
	r.buried, r.oldest = r.buried[:0], 0
	r.order = nil
}

// Changed returns the IDs of the runs that changed, or were dropped,
// since the last call, and forgets them. They come the oldest run first,
// in the order the runs came, as a view that keeps its own list of runs
// adds a new one after those it has; Runs lists them the other way.
func (r *Reducer) Changed() []string {
	ids := slices.SortedFunc(maps.Keys(r.changed), func(a, b string) int {
		return cmp.Compare(r.changed[a], r.changed[b])
	})
	clear(r.changed)
	return ids
}

// Runs returns the runs that have a command, the newest first, as a view
// lists them; Changed gives the oldest first. The Reducer never changes a
// Run it returned, but builds a run anew once it changes, so a Run may be
// kept and read from another goroutine; Runs returns the same Run again
// while it has not changed, its children shared, so a caller does not
// change what it was given.
func (r *Reducer) Runs() []Run {
	var out []Run
	for _, ru := range slices.Backward(r.order) {
		if ru.root != nil {
			out = append(out, ru.snapshot())
		}
	}
	return out
}

// Run returns the run id names, as Runs would, and false when the Reducer
// holds no such run: one it dropped, or one it never had a line of. A run
// whose command has not started yet is held, with a zero Root, so that a
// view that looks up each ID Changed gives can tell it from one that went.
func (r *Reducer) Run(id string) (Run, bool) {
	ru := r.runs[id]
	if ru == nil {
		return Run{}, false
	}
	return ru.snapshot(), true
}

// snapshot returns the run as Runs and Run return it, built anew when it
// changed.
func (ru *run) snapshot() Run {
	if ru.dirty {
		ru.snap, ru.dirty = ru.build(), false
	}
	return ru.snap
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
		r.bury(gone.id)
	}
	return ru
}

// bury remembers that the run id was dropped, and forgets the oldest run
// it remembers once it remembers goneRuns times keep of them.
func (r *Reducer) bury(id string) {
	if len(r.buried) < goneRuns*r.keep {
		r.buried = append(r.buried, id)
	} else {
		delete(r.gone, r.buried[r.oldest])
		r.buried[r.oldest] = id
		r.oldest = (r.oldest + 1) % len(r.buried)
	}
	r.gone[id] = struct{}{}
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
// not hold below the root. A run without a command has a zero Root.
func (ru *run) build() Run {
	out := Run{ID: ru.id, Trace: ru.trace, Program: ru.program, Version: ru.version, Last: ru.last}
	if ru.root == nil {
		return out
	}
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
	out.Root, out.Suspended = tree(ru.root, kids), ru.suspended && !ru.root.Done()
	return out
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
