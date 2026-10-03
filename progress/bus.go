// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// BusOptions configure a Bus.
type BusOptions struct {
	// Sinks receive every event, in the order given.
	Sinks []Sink
	// Now is the clock the events are stamped with; nil is time.Now.
	Now func() time.Time
	// Trace is the trace the spans belong to; zero draws one at random.
	Trace TraceID
	// Parent, TraceFlags and TraceState say where Trace came from when
	// another program handed it on, as ParseTraceContext reads them: the
	// span of that program's the work runs under, the trace flags and
	// the tracestate. They are recorded, for an event log, and change
	// nothing else; without a Trace they are ignored.
	Parent     SpanID
	TraceFlags byte
	TraceState string
	// PanicLog receives the stack of a sink that panicked, the front
	// end's diagnostics; nil is the process's standard error.
	PanicLog io.Writer
	// Program names the program in the line that says a sink panicked,
	// "prog: …", so that it is not read as a line of the work's;
	// empty leaves the name out.
	Program string
	// Classify is the fallback of the classes End gives the errors of
	// spans, asked for an error that none of Classify's first three rules
	// fits, such as a program's rule for its exit codes; nil is
	// ClassTarget.
	Classify func(error) Class
}

// Bus hands the events of one command's spans to its sinks. It is safe for
// concurrent use: every event is numbered and delivered under one lock, so
// each sink sees the same order.
type Bus struct {
	// suspending keeps one caller's Suspend or Resume calls to the sinks
	// from interleaving with another's, and guards holds.
	suspending sync.Mutex
	// holds are the displays sent a Suspend that returned, in the order
	// they were first sent one, with the Suspends not yet resumed at each
	// depth: those of a display taken off the Bus since are owed it all
	// the same.
	holds []*hold

	mu sync.Mutex
	// sinks are the sinks on the Bus, one entry for each time a sink is
	// listed in BusOptions.Sinks, and lines says that one of them asked for
	// lines.
	sinks    []*entry
	lines    bool
	now      func() time.Time
	panicLog io.Writer
	program  string
	classify func(error) Class
	trace    TraceContext
	base     uint64
	started  uint64
	seq      uint64
	// last is the newest of the open spans, which are linked in the order
	// they started, so that the innermost come last.
	last      *Span
	suspended int
	closed    bool
}

// entry is one listing of a sink on a Bus. The Bus tells sinks apart by
// their entries, never by comparing the sinks themselves, which panics for
// a sink of a type that cannot be compared, such as a func type or a struct
// value holding a slice.
type entry struct {
	sink Sink
	// lines says the sink asked for lines.
	lines bool
	// dead says the sink has been taken off the Bus; it is set under b.mu
	// and read outside it, while the displays are suspended or resumed.
	dead atomic.Bool
}

// NewBus returns a Bus that sends its events to o.Sinks.
func NewBus(o BusOptions) *Bus {
	b := &Bus{
		now:      o.Now,
		panicLog: o.PanicLog,
		program:  o.Program,
		classify: o.Classify,
		trace:    TraceContext{Trace: o.Trace, Parent: o.Parent, Flags: o.TraceFlags, State: o.TraceState},
	}
	if b.now == nil {
		b.now = time.Now
	}
	if b.panicLog == nil {
		b.panicLog = os.Stderr
	}
	if b.trace.Trace == (TraceID{}) {
		b.trace = TraceContext{}
		// crypto/rand never fails; it ends the process when it cannot.
		_, _ = rand.Read(b.trace.Trace[:])
	}
	// The base is drawn whatever the trace, so that runs that continue
	// one trace do not share span ids.
	var base [8]byte
	_, _ = rand.Read(base[:])
	b.base = binary.BigEndian.Uint64(base[:])
	for _, sink := range o.Sinks {
		b.sinks = append(b.sinks, &entry{sink: sink})
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.begin()
	b.ask()
	return b
}

// Trace returns the trace the Bus's spans belong to.
func (b *Bus) Trace() TraceID { return b.trace.Trace }

// TraceContext returns the trace the Bus's spans belong to, and where it
// came from.
func (b *Bus) TraceContext() TraceContext { return b.trace }

// begin tells the sinks that record the trace which one it is, and removes
// one that panics. b.mu is held.
func (b *Bus) begin() {
	for _, e := range b.sinks {
		ts, ok := e.sink.(TraceSink)
		if ok && !e.dead.Load() && !b.safely(func() { ts.Begin(b.trace) }) {
			b.kill(e)
		}
	}
	b.compact()
}

// Close ends every span still open as canceled, "not finished", the
// innermost first, and resumes a display still suspended, one taken off
// the Bus for panicking since too. Nothing is sent after Close; the sinks
// are not closed, which is for whoever made them to do once Close has
// returned. Closing a closed Bus does nothing.
func (b *Bus) Close() {
	b.suspending.Lock()
	defer b.suspending.Unlock()
	resumes, ok := b.close()
	if !ok {
		return
	}
	for depth := resumes - 1; depth >= 0; depth-- {
		b.release(depth)
	}
}

// close ends the spans still open and marks the Bus closed, and returns
// how many suspensions it ended, or false when the Bus was closed already.
func (b *Bus) close() (resumes int, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, false
	}
	for b.last != nil {
		b.last.end(StatusCanceled, ClassCanceled, notFinished, nil)
	}
	resumes = b.suspended
	for range resumes {
		b.emit(Event{Type: TypeResume})
	}
	b.suspended = 0
	b.closed = true
	return resumes, true
}

// notFinished is the error of a span ended because its parent or the Bus
// ended first.
const notFinished = "not finished"

// Span is one unit of work. A nil *Span is one nobody watches, and every
// method of it does nothing. The methods are safe for concurrent use.
type Span struct {
	bus    *Bus
	parent *Span
	id     SpanID
	kind   Kind
	name   string
	flags  Flags

	// The rest is guarded by bus.mu.
	state  State
	fields Fields
	// open counts the spans started under this one that have not ended.
	open int
	// prev and next link the Bus's open spans.
	prev, next *Span
	lines      *lineState
}

type busKey struct{}

type spanKey struct{}

// WithBus returns a context that carries b, so that the spans started under
// it are sent to b's sinks. A span of another Bus the context carries is no
// parent to them.
func WithBus(ctx context.Context, b *Bus) context.Context {
	if b == nil {
		return ctx
	}
	return context.WithValue(ctx, busKey{}, b)
}

// BusFrom returns the Bus ctx carries, or nil.
func BusFrom(ctx context.Context) *Bus {
	b, _ := ctx.Value(busKey{}).(*Bus)
	return b
}

// SpanFrom returns the innermost span ctx carries, or nil.
func SpanFrom(ctx context.Context) *Span {
	_, s := from(ctx)
	return s
}

// from returns the Bus ctx carries and its innermost span in it.
func from(ctx context.Context) (*Bus, *Span) {
	b := BusFrom(ctx)
	if b == nil {
		return nil, nil
	}
	s, _ := ctx.Value(spanKey{}).(*Span)
	if s != nil && s.bus != b {
		s = nil
	}
	return b, s
}

// Start begins a span under the innermost one ctx carries, and returns a
// context that carries the new span along with it. The span is running
// unless Queued is given. Without a Bus in ctx, once it is closed, or under
// a span that has ended, Start returns ctx itself and a nil span: what a
// worker starts after its target was ended for it, by the end of its step
// or of the command, belongs to nothing still open, and would outlive the
// span its ending ended.
func Start(ctx context.Context, k Kind, name string, opts ...Option) (context.Context, *Span) {
	b, parent := from(ctx)
	if b == nil {
		return ctx, nil
	}
	var o options
	o.apply(opts)
	o.Fields = sanitizeFields(o.Fields, Fields{})
	s := &Span{
		bus:    b,
		parent: parent,
		kind:   k,
		name:   Sanitize(name, MaxField),
		flags:  o.flags,
		state:  StateRunning,
		fields: o.Fields,
	}
	if o.queued {
		s.state = StateQueued
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || parent != nil && parent.state == StateEnded {
		return ctx, nil
	}
	if parent != nil {
		s.flags |= parent.flags & inherited
		parent.open++
	}
	b.started++
	s.id = SpanID(b.base + b.started)
	if s.id == 0 {
		b.started++
		s.id = SpanID(b.base + b.started)
	}
	s.prev = b.last
	if b.last != nil {
		b.last.next = s
	}
	b.last = s
	b.emit(s.event(TypeStart))
	return context.WithValue(ctx, spanKey{}, s), s
}

// Run marks a queued span as running, when its work takes its place in a
// pool. It does nothing to a span that is running or ended.
func (s *Span) Run() {
	if s == nil {
		return
	}
	b := s.bus
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || s.state != StateQueued {
		return
	}
	s.state = StateRunning
	b.emit(s.event(TypeRun))
}

// Update changes the Total and Message of a span that has not ended. A
// Total smaller than the one before is ignored, and so are other options.
func (s *Span) Update(opts ...Option) {
	if s == nil {
		return
	}
	b := s.bus
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || s.state == StateEnded {
		return
	}
	o := options{Fields: s.fields}
	o.apply(opts)
	s.fields.Total = max(s.fields.Total, o.Total)
	if o.Message != s.fields.Message {
		s.fields.Message = Sanitize(o.Message, MaxField)
	}
	b.emit(s.event(TypeUpdate))
}

// End ends a span with the outcome of its work: ok for a nil error,
// otherwise failed or canceled as Classify tells from err, with the
// Bus's BusOptions.Classify as its fallback, and err's text becomes the
// span's one-line Err. The options set the fields that are known only at
// the end, such as Exit. Only the first End or Skip of a span
// counts. Spans started under it that are still open end first, as
// canceled, "not finished".
func (s *Span) End(err error, opts ...Option) {
	if s == nil {
		return
	}
	b := s.bus
	status, class, text := StatusOK, ClassNone, ""
	if err != nil {
		class = Classify(err, b.classify)
		status = StatusFailed
		if class == ClassCanceled {
			status = StatusCanceled
		}
		text = Sanitize(err.Error(), MaxErr)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	s.end(status, class, text, opts)
}

// Skip ends a queued or running span as skipped, with reason as its Err:
// work left out on purpose, such as a batch after one that failed or a
// call a dry run only recorded. It is the span's End, and like End only
// counts the first time.
func (s *Span) Skip(reason string) {
	if s == nil {
		return
	}
	text := Sanitize(reason, MaxErr)
	b := s.bus
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	s.end(StatusSkipped, ClassNone, text, nil)
}

// end ends s, and the spans still open under it first. b.mu is held.
func (s *Span) end(status Status, class Class, text string, opts []Option) {
	b := s.bus
	if s.state == StateEnded {
		return
	}
	if s.open > 0 {
		// The innermost first: a span that started later comes later in
		// the list, and its own children have ended by the time it is
		// reached.
		for o := b.last; o != nil && s.open > 0; {
			prev := o.prev
			if o.under(s) {
				o.end(StatusCanceled, ClassCanceled, notFinished, nil)
			}
			o = prev
		}
	}
	dropped := s.flushLines()
	if len(opts) > 0 {
		o := options{Fields: s.fields}
		o.apply(opts)
		o.Total = max(o.Total, s.fields.Total)
		s.fields = sanitizeFields(o.Fields, s.fields)
	}
	s.state = StateEnded
	if s.prev != nil {
		s.prev.next = s.next
	}
	if s.next != nil {
		s.next.prev = s.prev
	} else {
		b.last = s.prev
	}
	s.prev, s.next = nil, nil
	if s.parent != nil {
		s.parent.open--
	}
	e := s.event(TypeEnd)
	e.Status, e.Class, e.Err, e.Dropped = status, class, text, dropped
	b.emit(e)
}

// under reports whether s was started below ancestor.
func (s *Span) under(ancestor *Span) bool {
	for p := s.parent; p != nil; p = p.parent {
		if p == ancestor {
			return true
		}
	}
	return false
}

// event returns an event about s as it is now. b.mu is held.
func (s *Span) event(t Type) Event {
	e := Event{
		Type:   t,
		Span:   s.id,
		Kind:   s.kind,
		Name:   s.name,
		Flags:  s.flags,
		State:  s.state,
		Fields: s.fields,
	}
	if s.parent != nil {
		e.Parent = s.parent.id
	}
	return e
}

// Suspend takes every display off the terminal, for a question to be asked
// there, and returns once they are all off. Calling the function it returns
// puts them back, a display taken off the Bus in between for panicking
// too; calling that again does nothing. Suspensions may nest, and without
// a Bus in ctx nothing happens.
func Suspend(ctx context.Context) (resume func()) {
	b, s := from(ctx)
	if b == nil {
		return func() {}
	}
	var id SpanID
	if s != nil {
		id = s.id
	}
	if !b.suspend(id) {
		return func() {}
	}
	var once sync.Once
	return func() { once.Do(func() { b.resume(id) }) }
}

func (b *Bus) suspend(id SpanID) bool {
	b.suspending.Lock()
	defer b.suspending.Unlock()
	depth, sus, ok := b.enterSuspension(id)
	if !ok {
		return false
	}
	for _, e := range sus {
		// A display listed again, whose Suspend just panicked, is off.
		if e.dead.Load() {
			continue
		}
		if b.safely(e.sink.(Suspender).Suspend) {
			b.hold(e, depth)
		} else {
			b.drop(e)
		}
	}
	return true
}

// enterSuspension counts one suspension more and sends its event, and
// returns its depth and the displays to suspend, or false when the Bus is
// closed.
func (b *Bus) enterSuspension(id SpanID) (depth int, sus []*entry, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, nil, false
	}
	b.emit(Event{Type: TypeSuspend, Span: id})
	b.suspended++
	return b.suspended, b.suspenders(), true
}

func (b *Bus) resume(id SpanID) {
	b.suspending.Lock()
	defer b.suspending.Unlock()
	if depth, ok := b.leaveSuspension(id); ok {
		b.release(depth)
	}
}

// leaveSuspension counts one suspension less and sends its event, and
// returns the depth left, or false when there is none to leave: Close has
// resumed whatever it found suspended.
func (b *Bus) leaveSuspension(id SpanID) (depth int, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.suspended == 0 {
		return 0, false
	}
	b.emit(Event{Type: TypeResume, Span: id})
	b.suspended--
	return b.suspended, true
}

// suspenders returns the sinks that draw on the terminal. b.mu is held.
func (b *Bus) suspenders() []*entry {
	var out []*entry
	for _, e := range b.sinks {
		if _, ok := e.sink.(Suspender); ok {
			out = append(out, e)
		}
	}
	return out
}

// hold is a display sent Suspends not yet resumed, with the depth of
// suspension each was sent at, the outermost first. A display listed more
// than once in BusOptions.Sinks has a hold for each time it is listed, and is
// sent a Suspend and a Resume for each.
type hold struct {
	display *entry
	depths  []int
}

// hold records a Suspend that the display e returned from at depth.
// b.suspending is held.
func (b *Bus) hold(e *entry, depth int) {
	i := slices.IndexFunc(b.holds, func(h *hold) bool { return h.display == e })
	if i < 0 {
		b.holds = append(b.holds, &hold{display: e})
		i = len(b.holds) - 1
	}
	b.holds[i].depths = append(b.holds[i].depths, depth)
}

// release resumes every display sent a Suspend deeper than the depth the
// Bus is left suspended at, outside b.mu, whether or not it is still on the
// Bus: one taken off for panicking while suspended would otherwise keep the
// terminal from its own output. A display that panics when resumed is taken
// off and called no more, under any of its listings. b.suspending is held.
func (b *Bus) release(depth int) {
	for _, h := range b.holds {
		last := len(h.depths) - 1
		// A display that panicked when resumed under another listing is
		// called no more.
		if last < 0 || h.depths[last] <= depth {
			continue
		}
		h.depths = h.depths[:last]
		if !b.safely(h.display.sink.(Suspender).Resume) {
			for _, gone := range b.drop(h.display) {
				b.forget(gone)
			}
		}
	}
	b.holds = slices.DeleteFunc(b.holds, func(h *hold) bool { return len(h.depths) == 0 })
}

// forget drops the Suspends still held for the display e. b.suspending is
// held.
func (b *Bus) forget(e *entry) {
	for _, h := range b.holds {
		if h.display == e {
			h.depths = nil
		}
	}
}

// drop takes the display e off the Bus, with its other listings, and
// returns them all. b.mu is not held.
func (b *Bus) drop(e *entry) []*entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	gone := b.kill(e)
	b.compact()
	return gone
}

// emit numbers ev and hands it to every sink. The clock is read first, so
// that one that panics leaves no number used. b.mu is held.
func (b *Bus) emit(ev Event) {
	ev.Time = b.now()
	b.seq++
	ev.Seq = b.seq
	removed := false
	for _, e := range b.sinks {
		if e.dead.Load() {
			continue
		}
		if !b.safely(func() { e.sink.Handle(ev) }) {
			b.kill(e)
			removed = true
		}
	}
	if removed {
		b.compact()
	}
}

// safely calls f and reports whether it returned rather than panicked; the
// stack of a panic goes to the panic log. A sink is part of how the work is
// shown, not of the work, so a sink that fails must not take the work with
// it.
func (b *Bus) safely(f func()) (ok bool) {
	defer func() {
		if v := recover(); v != nil {
			prefix := ""
			if b.program != "" {
				prefix = b.program + ": "
			}
			// The log is a courtesy; a write that fails changes nothing.
			_, _ = fmt.Fprintf(b.panicLog, "%sa progress display panicked and was stopped: %q\n%s", prefix, fmt.Sprint(v), debug.Stack())
			ok = false
		}
	}()
	f()
	return true
}

// kill marks the sink of e as taken off the Bus, with every other listing
// of the same sink, and returns them all; compact then removes them. A
// sink of a type that cannot be compared is known by its entry alone.
// b.mu is held.
func (b *Bus) kill(e *entry) []*entry {
	gone := []*entry{e}
	e.dead.Store(true)
	for _, o := range b.sinks {
		if o != e && sameSink(o.sink, e.sink) {
			o.dead.Store(true)
			gone = append(gone, o)
		}
	}
	return gone
}

// sameSink reports whether a and b are the same sink, and false for sinks
// of a type that cannot be compared, rather than panic.
func sameSink(a, b Sink) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

// compact removes the sinks taken off the Bus. b.mu is held.
func (b *Bus) compact() {
	b.sinks = slices.DeleteFunc(b.sinks, func(e *entry) bool { return e.dead.Load() })
	b.lines = slices.ContainsFunc(b.sinks, func(e *entry) bool { return e.lines })
}

// ask asks each sink once whether it wants the lines, and removes one that
// panics. b.mu is held.
func (b *Bus) ask() {
	for _, e := range b.sinks {
		ls, ok := e.sink.(LineSink)
		if !ok || e.dead.Load() {
			continue
		}
		want := false
		if !b.safely(func() { want = ls.WantsLines() }) {
			b.kill(e)
		} else {
			e.lines = want
		}
	}
	b.compact()
}

// sanitizeFields makes the text fields of f that differ from those of prev
// safe to show.
func sanitizeFields(f, prev Fields) Fields {
	clean := func(s *string, was string, limit int) {
		if *s != was {
			*s = Sanitize(*s, limit)
		}
	}
	// Node is a node set a display folds, so it is escaped but never cut.
	clean(&f.Node, prev.Node, 0)
	clean(&f.Host, prev.Host, MaxField)
	clean(&f.Role, prev.Role, MaxField)
	clean(&f.Batch, prev.Batch, MaxField)
	clean(&f.Message, prev.Message, MaxField)
	clean(&f.Method, prev.Method, MaxField)
	clean(&f.Path, prev.Path, MaxField)
	clean(&f.Cache, prev.Cache, MaxField)
	clean(&f.Source, prev.Source, MaxField)
	return f
}
