// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package sse streams the live parts of a local web page as Server-Sent
// Events: each event is named after a fragment of the page and carries its
// HTML, which htmx's SSE extension swaps into the element of that name.
//
// What the page shows changes in topics, which a Notifier tells its
// subscribers. Serve subscribes, renders every fragment and sends it, the
// baseline; then it gathers the topics that change for a moment, renders
// what depends on them, and sends each fragment whose HTML differs from
// what it sent last. The baseline comes first because the page may have
// been rendered before a change the stream did not hear of: the time
// between the page and its stream, or the time a broken stream took to
// reconnect. So a stream needs no event IDs, and a reconnect loses nothing.
//
// A program's store notifies the topics it changes:
//
//	func (s *Store) SetRuns(runs []Run) {
//		s.mu.Lock()
//		s.runs = runs
//		s.mu.Unlock()
//		s.notifier.Notify("runs")
//	}
//
// and its handler of the stream renders the fragments that the page named
// and that depend on the topics that changed, or all of them when changed
// is nil:
//
//	err := sse.Serve(w, r, &store.notifier, func(changed []string) ([]sse.Event, error) {
//		return renderFragments(names, changed)
//	}, sse.Options{})
//
// The stream lifts the write deadline of its connection, which
// localweb.Serve sets for every other answer, and ends when that server
// starts to stop (localweb.Stopping).
package sse

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GSI-HPC/go-clikit/localweb"
)

// The timings Serve uses when Options leaves them zero.
const (
	// DefaultCoalesce is how long changes gather before a stream renders
	// them, so that a burst of changes costs one render: about two frames,
	// so that a page moves as soon as a terminal would.
	DefaultCoalesce = 30 * time.Millisecond
	// DefaultKeepAlive is how often an idle stream sends a comment, so
	// that nothing between drops it and a gone browser is noticed.
	DefaultKeepAlive = 25 * time.Second
	// DefaultRetry is how long a browser waits before it opens a broken
	// stream again.
	DefaultRetry = 2 * time.Second
)

// Event is one event of a stream: a fragment of a page.
type Event struct {
	// Name is the event's name, the fragment's; it holds no line break.
	Name string
	// Data is the fragment's HTML.
	Data string
}

// Options configures Serve. A zero duration is its default, and a
// negative one turns the thing off, as in localweb.ServeOptions; no value
// makes Serve panic (doc/decisions.md, decision 40).
type Options struct {
	// Coalesce is how long changes gather before they are rendered; 0 is
	// DefaultCoalesce, and a negative Coalesce gathers nothing: each
	// change is rendered at once.
	Coalesce time.Duration
	// KeepAlive is how often an idle stream sends a comment; 0 is
	// DefaultKeepAlive, and a negative KeepAlive sends none.
	KeepAlive time.Duration
	// Retry is how long a browser waits before it opens a broken stream
	// again; 0 is DefaultRetry, and a negative Retry sends no retry
	// field, so that the browser waits as long as it chooses.
	Retry time.Duration
	// Done ends the stream when it closes, as the context of a server
	// that stops does: http.Server.Shutdown does not end the requests in
	// flight, and would wait for a stream until its grace period ends.
	// nil is the stop signal of localweb.Serve, localweb.Stopping of the
	// request's context, which never closes for a request that another
	// server serves.
	Done <-chan struct{}
}

// Serve streams the events that render returns to the request r, until
// the client goes, or o.Done closes, or, when o.Done is nil, the
// localweb.Serve that serves r starts to stop. render is called with nil first,
// for every event, and then with the topics of n that changed since its
// last call, in the order of their first change, for the events that
// depend on them; an event whose data is what the stream sent last under
// its name is not sent again.
//
// Serve returns an error, and writes nothing, when the first render
// fails. A later render that fails ends the stream with its error, after
// which the browser opens it again and gets every event anew. A client
// that goes is no error. A program puts localweb.SameOrigin, without
// Navigate, in front of the stream: no page of another site may open it.
func Serve[T comparable](w http.ResponseWriter, r *http.Request, n *Notifier[T],
	render func(changed []T) ([]Event, error), o Options,
) error {
	o = o.withDefaults()
	done := o.Done
	if done == nil {
		done = localweb.Stopping(r.Context())
	}
	sub := n.Subscribe()
	defer sub.Close()
	first, err := render(nil)
	if err == nil {
		err = checkNames(first)
	}
	if err != nil {
		return fmt.Errorf("sse: render the first events: %w", err)
	}

	rc := http.NewResponseController(w)
	// A server without a write deadline, or a ResponseWriter that cannot
	// move it, has nothing to lift.
	_ = rc.SetWriteDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	// A proxy such as nginx would hold the events back.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	st := stream{w: w, rc: rc, sent: map[string][sha256.Size]byte{}}
	if o.Retry > 0 {
		_, _ = io.WriteString(w, "retry: "+strconv.FormatInt(o.Retry.Milliseconds(), 10)+"\n\n")
	}
	if err := st.send(first); err != nil {
		return st.end(err)
	}

	var timer, keepAlive <-chan time.Time
	if o.KeepAlive > 0 {
		t := time.NewTicker(o.KeepAlive)
		defer t.Stop()
		keepAlive = t.C
	}
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-done:
			return nil
		case <-sub.Changed():
			if timer == nil {
				timer = time.After(max(o.Coalesce, 0))
			}
		case <-keepAlive:
			_, _ = io.WriteString(w, ": keep-alive\n\n")
			if err := rc.Flush(); err != nil {
				return st.end(err)
			}
		case <-timer:
			timer = nil
			changed := sub.Take()
			if len(changed) == 0 {
				continue
			}
			events, err := render(changed)
			if err == nil {
				err = checkNames(events)
			}
			if err != nil {
				return fmt.Errorf("sse: render the events of %v: %w", changed, err)
			}
			if err := st.send(events); err != nil {
				return st.end(err)
			}
		}
	}
}

// withDefaults returns o with each zero field set to its default.
func (o Options) withDefaults() Options {
	if o.Coalesce == 0 {
		o.Coalesce = DefaultCoalesce
	}
	if o.KeepAlive == 0 {
		o.KeepAlive = DefaultKeepAlive
	}
	if o.Retry == 0 {
		o.Retry = DefaultRetry
	}
	return o
}

// stream is what Serve writes to.
type stream struct {
	w  io.Writer
	rc *http.ResponseController
	// sent holds the hash of the data last sent under each name.
	sent map[string][sha256.Size]byte
}

// checkNames refuses an event whose name holds a line break, which would
// end the name's line of the stream.
func checkNames(events []Event) error {
	for _, e := range events {
		if strings.ContainsAny(e.Name, "\r\n") {
			return fmt.Errorf("the event name %q holds a line break", e.Name)
		}
	}
	return nil
}

// send writes each event whose data differs from what was sent last under
// its name, and flushes.
func (st *stream) send(events []Event) error {
	for _, e := range events {
		sum := sha256.Sum256([]byte(e.Data))
		if old, ok := st.sent[e.Name]; ok && old == sum {
			continue
		}
		st.sent[e.Name] = sum
		// A write that fails shows again in the flush.
		_, _ = io.WriteString(st.w, Format(e))
	}
	return st.rc.Flush()
}

// end returns the error that ended the stream: none when the client went,
// which shows as a failed write or flush, and the error of a
// ResponseWriter that cannot flush, which cannot stream.
func (st *stream) end(err error) error {
	if errors.Is(err, http.ErrNotSupported) {
		return err
	}
	return nil
}

// Format returns e as the lines of one Server-Sent Event: its name, a data
// line for each line of its data, and an empty line. A carriage return
// also ends a line of the stream, so it is written as the character
// reference &#13;, which HTML reads the same.
func Format(e Event) string {
	var b strings.Builder
	b.WriteString("event: ")
	b.WriteString(e.Name)
	b.WriteByte('\n')
	for line := range strings.SplitSeq(strings.ReplaceAll(e.Data, "\r", "&#13;"), "\n") {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}
