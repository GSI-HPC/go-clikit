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
//
// With Styles set, a Screen applies the sequences that set the attributes
// of text, as a display drawn in a theme writes them, and Styled shows
// which attributes each run of text was written in.
type Screen struct {
	// Width is the number of columns, at which a row wraps; 0 is a row
	// that never does.
	Width int
	// Styles, when set, applies the sequences that set the attributes of
	// text (SGR, ESC [ … m) as a terminal does: those of bold (1), faint
	// (2), underline (4) and reverse (7), their resets (22, 24, 27), the
	// colours 30 to 37, 90 to 97 and 38;5;n, the colour's reset (39), and
	// 0 or none, which sets every attribute back. They take no column, so
	// that a row wraps where a terminal's would, and String shows the text
	// alone. A sequence with any other parameter, such as italic, blink, a
	// background or a colour of 24 bits, is shown as text, as without
	// Styles, so that a test sees it.
	Styles bool

	mu   sync.Mutex
	rows [][]rune
	// styles are the attributes each cell of rows was written in, kept
	// with Styles; cur are those the next rune is written in.
	styles   [][]style
	cur      style
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
	case final == 'm' && s.Styles && sgrOK(param):
		s.cur = s.cur.apply(param)
	case final == 'A' && err == nil:
		s.row = max(0, s.row-max(n, 1))
		s.wrap = false
	case final == 'K' && param == "2":
		s.at()
		s.rows[s.row] = s.rows[s.row][:0]
		s.styles[s.row] = s.styles[s.row][:0]
		s.wrap = false
	case final == 'K' && n == 0 && err == nil:
		s.cut()
	case final == 'J' && n == 0 && err == nil:
		s.cut()
		s.rows = s.rows[:s.row+1]
		s.styles = s.styles[:s.row+1]
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
		s.styles[s.row] = s.styles[s.row][:col]
	}
	s.wrap = false
}

// at makes sure the row the cursor is on exists.
func (s *Screen) at() {
	for len(s.rows) <= s.row {
		s.rows = append(s.rows, nil)
		s.styles = append(s.styles, nil)
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
	row, styles := s.rows[s.row], s.styles[s.row]
	for len(row) < s.col+w {
		row = append(row, ' ')
		styles = append(styles, style{})
	}
	if row[s.col] == wideRest {
		row[s.col-1], styles[s.col-1] = ' ', style{}
	}
	if next := s.col + w; next < len(row) && row[next] == wideRest {
		row[next], styles[next] = ' ', style{}
	}
	row[s.col], styles[s.col] = r, s.cur
	if w == 2 {
		row[s.col+1], styles[s.col+1] = wideRest, s.cur
	}
	s.rows[s.row], s.styles[s.row] = row, styles
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
	rows, _ := s.view()
	var b strings.Builder
	for _, row := range rows {
		for _, r := range row {
			if r != wideRest {
				b.WriteRune(r)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Styled returns what String returns, with «p» before each run of text
// written in the attributes p, and «» where they return to none or a row
// ends in them: «1;31»✗«» exe0007. p names the attributes in one order,
// bold (1), faint (2), underline (4) and reverse (7), then the colour, as
// 31, 91 or 38;5;n, whatever order and however many sequences set them.
// Without Styles it returns what String returns.
//
// Styled shows the attributes of the text on the screen, not the sequences
// that set them: a row that ends in a colour reads the same whether the
// output set it back after the row or not. A colour left on shows only in
// the text written after it, which takes it; a test that checks that none
// is left on writes text after each row, or after the output, and looks at
// that.
func (s *Screen) Styled() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, styles := s.view()
	var b strings.Builder
	for i, row := range rows {
		var on style
		for j, r := range row {
			if r == wideRest {
				continue
			}
			if st := styles[i][j]; st != on {
				b.WriteString("«" + st.String() + "»")
				on = st
			}
			b.WriteRune(r)
		}
		if on != (style{}) {
			b.WriteString("«»")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// view returns the rows the screen shows and the attributes of their
// cells, up to the last row that is not blank, with what the output ends
// in the middle of shown as text. They are the screen's own, which s.mu,
// held, guards while they are read.
func (s *Screen) view() (rows [][]rune, styles [][]style) {
	rows, styles = s.rows, s.styles
	if len(s.pending) > 0 {
		v := &Screen{Width: s.Width, Styles: s.Styles, rows: make([][]rune, len(s.rows)), styles: make([][]style, len(s.styles)),
			cur: s.cur, row: s.row, col: s.col, wrap: s.wrap}
		for i, row := range s.rows {
			v.rows[i] = append([]rune(nil), row...)
			v.styles[i] = append([]style(nil), s.styles[i]...)
		}
		v.feed(s.pending, true)
		rows, styles = v.rows, v.styles
	}
	last := len(rows) - 1
	for last >= 0 && len(rows[last]) == 0 {
		last--
	}
	return rows[:last+1], styles[:last+1]
}

// style is the attributes a cell was written in: bold, faint, underline,
// reverse and a colour, none, one of SGR 30 to 37 and 90 to 97, or 256 and
// up for 38;5;n, n being colour-256.
type style struct {
	bold, faint, underline, reverse bool
	colour                          int
}

// String names the attributes as Styled writes them: "1;38;5;30".
func (st style) String() string {
	var parts []string
	for _, a := range []struct {
		on   bool
		code string
	}{{st.bold, "1"}, {st.faint, "2"}, {st.underline, "4"}, {st.reverse, "7"}} {
		if a.on {
			parts = append(parts, a.code)
		}
	}
	switch {
	case st.colour >= 256:
		parts = append(parts, "38;5;"+strconv.Itoa(st.colour-256))
	case st.colour > 0:
		parts = append(parts, strconv.Itoa(st.colour))
	}
	return strings.Join(parts, ";")
}

// sgrParams splits the parameters of a sequence that sets attributes,
// "1;38;5;30" or "38:5:208", into its codes, a colour of 256 colours as
// one "38;5;n" whichever way it is written.
func sgrParams(param string) []string {
	var codes []string
	parts := strings.Split(param, ";")
	for i := 0; i < len(parts); i++ {
		code := parts[i]
		if code == "38" && i+2 < len(parts) && parts[i+1] == "5" {
			code = "38;5;" + parts[i+2]
			i += 2
		}
		codes = append(codes, strings.ReplaceAll(code, ":", ";"))
	}
	return codes
}

// sgrOK reports whether a Screen with Styles applies the sequence of
// attributes param is the parameters of: whether it knows every code.
func sgrOK(param string) bool {
	for _, code := range sgrParams(param) {
		if _, ok := codeOf(code); !ok {
			return false
		}
	}
	return true
}

// codeOf returns what code does to the attributes, and false for a code a
// Screen does not know.
func codeOf(code string) (func(*style), bool) {
	if n, ok := strings.CutPrefix(code, "38;5;"); ok {
		i, err := strconv.Atoi(n)
		if err != nil || i < 0 || i > 255 {
			return nil, false
		}
		return func(st *style) { st.colour = 256 + i }, true
	}
	n, err := strconv.Atoi(code)
	if code == "" {
		n, err = 0, nil
	}
	switch {
	case err != nil:
		return nil, false
	case n == 0:
		return func(st *style) { *st = style{} }, true
	case n == 1:
		return func(st *style) { st.bold = true }, true
	case n == 2:
		return func(st *style) { st.faint = true }, true
	case n == 4:
		return func(st *style) { st.underline = true }, true
	case n == 7:
		return func(st *style) { st.reverse = true }, true
	case n == 22:
		return func(st *style) { st.bold, st.faint = false, false }, true
	case n == 24:
		return func(st *style) { st.underline = false }, true
	case n == 27:
		return func(st *style) { st.reverse = false }, true
	case n == 39:
		return func(st *style) { st.colour = 0 }, true
	case n >= 30 && n <= 37, n >= 90 && n <= 97:
		return func(st *style) { st.colour = n }, true
	}
	return nil, false
}

// apply returns st with the attributes param sets, which sgrOK has found
// to be known.
func (st style) apply(param string) style {
	for _, code := range sgrParams(param) {
		set, _ := codeOf(code)
		set(&st)
	}
	return st
}
