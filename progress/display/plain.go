// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

const (
	// plainBeat is how often a Plain says how far each counted step under
	// way has got.
	plainBeat = 10 * time.Second
	// plainEvery is how often a Plain looks whether one is due.
	plainEvery = time.Second
)

// Plain is a display of plain lines, one for each thing worth a line, with
// the time since the command started in front and no escape codes unless
// its Theme draws in colour, for a log as much as for a terminal:
//
//	[0:00] bmc power › power off: start, 480 targets, 8 at a time
//	[0:03] bmc power › power off › exe0007 failed (transport): exe0007.mgmt: dial tcp: i/o timeout
//	[0:10] bmc power › power off: 312/480 done, 1 failed, 8 running, 160 queued
//	[0:18] bmc power › power off: failed in 18s: 478 ok, 2 failed
//
// A line names the span it tells of by the path to it from the command,
// its parts joined by "›", or by ">" with PlainOptions.ASCII. It
// is written as a step or a batch starts and ends, as a pause of a known
// length starts, and as a wait fails or is interrupted; for each
// target that fails, once; and, every ten seconds, for each counted step
// under way, a root of a progress.Tally, with how far it has got. The line
// that starts a step or a batch says how many targets it expects, in the
// word PlainOptions.Noun gives, and as "targets" without one. Hidden
// spans and calls get no line, and neither do the lines of output the work
// prints: a command whose product that output is prints it itself. A step
// with no name, as a pool given none reports its targets under, names no
// part of a path: its lines name the span above it, it has none of its
// own under a batch, or another span that counts its targets, and with no
// span above it that has a name its lines name nothing:
//
//	[0:00] start, 2 targets
//
// In a theme, PlainOptions.Theme, a line takes the theme's colours and one
// mark after the time: that of what runs for the start of a step, a batch
// or a pause and for how far a step has got, and that of how it ended for
// a target that failed and for the end of a span. Its words, punctuation
// and path are those of no theme, which draws no mark, so that the line
// without its colours and its mark is the line a log of no theme shows. In
// Classic:
//
//	[0:03] ✗ bmc power › power off › exe0007 failed (transport): exe0007.mgmt: dial tcp: i/o timeout
//
// The lines go out through the Terminal: ahead of whatever the command
// writes after the events they tell of, never into a line the command has
// not ended, and not while a question is asked. A Plain is a progress.Sink
// and a progress.Suspender; its methods are safe for concurrent use.
type Plain struct {
	term  *Terminal
	now   func() time.Time
	start time.Time
	// between is what the parts of a span's path are written with between,
	// in the colour of a separator.
	between string
	// look is what the lines are drawn with, and themed says each starts
	// with a mark after the time.
	look   look
	themed bool
	// noun says how many targets n are.
	noun func(n int) string

	mu    sync.Mutex
	tally progress.Tally
	// spans are the open spans.
	spans map[progress.SpanID]*plainSpan
	// beats are the counted steps under way that no counted span is
	// above, in the order they started, with when each is due to say
	// how far it has got.
	beats []beat
	// lines are the lines not yet written, each ended by a newline.
	lines strings.Builder
	// suspended counts the Suspends not yet resumed, during which no
	// heartbeat falls due.
	suspended int

	// wake has the lines not yet written flushed by the goroutine Start
	// begins, sooner than its next tick.
	wake   chan struct{}
	ticker ticker
}

type plainSpan struct {
	kind progress.Kind
	// path names the span from the command down.
	path   string
	hidden bool
	// counts says the span counts the targets below it, and below says a
	// span above it does.
	counts, below bool
	// ran is when the span started running; zero while it waits.
	ran time.Time
}

type beat struct {
	span progress.SpanID
	due  time.Time
}

// PlainOptions configure a Plain.
type PlainOptions struct {
	// Now is the clock the heartbeats are read from, and the time in front
	// of a line counted from; nil is time.Now. The Bus's clock should be
	// the same.
	Now func() time.Time
	// ASCII writes the path to a span with ">" between its parts, for a
	// locale that is not UTF-8, rather than "›", and the marks of Theme in
	// ASCII, "*" for what runs.
	ASCII bool
	// Theme draws the lines in its colours, each with a mark after the
	// time in front, and with the words, punctuation and paths of no
	// theme. The zero Theme draws them as they have always been drawn,
	// with no mark and no escape code.
	Theme Theme
	// Noun says how many targets a step or a batch expects, n of them,
	// in the line that starts it, in the program's own word for them, such
	// as "480 hosts"; nil is "1 target" and "%d targets".
	Noun func(n int) string
}

// NewPlain returns a display of plain lines on term, which counts the time
// in front of its lines from now. It panics if term has carried a display
// before.
func NewPlain(term *Terminal, o PlainOptions) *Plain {
	l := lookOf(o.Theme, o.ASCII)
	between := " › "
	if o.ASCII {
		between = " > "
	}
	p := &Plain{term: term, now: o.Now, spans: map[progress.SpanID]*plainSpan{}, between: l.paint(roleMuted, between),
		look: l, themed: o.Theme.art != nil, noun: o.Noun, wake: make(chan struct{}, 1)}
	if p.now == nil {
		p.now = time.Now
	}
	if p.noun == nil {
		p.noun = targets
	}
	term.attach(p.take)
	p.start = p.now()
	return p
}

// Start writes the lines as they come, and the heartbeats as they fall due,
// until Close. Start does nothing if the Plain was already started or
// closed.
func (p *Plain) Start() { p.ticker.start(p.term, plainEvery, p.Draw, p.wake) }

// Draw writes the lines not yet written and the heartbeats due now, where
// the terminal lets it.
func (p *Plain) Draw() {
	now := p.now()
	func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.suspended == 0 {
			p.heartbeats(now)
		}
	}()
	p.term.flush()
}

// Close stops the writing Start began, and writes what is left. Closing a
// closed Plain does nothing.
func (p *Plain) Close() { p.ticker.close(p.term) }

// Suspend writes the lines not yet written, and then no more until Resume.
func (p *Plain) Suspend() {
	p.mu.Lock()
	p.suspended++
	p.mu.Unlock()
	p.term.suspend()
}

// Resume lets the lines out again.
func (p *Plain) Resume() {
	p.mu.Lock()
	if p.suspended > 0 {
		p.suspended--
	}
	p.mu.Unlock()
	p.term.resume()
}

// take returns the lines not yet written, and forgets them. The terminal's
// lock is held.
func (p *Plain) take() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	text := p.lines.String()
	p.lines.Reset()
	return text
}

// Handle turns e into its line, if it is worth one.
func (p *Plain) Handle(e progress.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	count, counted := p.tally.Add(e)
	switch e.Type {
	case progress.TypeStart:
		p.begin(e)
	case progress.TypeRun:
		if s := p.spans[e.Span]; s != nil {
			p.run(e, s)
		}
	case progress.TypeEnd:
		if s := p.spans[e.Span]; s != nil {
			delete(p.spans, e.Span)
			p.end(e, s, count, counted)
		}
	}
}

// begin keeps a span that starts, and says so when it starts running.
func (p *Plain) begin(e progress.Event) {
	s := &plainSpan{
		kind:   e.Kind,
		hidden: e.Flags&progress.Hidden != 0,
		counts: e.Kind == progress.KindBatch || e.Kind == progress.KindStep && e.Flags&progress.Fold != 0,
	}
	parent := p.spans[e.Parent]
	if parent != nil {
		s.path = parent.path
		s.below = parent.counts || parent.below
	}
	// A call names no part of the path: nothing below one gets a line of
	// its own but through its target. Nor does a step with no name, as a
	// pool given none reports its targets under: it speaks under the path
	// above it, unless a span above it counts its targets and so speaks
	// for them.
	unnamed := e.Kind == progress.KindStep && e.Name == ""
	if unnamed && s.below {
		s.hidden = true
	}
	if e.Kind != progress.KindCall && !unnamed {
		name := e.Name
		if e.Kind == progress.KindBatch {
			name = "batch " + name
		}
		if s.path != "" {
			name = s.path + p.between + name
		}
		s.path = name
	}
	p.spans[e.Span] = s
	if e.State == progress.StateRunning {
		p.run(e, s)
	}
}

// run says that a span has started running.
func (p *Plain) run(e progress.Event, s *plainSpan) {
	s.ran = e.Time
	if s.hidden {
		return
	}
	switch e.Kind {
	case progress.KindStep, progress.KindBatch:
		p.say(e.Time, p.runMark(), s.path, p.look.paint(roleRunning, "start")+p.sizes(e.Fields))
		if s.counts && !s.below {
			p.beats = append(p.beats, beat{span: e.Span, due: e.Time.Add(plainBeat)})
		}
	case progress.KindWait:
		if e.Timeout > 0 {
			p.say(e.Time, p.runMark(), s.path, p.look.paint(roleMuted, fmt.Sprintf("waiting %s", e.Timeout)))
		}
	}
}

// end says how a span ended, if that is worth a line.
func (p *Plain) end(e progress.Event, s *plainSpan, count progress.Count, counted bool) {
	for i, b := range p.beats {
		if b.span == e.Span {
			p.beats = append(p.beats[:i], p.beats[i+1:]...)
			break
		}
	}
	if s.hidden {
		return
	}
	switch e.Kind {
	case progress.KindTarget:
		if e.Status == progress.StatusFailed {
			text := s.path + " " + outcome(p.look, e)
			if e.Err != "" {
				text += ": " + e.Err
			}
			p.line(e.Time, p.endMark(e.Status), text)
		}
	case progress.KindStep, progress.KindBatch:
		if s.ran.IsZero() {
			// Left out before it ran, as the batches after one that
			// failed are.
			p.say(e.Time, p.endMark(e.Status), s.path, outcome(p.look, e))
			return
		}
		text := fmt.Sprintf("%s in %s", wordIn(p.look, e.Status), p.look.paint(roleMuted, took(e.Time.Sub(s.ran))))
		if counted {
			text += ": " + ended(p.look, count)
		}
		p.say(e.Time, p.endMark(e.Status), s.path, text)
	case progress.KindWait:
		if e.Status == progress.StatusFailed || e.Status == progress.StatusCanceled {
			p.say(e.Time, p.endMark(e.Status), s.path, outcome(p.look, e))
		}
	}
}

// heartbeats says, for each counted step under way that is due, how far it
// has got. p.mu is held.
func (p *Plain) heartbeats(now time.Time) {
	for i := range p.beats {
		b := &p.beats[i]
		if now.Before(b.due) {
			continue
		}
		for !now.Before(b.due) {
			b.due = b.due.Add(plainBeat)
		}
		s := p.spans[b.span]
		count, ok := p.tally.Count(b.span)
		if s == nil || !ok {
			continue
		}
		p.say(now, p.runMark(), s.path, standing(p.look, count))
	}
}

// line adds a line at t to those not yet written, mark and text after the
// time, and has them written. p.mu is held.
func (p *Plain) line(t time.Time, mark, text string) {
	stamp := p.look.paint(roleClock, "["+elapsed(max(0, t.Sub(p.start)))+"]")
	fmt.Fprintf(&p.lines, "%s %s%s\n", stamp, mark, text)
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// say writes the line that says text of the span path names, after mark
// and its path when there is one. p.mu is held.
func (p *Plain) say(t time.Time, mark, path, text string) {
	if path != "" {
		text = path + ": " + text
	}
	p.line(t, mark, text)
}

// mark returns what a line starts with after the time in a theme: mark
// and a space; "" with no theme, whose lines have no mark.
func (p *Plain) mark(mark string) string {
	if !p.themed {
		return ""
	}
	return mark + " "
}

// runMark returns the mark of a line that tells of what runs: a start,
// and how far a step has got.
func (p *Plain) runMark() string { return p.mark(p.look.mark(roleRunning, p.look.running)) }

// endMark returns the mark of a line that tells of what ended with status.
func (p *Plain) endMark(status progress.Status) string { return p.mark(p.look.endMark(status)) }

// sizes says how many targets a step or a batch expects, and how many of
// them it works on at once when that is fewer.
func (p *Plain) sizes(f progress.Fields) string {
	if f.Total <= 0 {
		return ""
	}
	text := ", " + p.noun(f.Total)
	if f.Limit > 0 && f.Limit < f.Total {
		text += fmt.Sprintf(", %d at a time", f.Limit)
	}
	return text
}

// targets is the noun of a Plain that was given none: n targets.
func targets(n int) string {
	if n == 1 {
		return "1 target"
	}
	return fmt.Sprintf("%d targets", n)
}

// standing says how far a counted step has got while it is under way, each
// count of targets in the colour l gives what they are doing.
func standing(l look, c progress.Count) string {
	var parts []string
	if c.Batch != "" {
		parts = append(parts, "batch "+c.Batch)
	}
	parts = append(parts, fmt.Sprintf("%d/%d done", c.Done, c.Total))
	parts = append(parts, l.counts(c)...)
	if c.Waits > 0 {
		parts = append(parts, l.paint(roleMuted, "waiting"))
	}
	return strings.Join(parts, ", ")
}

// ended says how the targets of a counted step or batch ended, each count
// in the colour l gives how its targets ended.
func ended(l look, c progress.Count) string {
	return tallied(l, c.Done-c.Failed-c.Canceled-c.Skipped, c.Failed, c.Canceled, c.Skipped)
}

// tallied says how many ended how: "478 ok, 2 failed", leaving out none
// but ok, each count in the colour l gives how its targets ended, but for
// "0 ok", which is uncoloured: nothing ended well.
func tallied(l look, ok, failed, canceled, skipped int) string {
	okWord := fmt.Sprintf("%d ok", ok)
	if ok > 0 {
		okWord = l.paint(roleOK, okWord)
	}
	parts := append([]string{okWord}, l.counts(progress.Count{Failed: failed, Canceled: canceled, Skipped: skipped})...)
	return strings.Join(parts, ", ")
}

// outcome says in a word how a span ended that has no count to say, with
// the reason when it was left out: that is no failure, which report prints.
// The word, and the class of a failure, are in the colour l gives how the
// span ended; a reason stays uncoloured.
func outcome(l look, e progress.Event) string {
	switch e.Status {
	case progress.StatusFailed:
		return l.paint(roleFailed, fmt.Sprintf("failed (%s)", e.Class))
	case progress.StatusSkipped:
		if e.Err != "" {
			return l.paint(roleSkipped, "skipped") + ": " + e.Err
		}
	}
	return wordIn(l, e.Status)
}

// wordIn is word in the colour l gives how the span ended.
func wordIn(l look, s progress.Status) string { return l.paint(statusRole(s), word(s)) }

// word says in a word how a span ended.
func word(s progress.Status) string {
	switch s {
	case progress.StatusOK:
		return "done"
	case progress.StatusFailed:
		return "failed"
	case progress.StatusCanceled:
		return "canceled"
	case progress.StatusSkipped:
		return "skipped"
	}
	return s.String()
}
