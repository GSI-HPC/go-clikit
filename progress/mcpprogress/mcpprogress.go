// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package mcpprogress sends the progress of a call to an MCP tool, as the
// progress notifications of the Model Context Protocol: how many of the
// targets the call expects have ended, of how many, and a line for a
// person to read, such as "read the groups: 3/16 done, 1 failed".
//
// A Notifier is a progress.Sink. A server gives each call that came with a
// progress token a Bus of its own, with a Notifier among its sinks, runs
// the call under it, and once the handler has returned closes the Bus and
// then the Notifier, so that what is left is sent before the call's
// result and nothing after it. The package imports no MCP SDK: the server
// hands it a Send that puts the token in the notification and sends it on
// the session, so that the kit follows no release of an SDK (decision 3).
//
// What is expected is the Total of every step or batch that counts its
// targets with none above it that does, a root of a progress.Tally, that
// the call has started; it may grow as another starts, but never shrinks.
// What has ended is the targets ended below the roots under way, and the
// whole Total of each root that has ended, so it never goes back, never
// passes what is expected, and, once every root has ended, is all of it. A
// call that counts nothing sends nothing. Spans with FlagHidden are not
// counted, as a display does not show them.
//
// MCP asks that the progress of each notification be more than the one
// before: after the first, a notification is sent only once more targets
// have ended, and says what is expected and where the call stands by
// then. Notifications go out at most every Interval, and at once as a step
// or a batch ends, from a goroutine of the Notifier's, never while the
// Bus is locked. The line is about the root that started last, and, when
// the numbers count more than that root, ends with what they count in
// all: "read the uptime: 0/6 done; 6/12 in all".
//
// Interfaces beyond the Go API: Interval, the rule that the progress of a
// notification only grows, and the words of its line (decisions 27 and
// 32).
package mcpprogress

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// Interval is how often a Notifier sends at most, but as a step or a batch
// ends.
const Interval = 500 * time.Millisecond

// Options configure a Notifier.
type Options struct {
	// Send sends one progress notification: the targets ended, the targets
	// expected and the line. The server puts the call's progress token in
	// it and sends it on the call's session. Send is called from one
	// goroutine at a time, in the order the notifications are made. An
	// error, such as that of a session that has gone, stops the Notifier:
	// nothing more is sent for the call, and the call goes on.
	Send func(progress, total float64, message string) error
	// PanicLog receives the stack of a panic in Send or in the Notifier,
	// the server's log; nil is the process's standard error. Nothing more
	// is sent for the call after one, and the call and the server go on.
	PanicLog io.Writer
	// Program names the program in the line that says the notifications
	// stopped, "prog: …"; empty leaves the name out.
	Program string
}

// Notifier sends the progress of one call, as the package comment says.
// Its methods are safe for concurrent use.
type Notifier struct {
	send     func(progress, total float64, message string) error
	panicLog io.Writer
	program  string

	mu    sync.Mutex
	tally progress.Tally
	// roots are the roots of the tally that are shown and open.
	roots map[progress.SpanID]bool
	// ended is the Total of the roots that have ended, and last says how
	// the one that ended last stood; counted is how many roots have
	// started, ended or not.
	ended, counted int
	last           string
	// dirty says something changed since the last notification, and
	// urgent that a step or a batch ended.
	dirty, urgent bool
	// sent is what the last notification said, and sentAt when; any says
	// one was sent.
	sent   standing
	sentAt time.Time
	any    bool
	// stopped says a Send failed or panicked; closed that Close was called.
	stopped, closed bool

	wake chan struct{}
	stop chan struct{}
	done chan struct{}
}

// standing is what a notification says.
type standing struct {
	progress, total int
	message         string
}

// New returns a Notifier that sends with o.Send, and starts the goroutine
// it sends from, which Close stops.
func New(o Options) *Notifier {
	n := &Notifier{
		send:     o.Send,
		panicLog: o.PanicLog,
		program:  o.Program,
		roots:    map[progress.SpanID]bool{},
		wake:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	if n.panicLog == nil {
		n.panicLog = os.Stderr
	}
	go n.run()
	return n
}

// Handle counts e in, and has a notification sent if it changed anything.
func (n *Notifier) Handle(e progress.Event) {
	n.mu.Lock()
	defer n.mu.Unlock()
	count, counted := n.tally.Add(e)
	shown := e.Flags&progress.FlagHidden == 0
	switch e.Type {
	case progress.TypeStart:
		if _, ok := n.tally.Count(e.Span); ok && shown && isRoot(n.tally.Roots(), e.Span) {
			n.roots[e.Span] = true
			n.counted++
		}
	case progress.TypeEnd:
		if counted && n.roots[e.Span] {
			delete(n.roots, e.Span)
			n.ended += count.Total
			n.last = line(count)
		}
		if shown && (e.Kind == progress.KindStep || e.Kind == progress.KindBatch) {
			n.urgent = true
		}
	}
	n.dirty = true
	select {
	case n.wake <- struct{}{}:
	default:
	}
}

// isRoot reports whether span is one of roots.
func isRoot(roots []progress.Count, span progress.SpanID) bool {
	for _, root := range roots {
		if root.Span == span {
			return true
		}
	}
	return false
}

// run sends the notifications as they fall due, until Close.
func (n *Notifier) run() {
	defer close(n.done)
	var later <-chan time.Time
	for {
		select {
		case <-n.stop:
			return
		case <-n.wake:
		case <-later:
			later = nil
		}
		n.mu.Lock()
		if wait := time.Until(n.sentAt.Add(Interval)); n.dirty && !n.urgent && wait > 0 {
			if later == nil {
				later = time.After(wait)
			}
			n.mu.Unlock()
			continue
		}
		s, ok := n.due()
		n.mu.Unlock()
		if ok {
			n.deliver(s)
		}
	}
}

// Close stops the Notifier once the Bus it is a sink of has closed, and
// sends where the call stands then if that is more than was sent last.
// Nothing is sent once it has returned, so the server answers the call
// after it. A second Close does nothing.
func (n *Notifier) Close() {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return
	}
	n.closed = true
	n.mu.Unlock()
	close(n.stop)
	<-n.done
	n.mu.Lock()
	s, ok := n.due()
	n.mu.Unlock()
	if ok {
		n.deliver(s)
	}
}

// deliver sends s, unless a Send failed or panicked before. A Send that
// fails, or panics, stops the Notifier; the stack of a panic goes to the
// panic log.
func (n *Notifier) deliver(s standing) {
	defer func() {
		p := recover()
		if p == nil {
			return
		}
		n.mu.Lock()
		n.stopped = true
		n.mu.Unlock()
		prefix := ""
		if n.program != "" {
			prefix = n.program + ": "
		}
		// The log is a courtesy; a write that fails changes nothing.
		_, _ = fmt.Fprintf(n.panicLog, "%sprogress notifications stopped: %v\n%s", prefix, p, debug.Stack())
	}()
	if err := n.send(float64(s.progress), float64(s.total), s.message); err != nil {
		n.mu.Lock()
		n.stopped = true
		n.mu.Unlock()
	}
}

// due returns the notification to send, and false when nothing is counted,
// no more targets have ended since the last one, or the Notifier has
// stopped. n.mu is held.
func (n *Notifier) due() (standing, bool) {
	if !n.dirty || n.stopped {
		return standing{}, false
	}
	n.dirty, n.urgent = false, false
	now := standing{progress: n.ended, total: n.ended, message: n.last}
	for _, root := range n.tally.Roots() {
		if n.roots[root.Span] {
			now.progress += min(root.Done, root.Total)
			now.total += root.Total
			now.message = line(root)
		}
	}
	if now.total == 0 || n.any && now.progress <= n.sent.progress {
		return standing{}, false
	}
	if n.counted > 1 {
		now.message += fmt.Sprintf("; %d/%d in all", now.progress, now.total)
	}
	n.sent, n.sentAt, n.any = now, time.Now(), true
	return now, true
}

// line says how far a root has got: "read the groups: 3/16 done, 1
// failed", with the batch that ran last first, "batch 2/4", where it ran
// in batches.
func line(c progress.Count) string {
	parts := []string{fmt.Sprintf("%d/%d done", c.Done, c.Total)}
	if c.Batch != "" {
		parts = append([]string{"batch " + c.Batch}, parts...)
	}
	for _, k := range []struct {
		n    int
		what string
	}{{c.Failed, "failed"}, {c.Canceled, "canceled"}, {c.Skipped, "skipped"}} {
		if k.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", k.n, k.what))
		}
	}
	return c.Name + ": " + strings.Join(parts, ", ")
}
