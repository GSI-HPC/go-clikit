// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext

import (
	"strings"
	"unicode"

	"golang.org/x/text/width"
)

// RuneWidth returns how many columns a terminal gives r: two for a
// character whose East Asian Width is wide or fullwidth, as CJK, kana,
// Hangul and the emoji drawn as pictures are; none for a mark that
// combines with the character before it or a character that only formats,
// such as a zero-width joiner; and one for everything else. The soft hyphen
// U+00AD and the prepended concatenation marks, such as U+0600, format but
// are drawn, and take one column.
func RuneWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r == 0x00ad, unicode.Is(prependedConcatenationMarks, r):
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// Width returns how many columns s takes on a terminal, the widths of its
// runes added up.
func Width(s string) int {
	n := 0
	for _, r := range s {
		n += RuneWidth(r)
	}
	return n
}

// Truncate shortens s to at most cols columns, as Width counts them, so that
// a row never wraps. A wide character that would reach past cols is left out
// whole, never split; cols of zero or less leaves nothing.
//
// s must be plain text that is already escaped, by EscapeCell for one line:
// a control character or an escape sequence would be counted as the columns
// of its runes, not as what it does. Truncate does not cluster graphemes,
// so it may cut a sequence of runes a terminal draws as one character, such
// as an emoji joined with U+200D, between its runes.
func Truncate(s string, cols int) string {
	if cols < 1 {
		return ""
	}
	if Width(s) <= cols {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		w := RuneWidth(r)
		if w > cols {
			break
		}
		b.WriteRune(r)
		cols -= w
	}
	return b.String()
}

// prependedConcatenationMarks are the format characters with the Unicode
// property Prepended_Concatenation_Mark, which a terminal draws, such as the
// Arabic number sign U+0600.
var prependedConcatenationMarks = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x0600, Hi: 0x0605, Stride: 1},
		{Lo: 0x06dd, Hi: 0x06dd, Stride: 1},
		{Lo: 0x070f, Hi: 0x070f, Stride: 1},
		{Lo: 0x0890, Hi: 0x0891, Stride: 1},
		{Lo: 0x08e2, Hi: 0x08e2, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x110bd, Hi: 0x110bd, Stride: 1},
		{Lo: 0x110cd, Hi: 0x110cd, Stride: 1},
	},
}
