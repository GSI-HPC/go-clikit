// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// A terminal gives CJK, Hangul, fullwidth forms and emoji two columns, a
// combining mark and a zero-width joiner none, and the rest one.
func TestTheWidthOfTextOnATerminal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text string
		want int
	}{
		{"exe0001", 7},
		{"失败", 4},
		{"✅ done", 7},
		{"한국어", 6},
		{"ＡＢ", 4},
		{"é", 1},
		{"a\u200db", 2},
		{"✓ ▸ … ·", 7},
		{"😀", 2},
	} {
		if got := termtext.Width(tc.text); got != tc.want {
			t.Errorf("Width(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

// A variation selector U+FE0F asks for the emoji picture of the character
// before it, which a terminal draws two columns wide, as it does a keycap.
func TestAnEmojiPresentationSequenceTakesTwoColumns(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text string
		want int
	}{
		{"\u2764\ufe0f", 2},
		{"\u263a\ufe0f", 2},
		{"1\ufe0f\u20e3", 2},
		{"\u2764\ufe0f\u2764\ufe0f", 4},
		{"\u2764\ufe0f\ufe0f", 2},
		{"\U0001F600\ufe0f", 2},
		{"\ufe0f", 0},
		{"\u2764", 1},
		{"\u2764\ufe0e", 1},
		{"e\u0301\ufe0f", 2},
		{"\u2764\ufe0f\u200d\U0001F525", 4},
	} {
		if got := termtext.Width(tc.text); got != tc.want {
			t.Errorf("Width(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

// Of the format characters, the soft hyphen and the prepended concatenation
// marks are drawn, one column wide; the others take none.
func TestTheFormatCharactersATerminalDraws(t *testing.T) {
	t.Parallel()
	for _, r := range []rune{0x00ad, 0x0600, 0x0605, 0x06dd, 0x070f, 0x0890, 0x0891, 0x08e2, 0x110bd, 0x110cd} {
		if got := termtext.RuneWidth(r); got != 1 {
			t.Errorf("RuneWidth(%U) = %d, want 1", r, got)
		}
	}
	for _, r := range []rune{0, 0x200b, 0x200d, 0xfeff, 0x2060, 0x180e, 0xe0041} {
		if got := termtext.RuneWidth(r); got != 0 {
			t.Errorf("RuneWidth(%U) = %d, want 0", r, got)
		}
	}
	if got := termtext.Width(strings.Repeat("\u00ad", 200) + "ok"); got != 202 {
		t.Errorf("Width(200 soft hyphens and ok) = %d, want 202", got)
	}
}

// Characters that became wide after the Unicode version golang.org/x/text
// knows, such as the emoji of Unicode 16, take two columns too.
func TestCharactersWideSinceUnicode15(t *testing.T) {
	t.Parallel()
	for _, r := range []rune{
		0x2630, 0x2637, 0x268a, 0x2ffc, 0x31e4, 0x31ef, 0x4dc0, 0x4dff,
		0x16ff2, 0x187f8, 0x18cd6, 0x18cff, 0x18d09, 0x18d80, 0x18e00,
		0x191d2, 0x1b123, 0x1b168, 0x1d300, 0x1d376, 0x1f1ae, 0x1f6d8,
		0x1f7da, 0x1fa89, 0x1fabe, 0x1fac6, 0x1fac8, 0x1facc, 0x1fadc,
		0x1fadf, 0x1fae9, 0x1faef, 0x1fafa,
	} {
		if got := termtext.RuneWidth(r); got != 2 {
			t.Errorf("RuneWidth(%U) = %d, want 2", r, got)
		}
	}
	for _, r := range []rune{0x2638, 0x2689, 0x1d357} {
		if got := termtext.RuneWidth(r); got != 1 {
			t.Errorf("RuneWidth(%U) = %d, want 1", r, got)
		}
	}
}

// A mark that combines in the Unicode tables of Go but spaces in Unicode
// 18.0, the Ahom medial ra U+1171E, is drawn, and takes one column.
func TestAMarkThatSpacesSinceUnicode15(t *testing.T) {
	t.Parallel()
	if got := termtext.RuneWidth(0x1171e); got != 1 {
		t.Errorf("RuneWidth(U+1171E) = %d, want 1", got)
	}
	if got := termtext.Width("\U0001171e\U0001171d"); got != 1 {
		t.Errorf("Width(U+1171E U+1171D) = %d, want 1", got)
	}
}

// Truncate never lets text reach past the columns given, and leaves out a
// wide character whole rather than split it.
func TestTruncate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text string
		cols int
		want string
	}{
		{"exe0001", 7, "exe0001"},
		{"exe0001", 20, "exe0001"},
		{"exe0001", 3, "exe"},
		{"exe0001", 0, ""},
		{"exe0001", -1, ""},
		{"", 5, ""},
		{"失败了", 4, "失败"},
		{"失败了", 5, "失败"},
		{"a失败", 2, "a"},
		{"失败", 1, ""},
		{"e\u0301e\u0301", 1, "e\u0301"},
		{"\u2764\ufe0f\u2764\ufe0f", 3, "\u2764\ufe0f\u2764"},
		{"\u2764\ufe0f\u2764\ufe0f", 2, "\u2764\ufe0f"},
		{"\u2764\ufe0f", 1, "\u2764"},
		{"a\u2764\ufe0f", 2, "a\u2764"},
		{"1\ufe0f\u20e3 ok", 2, "1\ufe0f\u20e3"},
		{"\u00ad\u00ad\u00adok", 2, "\u00ad\u00ad"},
		{"\U0001FAE9\U0001FAE9", 3, "\U0001FAE9"},
		{"bad \xff", 4, "bad "},
	} {
		got := termtext.Truncate(tc.text, tc.cols)
		if got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tc.text, tc.cols, got, tc.want)
		}
		if w := termtext.Width(got); w > max(tc.cols, 0) {
			t.Errorf("Truncate(%q, %d) is %d columns wide", tc.text, tc.cols, w)
		}
	}
}

// FuzzTruncate checks that Truncate returns a prefix of its input, cut on a
// rune boundary, that fits in the columns given and is the longest that
// does, and the input itself when that fits already. It holds Width to
// bounds of its own too: no less than the RuneWidth of its runes added up,
// and no more than that with one column for each U+FE0F.
func FuzzTruncate(f *testing.F) {
	for _, seed := range []string{
		"", "exe0001", "失败了", "a\u200db", "😀 done", "e\u0301e\u0301",
		"\u2764\ufe0f\u2764\ufe0f", "1\ufe0f\u20e3", "\u00ad\u0600ok", "\U0001FAE9 new",
	} {
		f.Add(seed, 4)
	}
	f.Fuzz(func(t *testing.T, in string, cols int) {
		if !utf8.ValidString(in) {
			t.Skip("Truncate takes text that is already escaped")
		}
		low, selectors := 0, 0
		for _, r := range in {
			low += termtext.RuneWidth(r)
			if r == 0xfe0f {
				selectors++
			}
		}
		if w := termtext.Width(in); w < low || w > low+selectors {
			t.Fatalf("Width(%q) = %d, though its runes take %d and it holds %d U+FE0F", in, w, low, selectors)
		}
		got := termtext.Truncate(in, cols)
		if !strings.HasPrefix(in, got) || !utf8.ValidString(got) {
			t.Fatalf("Truncate(%q, %d) = %q, not a prefix cut on a rune", in, cols, got)
		}
		if termtext.Width(got) > max(cols, 0) {
			t.Fatalf("Truncate(%q, %d) = %q, %d columns wide", in, cols, got, termtext.Width(got))
		}
		if cols > 0 && termtext.Width(in) <= cols && got != in {
			t.Fatalf("Truncate(%q, %d) = %q, though the text fits", in, cols, got)
		}
		if cols > 0 && got != in {
			_, size := utf8.DecodeRuneInString(in[len(got):])
			if longer := in[:len(got)+size]; termtext.Width(longer) <= cols {
				t.Fatalf("Truncate(%q, %d) = %q, though %q fits too", in, cols, got, longer)
			}
		}
	})
}
