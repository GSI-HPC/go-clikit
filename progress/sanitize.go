// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"strings"
	"unicode/utf8"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// Sanitize makes text from elsewhere, a remote line or an error, safe to
// show on one line of a terminal, in at most limit bytes; a limit of zero
// or less sets no bound.
//
// A carriage return is first applied the way a terminal shows it: it goes
// back to the start of the line and what follows overwrites what was
// there, so "50%\r100%" reads "100%", "abcdef\r12" reads "12cdef", and a
// line that ends in "\r\n" loses nothing. What is left is escaped by
// termtext.EscapeCell, the one escaper, which shows every other control
// character, an escape sequence's introducer, a bidirectional control, a
// newline and bytes that are not UTF-8 as a visible escape such as \x1b.
// The result is cut on a rune boundary.
func Sanitize(s string, limit int) string {
	if strings.IndexByte(s, '\r') >= 0 {
		s = foldCR(s, limit)
	}
	// Escaping never makes text shorter, so what lies past limit before it
	// lies past limit after it too, and need not be escaped.
	if limit > 0 && len(s) > limit {
		s = s[:runeCut(s, limit)]
	}
	s = termtext.EscapeCell(s)
	if limit > 0 && len(s) > limit {
		s = s[:runeCut(s, limit)]
	}
	return s
}

// foldCR applies every carriage return of s, line by line, as a cursor
// that returns to the first column. The newline that ends a line moves on
// to the next and overwrites nothing. Only what can land in the first
// limit bytes is kept.
func foldCR(s string, limit int) string {
	var b strings.Builder
	for line := range strings.SplitAfterSeq(s, "\n") {
		body, ended := strings.CutSuffix(line, "\n")
		b.WriteString(foldLine(body, limit))
		if ended {
			b.WriteByte('\n')
		}
		if limit > 0 && b.Len() >= limit {
			break
		}
	}
	return b.String()
}

// foldLine folds the carriage returns of one line. A cell is one rune, or
// one byte that is not UTF-8, which stays as it is for EscapeCell to show.
// Every cell takes at least a byte, so no cell past the first limit columns
// can reach the first limit bytes, and none is kept.
func foldLine(line string, limit int) string {
	if strings.IndexByte(line, '\r') < 0 {
		return line
	}
	var cells []string
	col := 0
	for i := 0; i < len(line); {
		_, size := utf8.DecodeRuneInString(line[i:])
		cell := line[i : i+size]
		i += size
		switch {
		case cell == "\r":
			col = 0
			continue
		case col < len(cells):
			cells[col] = cell
		case limit <= 0 || col < limit:
			cells = append(cells, cell)
		}
		col++
	}
	return strings.Join(cells, "")
}
