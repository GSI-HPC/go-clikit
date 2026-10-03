// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progresstest

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// Screen is a terminal for a test to draw on and read back: it shows what
// is written to it the way a terminal would, as far as a display writes
// it. It knows text, a newline, which starts the next row at its first
// column as a terminal's output processing has it do, a carriage return,
// and the sequences ESC [ n A, which moves the cursor up n rows, ESC [ 2K,
// which erases the row the cursor is on, ESC [ K, which erases the rest of
// it, and ESC [ J, which erases from the cursor to the end of the screen.
// Any other control character or sequence is shown as text, "^[[?25l" or
// "^G", so that a test sees it. So is an escape or a sequence that a byte
// it cannot hold, such as a newline or another escape, cuts short, up to
// that byte, which then takes effect, and an escape, a sequence or a rune
// that the output ends in the middle of.
//
// Every row is kept, those that have scrolled off the top too, so that what
// a Screen shows is the scrollback and the screen in one. With Width set, a
// row wraps at that column, as a terminal's does, so that a row drawn too
// long shows as the two it is. As on a terminal, the wrap waits for the next
// rune: a row written to its last column leaves the cursor on that column,
// where ESC [ K erases the last rune and a carriage return or a newline
// wraps nothing. A wide rune, as CJK text and emoji are, takes two columns;
// writing over either half of one blanks the other. A Screen is safe for
// concurrent use.
type Screen struct {
	// Width is the number of columns, at which a row wraps; 0 is a row
	// that never does.
	Width int

	mu       sync.Mutex
	rows     [][]rune
	row, col int
	// wrap is set when a rune has filled the last column, and the next one
	// starts the next row.
	wrap bool
	// pending is the start of a sequence or of a rune that the next
	// write completes.
	pending []byte
}

// Write shows p on the screen.
func (s *Screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = s.feed(append(s.pending, p...), false)
	return len(p), nil
}

// feed applies b, and returns what is left of it: the start of a sequence
// or of a rune that b ends in the middle of. With end set, nothing more
// comes, and that start is shown as it is.
func (s *Screen) feed(b []byte, end bool) []byte {
	for len(b) > 0 {
		switch c := b[0]; {
		case c == 0x1b:
			n, ok := s.escape(b)
			if !ok {
				if !end {
					return append([]byte(nil), b...)
				}
				s.text("^[" + string(b[1:]))
				return nil
			}
			b = b[n:]
		case c == '\n':
			s.row++
			s.col = 0
			s.wrap = false
			s.at()
			b = b[1:]
		case c == '\r':
			s.col = 0
			s.wrap = false
			b = b[1:]
		case c < 0x20 || c == 0x7f:
			s.text(fmt.Sprintf("^%c", c^0x40))
			b = b[1:]
		default:
			if !end && !utf8.FullRune(b) {
				return append([]byte(nil), b...)
			}
			r, n := utf8.DecodeRune(b)
			s.put(r)
			b = b[n:]
		}
	}
	return nil
}

// escape applies the sequence b starts with, and returns its length; false
// when b ends before the sequence does. A sequence that a byte it cannot
// hold cuts short is shown up to that byte, which is left to take effect.
func (s *Screen) escape(b []byte) (int, bool) {
	if len(b) < 2 {
		return 0, false
	}
	if b[1] != '[' {
		if b[1] < 0x20 || b[1] > 0x7e {
			s.text("^[")
			return 1, true
		}
		s.text("^[" + string(b[1]))
		return 2, true
	}
	end := 2
	for end < len(b) && b[end] >= 0x20 && b[end] <= 0x3f {
		end++
	}
	if end == len(b) {
		return 0, false
	}
	if b[end] > 0x7e || b[end] < 0x40 {
		s.text("^[" + string(b[1:end]))
		return end, true
	}
	param, final := string(b[2:end]), b[end]
	n, err := strconv.Atoi(param)
	if param == "" {
		n, err = 0, nil
	}
	switch {
	case final == 'A' && err == nil:
		s.row = max(0, s.row-max(n, 1))
		s.wrap = false
	case final == 'K' && param == "2":
		s.at()
		s.rows[s.row] = s.rows[s.row][:0]
		s.wrap = false
	case final == 'K' && n == 0 && err == nil:
		s.cut()
	case final == 'J' && n == 0 && err == nil:
		s.cut()
		s.rows = s.rows[:s.row+1]
	default:
		s.text("^[" + string(b[1:end+1]))
	}
	return end + 1, true
}

// cut erases the row the cursor is on from the cursor to its end, and a
// wide rune whose right half the cursor is on with it.
func (s *Screen) cut() {
	s.at()
	row := s.rows[s.row]
	if col := s.col; col < len(row) {
		if row[col] == wideRest {
			col--
		}
		s.rows[s.row] = row[:col]
	}
	s.wrap = false
}

// at makes sure the row the cursor is on exists.
func (s *Screen) at() {
	for len(s.rows) <= s.row {
		s.rows = append(s.rows, nil)
	}
}

func (s *Screen) text(t string) {
	for _, r := range t {
		s.put(r)
	}
}

// wideRest fills the column a wide rune takes after its own, which String
// leaves out.
const wideRest = rune(0)

// put writes r where the cursor is, over what was there. A wide rune, as
// CJK text and emoji are, takes two columns, and wraps when only one is
// left, as it does on a terminal; writing over half of one blanks the other
// half. A rune that fills the last column leaves the cursor on it, and the
// next rune wraps.
func (s *Screen) put(r rune) {
	w := max(1, termtext.Width(string(r)))
	if s.Width > 0 && (s.wrap || s.col+w > s.Width) {
		s.row++
		s.col = 0
	}
	s.wrap = false
	s.at()
	row := s.rows[s.row]
	for len(row) < s.col+w {
		row = append(row, ' ')
	}
	if row[s.col] == wideRest {
		row[s.col-1] = ' '
	}
	if next := s.col + w; next < len(row) && row[next] == wideRest {
		row[next] = ' '
	}
	row[s.col] = r
	if w == 2 {
		row[s.col+1] = wideRest
	}
	s.rows[s.row] = row
	s.col += w
	if s.Width > 0 && s.col >= s.Width {
		s.col = s.Width - 1
		s.wrap = true
	}
}

// String returns what the screen shows, each row ended by a newline, up to
// the last row that is not blank. An escape, a sequence or a rune that the
// output so far ends in the middle of is shown as text, as if nothing more
// came; the next Write still completes it.
func (s *Screen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.rows
	if len(s.pending) > 0 {
		v := &Screen{Width: s.Width, rows: make([][]rune, len(s.rows)), row: s.row, col: s.col, wrap: s.wrap}
		for i, row := range s.rows {
			v.rows[i] = append([]rune(nil), row...)
		}
		v.feed(s.pending, true)
		rows = v.rows
	}
	last := len(rows) - 1
	for last >= 0 && len(rows[last]) == 0 {
		last--
	}
	var b strings.Builder
	for _, row := range rows[:last+1] {
		for _, r := range row {
			if r != wideRest {
				b.WriteRune(r)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
