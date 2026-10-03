// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"bytes"
	"context"
	"io"
	"slices"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// lineBurst lines of a call are sent at once, and then one each
	// lineEvery, so that a command that prints fast does not flood the
	// sinks: at a fan-out of 16 that is at most 160 lines a second.
	lineBurst = 20
	lineEvery = 100 * time.Millisecond
	// cutLine is how much of a line without an end a display is sent at a
	// time.
	cutLine = 4 << 10
	// maxParsedLine is the longest line handed to a parser. A longer one
	// is no answer any parser here reads, and holding it would take as
	// much memory as the output.
	maxParsedLine = 64 << 10
)

// lineState is the rate limit of a span's lines, guarded by bus.mu.
type lineState struct {
	tokens int
	last   time.Time
	// pending is the newest line that found no token, sent with the next
	// token or at the End, so that the last line a display shows is never
	// an old one; late sends it once the next token is due when no line
	// comes before.
	pending    string
	hasPending bool
	pendingOn  Stream
	late       *time.Timer
	// since counts the lines left out since the last one sent, and
	// dropped all of them.
	since, dropped int
	// tails are the tails that hold a line that has not ended, to be sent
	// before the End; a tail is listed only while it holds one, so that a
	// span that runs many commands keeps nothing of those that are done.
	tails []*tail
}

// Tee returns a writer that writes to w, and cuts what it writes into
// lines on the way.
//
// Each complete line goes to parse, when it is not nil, with its "\n" and a
// "\r" before it taken off, from the goroutine that writes; the tees of a
// command's two streams may call it at the same time. A parser sees only
// lines that ended: a line cut off by the end of the output, or longer than
// 64 KiB, is none it is given.
//
// When the innermost span in ctx has ShowLines and a sink wants lines, they
// also go to the sinks as TypeLine events, sanitised and rate limited: a
// burst of 20, then one each 100ms. A line that finds no place waits for
// the next one, in place of the line that waited before it, which is
// counted as dropped; so the newest line is never lost, and is sent when
// its place comes, whether another line comes or not. A line that does
// not end is sent in pieces of 4 KiB, and what is left of it when the span
// ends is sent before the End. The End sends the line that waits, and what
// is left of the lines that did not end, though it finds no place for
// them: of those, the newest of each stream, so that the last line of
// stdout and that of stderr both reach the sinks. Until the End, the span
// keeps the line of a writer that has not ended, but neither the writer
// nor anything of one whose output ended with a line.
//
// Tee returns w itself when nothing needs the lines. The lines never fail a
// write: what Write returns is what w returned.
func Tee(ctx context.Context, w io.Writer, st Stream, parse func(Stream, string)) io.Writer {
	s := SpanFrom(ctx)
	if s != nil && (s.flags&ShowLines == 0 || !s.wantsLines()) {
		s = nil
	}
	if s == nil && parse == nil {
		return w
	}
	t := &tee{w: w, stream: st, parse: parse}
	if s != nil {
		t.tail = &tail{stream: st, span: s}
	}
	return t
}

// wantsLines reports whether a sink wants the lines of s, which has not
// ended.
func (s *Span) wantsLines() bool {
	b := s.bus
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.closed && b.lines && s.state != StateEnded
}

func (s *Span) lineState() *lineState {
	if s.lines == nil {
		s.lines = &lineState{tokens: lineBurst, last: s.bus.now()}
	}
	return s.lines
}

type tee struct {
	w      io.Writer
	stream Stream
	parse  func(Stream, string)
	// tail is nil when no display wants the lines.
	tail *tail

	mu sync.Mutex
	// parsed is the line so far for the parser, and long is set when it
	// is too long to be one.
	parsed []byte
	long   bool
}

// tail is the part of a tee a display needs, apart from the tee so that
// the span that lists it keeps neither the writer nor the parser's line.
type tail struct {
	stream Stream

	mu sync.Mutex
	// span is nil once it has ended.
	span *Span
	// shown is the part of the line so far not yet sent to the display.
	shown []byte

	// listed is set while the span lists the tail; b.mu guards it.
	listed bool
}

func (t *tee) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	if n > 0 && n <= len(p) {
		t.frame(p[:n])
	}
	return n, err
}

// frame cuts p into lines, and hands them on once t.mu and the tail's lock
// are let go of: a span's End takes the tail's lock while it holds the Bus
// lock, so the locks are never taken the other way round.
func (t *tee) frame(p []byte) {
	var parsed, shown []string
	var span *Span
	if t.parse != nil {
		parsed = t.lines(p)
	}
	if t.tail != nil {
		span, shown = t.tail.cut(p)
	}
	for _, line := range parsed {
		t.parse(t.stream, line)
	}
	if span != nil {
		span.show(t.tail, shown)
	}
}

// lines returns the lines of p that ended, for the parser.
func (t *tee) lines(p []byte) []string {
	var lines []string
	t.mu.Lock()
	defer t.mu.Unlock()
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		part := p
		if i >= 0 {
			part = p[:i]
		}
		if !t.long {
			if len(t.parsed)+len(part) > maxParsedLine {
				t.long, t.parsed = true, t.parsed[:0]
			} else {
				t.parsed = append(t.parsed, part...)
			}
		}
		if i < 0 {
			break
		}
		if !t.long {
			lines = append(lines, string(bytes.TrimSuffix(t.parsed, []byte("\r"))))
		}
		t.parsed, t.long = t.parsed[:0], false
		p = p[i+1:]
	}
	return lines
}

// cut returns the lines and pieces of p for the display, and the span to
// send them to, which is nil once it has ended.
func (tl *tail) cut(p []byte) (*Span, []string) {
	var lines []string
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if tl.span == nil {
		return nil, nil
	}
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		part := p
		if i >= 0 {
			part = p[:i]
		}
		lines = tl.pieces(lines, part)
		if i < 0 {
			break
		}
		if len(tl.shown) > 0 {
			lines = append(lines, string(tl.shown))
			tl.shown = tl.shown[:0]
		}
		p = p[i+1:]
	}
	return tl.span, lines
}

// pieces adds part to the line so far, and appends to lines the pieces of
// 4 KiB it is cut into. The line so far is filled to one byte past a piece,
// which is all runeCut looks at, and never holds more: so a long write is
// copied once, not once for each piece cut from it. tl.mu is held.
func (tl *tail) pieces(lines []string, part []byte) []string {
	for {
		n := min(cutLine+1-len(tl.shown), len(part))
		tl.shown = append(tl.shown, part[:n]...)
		part = part[n:]
		if len(tl.shown) < cutLine {
			return lines
		}
		k := runeCut(tl.shown, cutLine)
		lines = append(lines, string(tl.shown[:k]))
		tl.shown = append(tl.shown[:0], tl.shown[k:]...)
	}
}

// runeCut returns where to cut p to at most n bytes without splitting a
// rune.
func runeCut[T string | []byte](p T, n int) int {
	if n >= len(p) {
		return len(p)
	}
	for i := n; i > 0 && i > n-utf8.UTFMax; i-- {
		if utf8.RuneStart(p[i]) {
			return i
		}
	}
	// Continuation bytes that follow no start are no rune to split.
	return n
}

// show offers lines of tl to the sinks, then the line held back if a token
// has come free since, and lists tl while it holds a line that has not
// ended.
func (s *Span) show(tl *tail, lines []string) {
	b := s.bus
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || s.state == StateEnded {
		tl.mu.Lock()
		tl.span, tl.shown = nil, nil
		tl.mu.Unlock()
		return
	}
	ls := s.lineState()
	ls.refill(b.now())
	for _, line := range lines {
		s.offer(tl.stream, line)
	}
	if ls.hasPending && ls.tokens > 0 {
		ls.tokens--
		s.sendPending()
	}
	tl.mu.Lock()
	unended := len(tl.shown) > 0
	tl.mu.Unlock()
	if unended != tl.listed {
		if unended {
			ls.tails = append(ls.tails, tl)
		} else {
			ls.tails = slices.DeleteFunc(ls.tails, func(o *tail) bool { return o == tl })
		}
		tl.listed = unended
	}
}

// offer sends line if a token is free, and holds it back otherwise, in
// place of any line held back before it. b.mu is held.
func (s *Span) offer(st Stream, line string) {
	ls := s.lines
	if ls.hasPending {
		// The newest line wins; the one before it is not sent.
		ls.since++
		ls.dropped++
		ls.hasPending = false
	}
	if ls.tokens > 0 {
		ls.tokens--
		s.sendLine(st, line)
		return
	}
	ls.pending, ls.pendingOn, ls.hasPending = line, st, true
	s.sendLate()
}

// sendLate has the line held back sent once the next token is due, unless
// another line or the End sends it first. b.mu is held.
func (s *Span) sendLate() {
	ls := s.lines
	if ls.late != nil {
		return
	}
	wait := lineEvery - s.bus.now().Sub(ls.last)
	if wait <= 0 {
		wait = lineEvery
	}
	ls.late = time.AfterFunc(wait, func() {
		b := s.bus
		b.mu.Lock()
		defer b.mu.Unlock()
		ls.late = nil
		if b.closed || s.state == StateEnded || !ls.hasPending {
			return
		}
		ls.refill(b.now())
		if ls.tokens > 0 {
			ls.tokens--
			s.sendPending()
			return
		}
		s.sendLate()
	})
}

func (s *Span) sendPending() {
	ls := s.lines
	ls.hasPending = false
	s.sendLine(ls.pendingOn, ls.pending)
	ls.pending = ""
}

// sendLine sends one line. b.mu is held.
func (s *Span) sendLine(st Stream, line string) {
	e := s.event(TypeLine)
	e.Stream, e.Text, e.Dropped = st, Sanitize(line, MaxText), s.lines.since
	s.lines.since = 0
	s.bus.emit(e)
}

// refill adds the tokens that have come due since the last one was
// spent.
func (ls *lineState) refill(now time.Time) {
	if ls.tokens >= lineBurst {
		ls.last = now
		return
	}
	n := int(now.Sub(ls.last) / lineEvery)
	if n <= 0 {
		return
	}
	ls.tokens = min(lineBurst, ls.tokens+n)
	ls.last = ls.last.Add(time.Duration(n) * lineEvery)
}

// heldLine is a line that found no token when its span ended.
type heldLine struct {
	stream Stream
	text   string
}

// flushLines sends what the tails of s hold of an unfinished line and the
// line held back, before s ends, and returns how many lines were dropped
// in all. b.mu is held.
//
// The lines that find no token are sent all the same, the newest of each
// stream: the line held back gives way to an unfinished line of its own
// stream, but not to one of the other, so that the last line of each is
// what a display shows.
func (s *Span) flushLines() int {
	ls := s.lines
	if ls == nil {
		return 0
	}
	ls.refill(s.bus.now())
	if ls.hasPending && ls.tokens > 0 {
		ls.tokens--
		s.sendPending()
	}
	var held []heldLine
	if ls.hasPending {
		held = append(held, heldLine{ls.pendingOn, ls.pending})
		ls.hasPending, ls.pending = false, ""
	}
	for _, tl := range ls.tails {
		tl.mu.Lock()
		rest := string(tl.shown)
		tl.span, tl.shown, tl.listed = nil, nil, false
		tl.mu.Unlock()
		switch {
		case rest == "":
			// A write ended the line, and has yet to show it.
		case ls.tokens > 0:
			ls.tokens--
			s.sendLine(tl.stream, rest)
		default:
			held = s.hold(held, heldLine{tl.stream, rest})
		}
	}
	ls.tails = nil
	for _, h := range held {
		s.sendLine(h.stream, h.text)
	}
	if ls.late != nil {
		ls.late.Stop()
		ls.late = nil
	}
	return ls.dropped
}

// hold adds h to the lines held at the End, in place of the one of its
// stream held before it, which is counted as dropped. b.mu is held.
func (s *Span) hold(held []heldLine, h heldLine) []heldLine {
	if i := slices.IndexFunc(held, func(o heldLine) bool { return o.stream == h.stream }); i >= 0 {
		held = slices.Delete(held, i, i+1)
		s.lines.since++
		s.lines.dropped++
	}
	return append(held, h)
}
