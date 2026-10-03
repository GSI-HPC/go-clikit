// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext

import (
	"unicode"

	"golang.org/x/text/width"
)

// runeWidth returns how many columns a terminal gives r: two for a
// character whose East Asian Width is wide or fullwidth, as CJK, kana,
// Hangul and the emoji drawn as pictures are; none for NUL, for a mark that
// combines with the character before it, even one whose East Asian Width
// is wide, such as the ideographic tone marks U+302A to U+302D, and for a
// character that only formats, such as a zero-width joiner; and one for
// everything else. The soft hyphen U+00AD and the prepended concatenation
// marks, such as U+0600, format but are drawn, and take one column, as does
// the Ahom medial ra U+1171E, which combines in the Unicode tables of Go but
// is a spacing mark since Unicode 16.0.
//
// The East Asian Widths are those of golang.org/x/text, with the characters
// that later versions of Unicode, up to 18.0, made wide added. A character
// assigned after that takes one column, as does a combining mark newer than
// the Unicode tables of Go.
//
// It is not exported: added up over text, the widths of runes alone fall
// short of Width wherever U+FE0F follows a rune of one column.
func runeWidth(r rune) int {
	switch {
	case r == 0:
		return 0
	case r == 0x00ad, r == ahomMedialRa, unicode.Is(prependedConcatenationMarks, r):
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	case unicode.Is(wideSinceUnicode15, r):
		return 2
	}
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// ahomMedialRa is U+1171E, the one character the Unicode tables of Go call
// a combining mark, of no width, that Unicode 18.0 calls a spacing mark,
// which a terminal draws.
const ahomMedialRa = 0x1171e

// emojiPresentation is the variation selector U+FE0F, which asks for the
// emoji picture of the character before it.
const emojiPresentation = 0xfe0f

// advance returns the columns r adds to text whose last rune that took
// columns took last of them, and what last becomes. U+FE0F after a rune of
// one column makes that rune two columns, as a terminal draws its emoji
// picture; any other rune adds its runeWidth.
func advance(r rune, last int) (cols, next int) {
	if r == emojiPresentation {
		if last == 1 {
			return 1, 2
		}
		return 0, last
	}
	cols = runeWidth(r)
	if cols == 0 {
		return 0, last
	}
	return cols, cols
}

// Width returns how many columns s takes on a terminal: the widths of its
// runes added up, except that a rune of one column followed by U+FE0F is
// counted as two, since a terminal draws it as an emoji picture, as it does
// a keycap such as 1, U+FE0F, U+20E3. The width of one rune is
// Width(string(r)).
//
// s must be plain text that is already escaped, by Escape or EscapeLines: a
// control character or an escape sequence is counted as the columns of its
// runes, not as what it does to the terminal.
func Width(s string) int {
	n, last := 0, 0
	for _, r := range s {
		var w int
		w, last = advance(r, last)
		n += w
	}
	return n
}

// Truncate shortens s to at most cols columns, as Width counts them, so that
// a row never wraps. A wide character that would reach past cols is left out
// whole, never split, and a character whose U+FE0F would reach past cols is
// kept without it; cols of zero or less leaves nothing.
//
// s must be plain text that is already escaped, by Escape for one line:
// a control character or an escape sequence would be counted as the columns
// of its runes, not as what it does. Truncate does not cluster graphemes,
// so it may cut a sequence of runes a terminal draws as one character, such
// as an emoji joined with U+200D, between its runes.
func Truncate(s string, cols int) string {
	if cols < 1 {
		return ""
	}
	last := 0
	for i, r := range s {
		var w int
		w, last = advance(r, last)
		if w > cols {
			return s[:i]
		}
		cols -= w
	}
	return s
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

// wideSinceUnicode15 are the characters whose East Asian Width is wide in
// Unicode 18.0 but not in the Unicode 15.0 tables of golang.org/x/text:
// characters assigned since, such as the emoji U+1FAE9, and a few that
// became wide, such as the trigrams U+2630 to U+2637.
var wideSinceUnicode15 = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x2630, Hi: 0x2637, Stride: 1},
		{Lo: 0x268a, Hi: 0x268f, Stride: 1},
		{Lo: 0x2ffc, Hi: 0x2fff, Stride: 1},
		{Lo: 0x31e4, Hi: 0x31e5, Stride: 1},
		{Lo: 0x31ef, Hi: 0x31ef, Stride: 1},
		{Lo: 0x4dc0, Hi: 0x4dff, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x16ff2, Hi: 0x16ff6, Stride: 1},
		{Lo: 0x187f8, Hi: 0x187ff, Stride: 1},
		{Lo: 0x18cd6, Hi: 0x18cda, Stride: 1},
		{Lo: 0x18cff, Hi: 0x18cff, Stride: 1},
		{Lo: 0x18d09, Hi: 0x18d20, Stride: 1},
		{Lo: 0x18d80, Hi: 0x18df2, Stride: 1},
		{Lo: 0x18e00, Hi: 0x19191, Stride: 1},
		{Lo: 0x191a0, Hi: 0x191d2, Stride: 1},
		{Lo: 0x1b123, Hi: 0x1b128, Stride: 1},
		{Lo: 0x1b168, Hi: 0x1b168, Stride: 1},
		{Lo: 0x1d300, Hi: 0x1d356, Stride: 1},
		{Lo: 0x1d360, Hi: 0x1d376, Stride: 1},
		{Lo: 0x1f1ae, Hi: 0x1f1ae, Stride: 1},
		{Lo: 0x1f6d8, Hi: 0x1f6d9, Stride: 1},
		{Lo: 0x1f7da, Hi: 0x1f7da, Stride: 1},
		{Lo: 0x1fa89, Hi: 0x1fa8f, Stride: 1},
		{Lo: 0x1fabe, Hi: 0x1fabe, Stride: 1},
		{Lo: 0x1fac6, Hi: 0x1fac6, Stride: 1},
		{Lo: 0x1fac8, Hi: 0x1fac8, Stride: 1},
		{Lo: 0x1facc, Hi: 0x1facd, Stride: 1},
		{Lo: 0x1fadc, Hi: 0x1fadd, Stride: 1},
		{Lo: 0x1fadf, Hi: 0x1fadf, Stride: 1},
		{Lo: 0x1fae9, Hi: 0x1faeb, Stride: 1},
		{Lo: 0x1faef, Hi: 0x1faef, Stride: 1},
		{Lo: 0x1faf9, Hi: 0x1fafa, Stride: 1},
	},
}
