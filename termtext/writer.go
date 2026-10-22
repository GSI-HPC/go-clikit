// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext

import (
	"bytes"
	"io"
	"sync"
	"unicode/utf8"
)

// maxHeld is how many bytes of a line that has not ended an escape writer
// holds back before it writes them out.
const maxHeld = 64 << 10

// NewEscapeWriter returns a writer that passes what it is given on to w,
// escaped, as a stream: the output of a child program or of a command on a
// remote host, say, which is shown on a terminal as it arrives. Each line is
// escaped with EscapeLines, so newline and tab are kept and every other
// control character is shown as an escape, a carriage return too.
//
// The writer holds a line back until its newline comes, or until Close, so
// that a rune that arrives in two writes is not cut in two and escaped as
// bytes that are not UTF-8. It holds back no more than 64 KiB: a longer
// line is written out, escaped, up to its last whole rune, without a
// newline, and goes on with the next write; the bytes of a rune that is not
// whole yet, three at most, are held back. The text written to w is the
// same whatever the writes were cut as.
//
// Write returns len(p) and the first error of w it met in that call: a
// write that fails has still consumed p, and the lines after the one that
// failed are written to w as well. The writer is safe for concurrent use,
// and calls w one at a time.
//
// Close writes the line that has not ended, escaped, followed by a newline,
// and does not close w. A second Close does nothing. Write after Close
// writes nothing and returns io.ErrClosedPipe.
func NewEscapeWriter(w io.Writer) io.WriteCloser {
	return &escapeWriter{w: w}
}

type escapeWriter struct {
	mu     sync.Mutex
	w      io.Writer
	part   []byte // the bytes of the line that has not ended, held back
	cut    bool   // part is the rest of a line already partly written
	closed bool
}

func (e *escapeWriter) Write(p []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return 0, io.ErrClosedPipe
	}
	n := len(p)
	var first error
	keep := func(err error) {
		if first == nil {
			first = err
		}
	}
	for len(p) > 0 {
		// Take no more than fills part to one byte over maxHeld, so that
		// what is held back stays bounded whatever p is.
		chunk := p[:min(len(p), maxHeld+1-len(e.part))]
		if i := bytes.IndexByte(chunk, '\n'); i >= 0 {
			e.part = append(e.part, chunk[:i]...)
			keep(e.write(string(e.part), "\n"))
			e.part, e.cut = e.part[:0], false
			p = p[i+1:]
			continue
		}
		e.part = append(e.part, chunk...)
		p = p[len(chunk):]
		if len(e.part) > maxHeld {
			whole := wholeRunes(e.part)
			keep(e.write(string(e.part[:whole]), ""))
			e.part, e.cut = e.part[:copy(e.part, e.part[whole:])], true
		}
	}
	return n, first
}

func (e *escapeWriter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	if len(e.part) == 0 && !e.cut {
		return nil
	}
	err := e.write(string(e.part), "\n")
	e.part = nil
	return err
}

// write escapes text and writes it, with end, to w in one call.
func (e *escapeWriter) write(text, end string) error {
	_, err := io.WriteString(e.w, EscapeLines(text)+end)
	return err
}

// wholeRunes returns how many bytes of b, which is longer than a rune, end
// at a rune boundary: all of them, unless b ends in the start of a rune that
// is not whole yet, which is left out. A byte that cannot start a rune is
// not waited for, since more bytes would not make it one.
func wholeRunes(b []byte) int {
	for i := len(b) - 1; i >= len(b)-utf8.UTFMax+1; i-- {
		if utf8.RuneStart(b[i]) {
			if utf8.FullRune(b[i:]) {
				return len(b)
			}
			return i
		}
	}
	return len(b)
}
