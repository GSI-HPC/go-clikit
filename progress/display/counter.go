// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"cmp"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/termtext"
)

const (
	// counterDelay is how long a command runs before its counter is drawn:
	// a command that is done by then never shows one.
	counterDelay = time.Second
	// counterEvery is how often the counter is drawn at most.
	counterEvery = 100 * time.Millisecond
	// counterBar is how many cells wide the bar of a counted step is, in
	// a theme that draws bars.
	counterBar = 8
)

// Counter is a display of one line, which says for each step whose targets
// are counted, the roots of a progress.Tally, how far it has got, and how
// long the command has run:
//
//	power on · batch 3/60 · 17/480 · 1 failed · 8 running · 0:41.3
//
// Steps under way side by side each get a segment of their own, split by
// " | ". The time runs in tenths of a second, so the line changes at every
// frame. While no counted step is under way the line names the innermost
// step that is, or the command. Hidden steps are not shown. A step with no
// name, as a pool given none reports its targets under, goes by the name
// of the nearest step above it that has one, or of the command.
//
// In a Theme the line says the same in the theme's art and colours. It
// starts with the theme's spinner, which turns as the time runs, or with
// its running mark that stands still, in Classic and in ASCII; each
// counted step has the bar of the targets it has done before their count,
// those that failed at the end of it in the colour of a failure; and the
// theme's separators split the parts and the segments. Names are bold, the
// counts of targets in the colour of how they stand, the time in the
// theme's colour for the clock, and the separators and what is queued or
// waits muted. In Neon:
//
//	◶ power on ⋄ batch 3/60 ⋄ ▰▱▱▱▱▱▱▱ 17/480 ⋄ 1 failed ⋄ 8 running ⋄ 0:41.3
//
// A line that would not fit the terminal with its bars is drawn without
// any.
//
// A Counter is a progress.Sink and a progress.Suspender. Its methods are
// safe for concurrent use.
type Counter struct {
	term  *Terminal
	now   func() time.Time
	start time.Time
	g     look

	mu     sync.Mutex
	counts counting

	ticker ticker
}

// counting is what the line of a counter is drawn from, which a Tree keeps
// too, for the line it draws in its place on a terminal too small for it,
// so that each event is counted once for both. Its owner's lock guards it.
type counting struct {
	tally progress.Tally
	// named are the open steps that have a name and the command, the
	// newest last, which the line names when no counted step is under way.
	named []named
	// labels are what the line calls each open span other than a target:
	// its name for a step or the command that has one, otherwise the
	// label of the span above it.
	labels map[progress.SpanID]string
}

type named struct {
	span progress.SpanID
	name string
}

// CounterOptions configure a Counter.
type CounterOptions struct {
	// Now is the clock the time on the line is read from; nil is time.Now.
	Now func() time.Time
	// Theme is how the line looks; the zero Theme draws it as plain text,
	// as a Counter has always drawn it.
	Theme Theme
	// ASCII splits the parts of the line with " - " rather than " · ",
	// for a terminal whose locale is not UTF-8. In a Theme, the line keeps
	// the theme's colours, with a running mark that stands still, ">", and,
	// in a theme that draws bars, bars of [###.....].
	ASCII bool
}

// NewCounter returns a counter that draws on term once Start is called. The
// time on its line is counted from now. It panics if term has carried a
// display before.
func NewCounter(term *Terminal, o CounterOptions) *Counter {
	term.attach(nil)
	c := &Counter{term: term, now: o.Now, g: lookOf(o.Theme, o.ASCII)}
	if c.now == nil {
		c.now = time.Now
	}
	c.start = c.now()
	return c
}

// Start draws the counter every 100ms, from a second after it was made,
// until Close. Start does nothing if the Counter was already started or
// closed.
func (c *Counter) Start() { c.ticker.start(c.term, counterEvery, c.Draw, nil) }

// Draw draws the line as the work stands now, unless the command has run
// for less than a second.
func (c *Counter) Draw() {
	now := c.now()
	if now.Sub(c.start) < counterDelay {
		return
	}
	width, _ := c.term.dims()
	line := func() string {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.counts.line(now, c.start, c.g, width)
	}()
	c.term.draw([]string{line})
}

// Close stops the drawing and takes the line off the terminal. Closing a
// closed Counter does nothing.
func (c *Counter) Close() { c.ticker.close(c.term) }

// Handle counts e in.
func (c *Counter) Handle(e progress.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts.add(e)
}

// add counts e in, and returns what the tally says of the step or batch e
// counts towards, as progress.Tally.Add does.
func (c *counting) add(e progress.Event) (progress.Count, bool) {
	count, counted := c.tally.Add(e)
	if e.Kind == progress.KindTarget {
		return count, counted
	}
	titled := (e.Kind == progress.KindCommand || e.Kind == progress.KindStep) && e.Name != ""
	switch e.Type {
	case progress.TypeStart:
		if c.labels == nil {
			c.labels = map[progress.SpanID]string{}
		}
		label := c.labels[e.Parent]
		if titled {
			label = e.Name
		}
		c.labels[e.Span] = label
		if titled && e.Flags&progress.Hidden == 0 {
			c.named = append(c.named, named{e.Span, e.Name})
		}
	case progress.TypeEnd:
		delete(c.labels, e.Span)
		for i, n := range c.named {
			if n.span == e.Span {
				c.named = append(c.named[:i], c.named[i+1:]...)
				break
			}
		}
	}
	return count, counted
}

// Suspend takes the line off the terminal until Resume.
func (c *Counter) Suspend() { c.term.suspend() }

// Resume lets the line back.
func (c *Counter) Resume() { c.term.resume() }

// line is the line to draw at now, for a command that started at start, in
// the look g, on a terminal width columns wide: with the bars of the
// counted steps, unless they would take it past width-1 columns, where the
// terminal cuts it, and then without any.
func (c *counting) line(now, start time.Time, g look, width int) string {
	roots := c.tally.Roots()
	shown := roots[:0]
	for _, root := range roots {
		if root.Flags&progress.Hidden == 0 {
			// A step with no name is called by the span above it.
			root.Name = cmp.Or(root.Name, c.labels[root.Span])
			shown = append(shown, root)
		}
	}
	age := now.Sub(start)
	line := c.drawn(shown, age, g, true)
	if g.fill != "" && termtext.Width(visible(line)) > width-1 {
		line = c.drawn(shown, age, g, false)
	}
	return line
}

// drawn is the line of the counted steps roots, at age since the command
// started, in the look g, with their bars when bars is set.
func (c *counting) drawn(roots []progress.Count, age time.Duration, g look, bars bool) string {
	var segments []string
	for _, root := range roots {
		segments = append(segments, segment(root, g, bars))
	}
	if len(segments) == 0 && len(c.named) > 0 {
		segments = append(segments, g.paint(roleTitle, c.named[len(c.named)-1].name))
	}
	line := strings.Join(segments, g.paint(roleMuted, g.divider))
	if line != "" {
		line += g.paint(roleMuted, g.sep)
	}
	if g.lead {
		line = g.spinner(age) + " " + line
	}
	return line + g.paint(roleClock, clock(age))
}

// segment says how far one counted step has got, in the look g, its parts
// split by g's separator, after its name when it has one, and with the bar
// of its count before the count when bars is set and g draws one.
func segment(n progress.Count, g look, bars bool) string {
	var parts []string
	if n.Name != "" {
		parts = append(parts, g.paint(roleTitle, n.Name))
	}
	if n.Batch != "" {
		parts = append(parts, "batch "+n.Batch)
	}
	done := fmt.Sprintf("%d/%d", n.Done, n.Total)
	if bars {
		if bar := g.bar(n, counterBar); bar != "" {
			done = bar + " " + done
		}
	}
	parts = append(parts, done)
	parts = append(parts, g.counts(n)...)
	if n.Waits > 0 {
		parts = append(parts, g.paint(roleMuted, "waiting"))
	}
	return strings.Join(parts, g.paint(roleMuted, g.sep))
}

// elapsed reads d as minutes and seconds, or hours, minutes and seconds,
// as the time in front of a plain line: "0:03", "1:02:03".
func elapsed(d time.Duration) string {
	s := int(d / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// clock reads d as the clock of a live display, which a frame redraws
// every tenth of a second: minutes, seconds and tenths, or hours, minutes,
// seconds and tenths, "0:41.3", "1:02:03.4". The tenths are cut, not
// rounded, so that the clock never runs ahead of the time.
func clock(d time.Duration) string {
	t := int(max(0, d) / (time.Second / 10))
	s := t / 10
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d.%d", s/3600, s/60%60, s%60, t%10)
	}
	return fmt.Sprintf("%d:%02d.%d", s/60, s%60, t%10)
}
