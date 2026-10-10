// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"cmp"
	"fmt"
	"sync"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-nodeset"
)

// Summary is the one line a display leaves behind once the command has
// ended: how the command ended, how long it ran, and how many of the things
// it worked on ended how.
//
//	provision reinstall: failed in 18m03s: 478 ok, 2 failed
//
// How the command ended comes first, since the counts need not say it: a
// command that fails in a step that counts nothing, or stops before the
// steps that would have counted the rest, can have counted nothing but ok.
// Each node, or whatever else a target names, counts once, as the worst of
// the ways its targets ended below the counted steps: failed, then
// canceled, then skipped, then ok, so that a node reset after its boot
// override was set is one node, not two. A batch left out, after one that
// failed or for an interrupt, counts as its nodes, each once with the
// rest, when its Node names as many as its Total, and otherwise as its
// Total. A command that counted no targets says only how it ended. The
// line says nothing of why a target failed: the command's error says that,
// once, after it.
//
// When the work the command's spans reported, rolled up by a
// progress.Meter, is all in one unit, the line ends with the total amount
// and the average rate, which the targets' counts need not have:
//
//	deploy: failed in 20m33s: 479 ok, 1 failed, 958 GiB at 796 MiB/s
//
// Work in two units says nothing, as amounts in different units are never
// added, and work in progress.Percent has no amount to say.
//
// In a Theme, the line takes the theme's colours and, in front, the mark of
// how the command ended, and keeps its words and punctuation. In Classic:
//
//	✗ provision reinstall: failed in 18m03s: 478 ok, 2 failed
//
// A Summary is a progress.Sink; its methods are safe for concurrent use.
type Summary struct {
	// Theme draws the line in its colours, with the mark of how the
	// command ended in front; the zero Theme draws it as it has always
	// been drawn, with no mark and no escape code. Line reads it: set it
	// before the Summary is put on a Bus.
	Theme Theme
	// ASCII draws the mark of Theme in ASCII, for a locale that is not
	// UTF-8; with the zero Theme it changes nothing. Line reads it: set it
	// before the Summary is put on a Bus.
	ASCII bool

	mu    sync.Mutex
	tally progress.Tally
	// command is the span of the command, the root of the tree.
	command    progress.SpanID
	name       string
	start, end time.Time
	status     progress.Status
	// roots are the counted steps no counted span is above, and under
	// says which of those each open span is under.
	roots map[progress.SpanID]*outcomes
	under map[progress.SpanID]progress.SpanID
	// nodes are the worst way each node's targets ended, and left the
	// Totals of what was left out.
	nodes map[string]progress.Status
	left  outcomes
	// meter rolls up the work of the spans; worked is what the command's
	// came to, when it had any, and units the units of work the spans
	// reported.
	meter   progress.Meter
	worked  progress.Reading
	hasWork bool
	units   map[progress.Unit]struct{}
}

// outcomes counts targets by how they ended.
type outcomes struct{ failed, canceled, skipped int }

// Handle counts e in.
func (s *Summary) Handle(e progress.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.roots == nil {
		s.roots = map[progress.SpanID]*outcomes{}
		s.under = map[progress.SpanID]progress.SpanID{}
		s.nodes = map[string]progress.Status{}
		s.units = map[progress.Unit]struct{}{}
	}
	count, counted := s.tally.Add(e)
	worked, hasWork := s.meter.Add(e)
	if e.Unit != 0 {
		s.units[e.Unit] = struct{}{}
	}
	switch e.Type {
	case progress.TypeStart:
		s.begin(e)
	case progress.TypeEnd:
		if e.Span == s.command {
			s.worked, s.hasWork = worked, hasWork
		}
		s.finish(e, count, counted)
	}
}

func (s *Summary) begin(e progress.Event) {
	if e.Kind == progress.KindCommand && s.command == 0 {
		s.command, s.name, s.start = e.Span, e.Name, e.Time
	}
	root := s.under[e.Parent]
	counts := e.Kind == progress.KindBatch || e.Kind == progress.KindStep && e.Flags&progress.Fold != 0
	// A hidden step is not shown, and neither is what it counts, as the
	// counter does not show it.
	if root == 0 && counts && e.Flags&progress.Hidden == 0 {
		root = e.Span
		s.roots[root] = &outcomes{}
	}
	if root != 0 {
		s.under[e.Span] = root
	}
}

func (s *Summary) finish(e progress.Event, count progress.Count, counted bool) {
	if e.Span == s.command {
		s.end, s.status = e.Time, e.Status
	}
	root, ok := s.under[e.Span]
	if !ok {
		return
	}
	delete(s.under, e.Span)
	if e.Kind == progress.KindTarget {
		s.node(cmp.Or(e.Node, e.Name), e.Status)
		s.roots[root].add(e.Status, 1)
	}
	if !counted {
		return
	}
	if root != e.Span {
		// A batch or a Fold step left out before any target of its own
		// started counts as its Total, as Tally has it: as its nodes,
		// each once with the others, when it names as many as that.
		if left := e.Status == progress.StatusSkipped || e.Status == progress.StatusCanceled; left && count.Started == 0 {
			if set, err := nodeset.Parse(e.Node); err == nil && set.Len() == count.Total {
				for _, node := range set.Expand() {
					s.node(node, e.Status)
				}
				s.roots[root].add(e.Status, count.Total)
			}
		}
		return
	}
	// What the root counted that no target, nor node of a span left out,
	// ended as was left out too: the Total of a batch or a Fold step below
	// it that never ran and named no nodes. A root left out whole counts
	// nothing of its own, as Tally has it.
	seen := s.roots[root]
	s.left.failed += count.Failed - seen.failed
	s.left.canceled += count.Canceled - seen.canceled
	s.left.skipped += count.Skipped - seen.skipped
	delete(s.roots, root)
}

// node counts in that a target of node ended with status, the worst of
// those of its targets.
func (s *Summary) node(node string, status progress.Status) {
	if was, seen := s.nodes[node]; !seen || worse(status, was) {
		s.nodes[node] = status
	}
}

func (o *outcomes) add(status progress.Status, n int) {
	switch status {
	case progress.StatusFailed:
		o.failed += n
	case progress.StatusCanceled:
		o.canceled += n
	case progress.StatusSkipped:
		o.skipped += n
	}
}

// worse reports whether a target that ended a says more about how its node
// fared than one that ended b.
func worse(a, b progress.Status) bool {
	rank := func(s progress.Status) int {
		switch s {
		case progress.StatusFailed:
			return 3
		case progress.StatusCanceled:
			return 2
		case progress.StatusSkipped:
			return 1
		}
		return 0
	}
	return rank(a) > rank(b)
}

// Line returns the summary once the command has ended, if it is worth a
// line: when the command ran for a second or longer, as long as a display
// takes to show itself, so that a command done within its first second
// leaves standard error as it would without a display, failed targets or
// not; its error says what failed. Otherwise, and before the command has
// ended, it is "".
func (s *Summary) Line() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.command == 0 || s.end.IsZero() {
		return ""
	}
	var ok int
	all := s.left
	for _, status := range s.nodes {
		if status == progress.StatusOK {
			ok++
		}
		all.add(status, 1)
	}
	d := s.end.Sub(s.start)
	if d < time.Second {
		return ""
	}
	l := lookOf(s.Theme, s.ASCII)
	line := fmt.Sprintf("%s: %s in %s", l.paint(roleTitle, s.name), wordIn(l, s.status), l.paint(roleMuted, took(d)))
	if s.Theme.art != nil {
		line = l.endMark(s.status) + " " + line
	}
	sep := ": "
	if ok+all.failed+all.canceled+all.skipped > 0 {
		line += sep + tallied(l, ok, all.failed, all.canceled, all.skipped)
		sep = ", "
	}
	// The work of the command is its amount and its mean rate, when all of
	// it is in one unit: amounts in different units are never added.
	if s.hasWork && len(s.units) == 1 {
		if done := endedText(l, s.worked); done != "" {
			line += sep + done
		}
	}
	return line
}

// took reads a length of time the way a person says it: tenths of a second,
// rounded, below ten seconds, then whole seconds, minutes and seconds, and
// hours, minutes and seconds. A time that rounds to ten seconds reads 10s,
// not 10.0s, as the times that follow it do.
func took(d time.Duration) string {
	d = max(0, d)
	s := int(d / time.Second)
	switch {
	case d < 9950*time.Millisecond:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < 10*time.Second:
		return "10s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", s)
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh%02dm%02ds", s/3600, s/60%60, s%60)
}
