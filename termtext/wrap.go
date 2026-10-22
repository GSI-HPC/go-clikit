// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext

import "strings"

// Wrap fills s into lines of at most width columns, as Width counts them,
// and returns them, each ended by a newline, the last one too. The first
// line starts with first and every later line with rest, such as an indent,
// and both count towards width.
//
// s is split into words as strings.Fields splits it, so its newlines and
// runs of spaces are lost: Wrap fills one paragraph. A word is added to the
// line while the line, a space and the word fit in width columns, and
// starts the next line otherwise. A line holds at least one word, and a
// word is never cut: one wider than width stays whole on a line of its own,
// which is then wider than width. s without words gives "". A width of zero
// or less never ends a line, so every word goes on the first line.
//
// Wrap does not escape: s is text of the program's own, such as the help of
// a command. Untrusted text has to be escaped by Escape first, since Width
// counts what a control character does as the columns of its runes.
func Wrap(s string, width int, first, rest string) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + len(first) + 1)
	firstCols, firstLast := measure(0, 0, first)
	restCols, restLast := measure(0, 0, rest)

	b.WriteString(first)
	cols, last := firstCols, firstLast
	onLine := false
	for _, w := range words {
		if onLine {
			// A space takes one column and ends in a rune of one column.
			c, l := measure(cols+1, 1, w)
			if width <= 0 || c <= width {
				b.WriteByte(' ')
				b.WriteString(w)
				cols, last = c, l
				continue
			}
			b.WriteByte('\n')
			b.WriteString(rest)
			cols, last = restCols, restLast
		}
		b.WriteString(w)
		cols, last = measure(cols, last, w)
		onLine = true
	}
	b.WriteByte('\n')
	return b.String()
}

// measure returns the columns of text that took cols columns and whose last
// rune that took columns took last of them, once s is added to it, and what
// last becomes, so that the columns of a line are those Width gives it.
func measure(cols, last int, s string) (int, int) {
	for _, r := range s {
		var w int
		w, last = advance(r, last)
		cols += w
	}
	return cols, last
}
