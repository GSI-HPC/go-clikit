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
)

const (
	// counterDelay is how long a command runs before its counter is drawn:
	// a command that is done by then never shows one.
	counterDelay = time.Second
	// counterEvery is how often the counter is drawn at most.
	counterEvery = 100 * time.Millisecond
)

// Counter is a display of one line, which says for each step whose targets
// are counted, the roots of a progress.Tally, how far it has got, and how
// long the command has run:
//
//	power on · batch 3/60 · 17/480 · 1 failed · 8 running · 0:41
//
// Steps under way side by side each get a segment of their own, split by
// " | ". While no counted step is under way the line names the innermost
// step that is, or the command. Hidden steps are not shown. A step with no
// name, as a pool given none reports its targets under, goes by the name
// of the nearest step above it that has one, or of the command.
//
// A Counter is a progress.Sink and a progress.Suspender. Its methods are
// safe for concurrent use.
type Counter struct {
	term  *Terminal
	now   func() time.Time
	start time.Time
	g     glyphs

	mu    sync.Mutex
	tally progress.Tally
	// named are the open steps that have a name and the command, the
	// newest last, which the line names when no counted step is under way.
	named []named
	// labels are what the line calls each open span other than a target:
	// its name for a step or the command that has one, otherwise the
	// label of the span above it.
	labels map[progress.SpanID]string

	ticker ticker
}

type named struct {
	span progress.SpanID
	name string
}

// CounterOptions configure a Counter.
type CounterOptions struct {
	// Now is the clock the time on the line is read from; nil is time.Now.
	Now func() time.Time
	// ASCII splits the parts of the line with " - " rather than " · ",
	// for a terminal whose locale is not UTF-8.
	ASCII bool
}

// NewCounter returns a counter that draws on term once Start is called. The
// time on its line is counted from now.
func NewCounter(term *Terminal, o CounterOptions) *Counter {
	c := &Counter{term: term, now: o.Now, g: glyphsFor(o.ASCII)}
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
	line := func() string {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.line(now)
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
	c.tally.Add(e)
	if e.Kind == progress.KindTarget {
		return
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
}

// Suspend takes the line off the terminal until Resume.
func (c *Counter) Suspend() { c.term.suspend() }

// Resume lets the line back.
func (c *Counter) Resume() { c.term.resume() }

// line is the line to draw at now. c.mu is held.
func (c *Counter) line(now time.Time) string {
	var segments []string
	for _, root := range c.tally.Roots() {
		if root.Flags&progress.Hidden == 0 {
			// A step with no name is called by the span above it.
			root.Name = cmp.Or(root.Name, c.labels[root.Span])
			segments = append(segments, segment(root, c.g.sep))
		}
	}
	if len(segments) == 0 && len(c.named) > 0 {
		segments = append(segments, c.named[len(c.named)-1].name)
	}
	line := strings.Join(segments, " | ")
	if line != "" {
		line += c.g.sep
	}
	return line + elapsed(now.Sub(c.start))
}

// segment says how far one counted step has got, its parts split by sep,
// after its name when it has one.
func segment(n progress.Count, sep string) string {
	var parts []string
	if n.Name != "" {
		parts = append(parts, n.Name)
	}
	if n.Batch != "" {
		parts = append(parts, "batch "+n.Batch)
	}
	parts = append(parts, fmt.Sprintf("%d/%d", n.Done, n.Total))
	for _, count := range []struct {
		n    int
		what string
	}{
		{n.Failed, "failed"},
		{n.Canceled, "canceled"},
		{n.Skipped, "skipped"},
		{n.Running, "running"},
		{n.Queued, "queued"},
	} {
		if count.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count.n, count.what))
		}
	}
	if n.Waits > 0 {
		parts = append(parts, "waiting")
	}
	return strings.Join(parts, sep)
}

// elapsed reads d as minutes and seconds, or hours, minutes and seconds.
func elapsed(d time.Duration) string {
	s := int(d / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
