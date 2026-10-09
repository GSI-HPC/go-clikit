// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progresstest

import (
	"io"
	"testing"
)

// The screen shows the bytes the way a terminal does: rows written over,
// moved up to and erased, what is below the cursor cleared, and anything
// else visible as text.
func TestScreenShowsWhatATerminalWould(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		width  int
		writes []string
		want   string
	}{
		{"plain lines", 0, []string{"one\n", "two\n"}, "one\ntwo\n"},
		{"a row not ended", 0, []string{"Continue? [y/N] "}, "Continue? [y/N] \n"},
		{"a carriage return writes over", 0, []string{"50%\r100%\n"}, "100%\n"},
		{"a row erased", 0, []string{"done\n", "counter", "\r\x1b[2K", "next\n"}, "done\nnext\n"},
		{"a region of three rows erased", 0,
			[]string{"above\n", "a\nb\nc", "\r\x1b[2K\x1b[1A\x1b[2K\x1b[1A\x1b[2K", "below\n"}, "above\nbelow\n"},
		{"a region drawn again in place", 0,
			[]string{"a\nb", "\r\x1b[2K\x1b[1A\x1b[2K", "c\nd"}, "c\nd\n"},
		{"up by a count", 0, []string{"a\nb\nc", "\x1b[2A", "\rX"}, "X\nb\nc\n"},
		{"the rest of a row erased", 0, []string{"abcdef\r", "ab\x1b[K"}, "ab\n"},
		{"all below erased", 0, []string{"a\nb\nc", "\x1b[1A\r\x1b[J", "z"}, "a\nz\n"},
		{"up past the top", 0, []string{"a", "\x1b[5A\rb"}, "b\n"},
		{"a sequence it does not know", 0, []string{"\x1b[?25lhidden"}, "^[[?25lhidden\n"},
		{"a control character", 0, []string{"bell\a\n"}, "bell^G\n"},
		{"split across writes", 0, []string{"✓ a\r\x1b", "[2", "K\xe2\x9c", "\x93 b"}, "✓ b\n"},
		{"a row too long wraps", 4, []string{"abcdef", "\r\x1b[2K"}, "abcd\n"},
		{"a row as long as the width does not", 4, []string{"abcd", "\r\x1b[2K"}, ""},
		{"an escape that starts no sequence", 0, []string{"\x1b7saved\n"}, "^[7saved\n"},
		{"a wide rune half written over leaves the other half blank", 0, []string{"失败\rX\n"}, "X 败\n"},
		{"a wide rune whose left half is written over", 0, []string{"a失败\rab\n"}, "ab 败\n"},
		{"a wide rune whose right half is written over", 0, []string{"a失\nbc", "\x1b[1Az"}, "a z\nbc\n"},
		{"a wide rune over the left half of another", 0, []string{"a失败\r失\n"}, "失 败\n"},
		{"a wide rune erased from its right half", 0, []string{"a失\nbc", "\x1b[1A\x1b[K"}, "a\nbc\n"},
		{"a wide rune wraps when one column is left", 4, []string{"a失败"}, "a失\n败\n"},
		{"an escape not ended shows", 0, []string{"done\x1b"}, "done^[\n"},
		{"a sequence not ended shows", 0, []string{"done\x1b[1"}, "done^[[1\n"},
		{"a rune not ended shows", 0, []string{"done\xe2\x9c"}, "done\ufffd\ufffd\n"},
		{"a sequence cut by a newline", 0, []string{"a\x1b[\n12/34\n"}, "a^[[\n12/34\n"},
		{"a sequence cut by an escape", 0, []string{"a\x1b[1\x1b[K\n"}, "a^[[1\n"},
		{"an escape before a newline", 0, []string{"x\x1b\ny", "\x1b[2K"}, "x^[\n"},
		{"an escape before an escape", 0, []string{"x\x1b\x1by"}, "x^[^[y\n"},
		{"an escape before a rune", 0, []string{"x\x1b✓"}, "x^[✓\n"},
		{"a full row erased to its end", 4, []string{"abcd\x1b[K\n"}, "abc\n"},
		{"a full row wraps at the next rune", 4, []string{"abcd", "e"}, "abcd\ne\n"},
		{"a full row returned to", 4, []string{"abcd\rx"}, "xbcd\n"},
		{"a full row and a newline", 4, []string{"abcd\nx"}, "abcd\nx\n"},
		{"a full row moved up from", 4, []string{"a\nbcde", "\x1b[1Ax"}, "a  x\nbcde\n"},
		{"a full row erased below", 4, []string{"abcd\x1b[J"}, "abc\n"},
		{"a wide rune at the end of a full row erased", 3, []string{"a失\x1b[K"}, "a\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Screen{Width: tc.width}
			for _, w := range tc.writes {
				if n, err := io.WriteString(s, w); n != len(w) || err != nil {
					t.Fatalf("Write(%q) = %d, %v", w, n, err)
				}
			}
			if got := s.String(); got != tc.want {
				t.Errorf("screen:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// What String shows of a sequence the output ends in the middle of is a
// view: the next write still completes the sequence.
func TestScreenCompletesWhatItShowedUnfinished(t *testing.T) {
	t.Parallel()
	s := &Screen{}
	io.WriteString(s, "ab\r\x1b")
	if got, want := s.String(), "^[\n"; got != want {
		t.Errorf("screen before the sequence ends: %q, want %q", got, want)
	}
	io.WriteString(s, "[2Kc")
	if got, want := s.String(), "c\n"; got != want {
		t.Errorf("screen after it ends: %q, want %q", got, want)
	}
}

// With Styles, the sequences that set the attributes of text take no
// column and String shows the text alone, while Styled shows which
// attributes each run was written in, however they were set.
func TestScreenAppliesStylesWhenAsked(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		width  int
		writes []string
		text   string
		styled string
	}{
		{"a colour and its reset", 0, []string{"\x1b[31m✗\x1b[0m exe0007\n"}, "✗ exe0007\n", "«31»✗«» exe0007\n"},
		{"ESC [ m sets everything back", 0, []string{"\x1b[1mok\x1b[m done\n"}, "ok done\n", "«1»ok«» done\n"},
		{"each attribute in one order", 0, []string{"\x1b[7;4;2;1;94mx\x1b[0m\n"}, "x\n", "«1;2;4;7;94»x«»\n"},
		{"attributes set by several sequences", 0, []string{"\x1b[94m\x1b[1mx\x1b[0m\n"}, "x\n", "«1;94»x«»\n"},
		{"a colour of 256 colours", 0, []string{"\x1b[1;38;5;30m✓\x1b[0m\n"}, "✓\n", "«1;38;5;30»✓«»\n"},
		{"a colour of 256 colours written with colons", 0, []string{"\x1b[38:5:208mx\x1b[0m\n"}, "x\n", "«38;5;208»x«»\n"},
		{"the resets of each attribute", 0,
			[]string{"\x1b[1;2;4;7;31ma\x1b[22mb\x1b[24mc\x1b[27md\x1b[39me\n"}, "abcde\n",
			"«1;2;4;7;31»a«4;7;31»b«7;31»c«31»d«»e\n"},
		{"an empty parameter sets everything back", 0, []string{"\x1b[1;;32mx\n"}, "x\n", "«32»x«»\n"},
		{"a row that ends in a colour closes it", 0, []string{"\x1b[33mone\ntwo\x1b[0m\n"}, "one\ntwo\n", "«33»one«»\n«33»two«»\n"},
		// Styled shows the attributes of the cells, not the sequences: a row
		// whose colour is set back after it reads as one whose colour is
		// left on, which shows only in the text written after it.
		{"a row whose colour is set back", 0, []string{"\x1b[33mtwo\x1b[0m\n"}, "two\n", "«33»two«»\n"},
		{"a row whose colour is left on", 0, []string{"\x1b[33mtwo\n"}, "two\n", "«33»two«»\n"},
		{"a colour left on shows in the text after it", 0, []string{"\x1b[33mtwo\n", "after\n"}, "two\nafter\n", "«33»two«»\n«33»after«»\n"},
		{"colours take no column where a row wraps", 4, []string{"\x1b[32mabcd\x1b[0m", "\x1b[31me\x1b[0m"}, "abcd\ne\n", "«32»abcd«»\n«31»e«»\n"},
		{"writing over a cell takes the new attributes", 0, []string{"\x1b[31mabc\x1b[0m\rx\n"}, "xbc\n", "x«31»bc«»\n"},
		{"an erase takes the attributes off", 0, []string{"\x1b[31mabc\r\x1b[2K", "\x1b[0mx\n"}, "x\n", "x\n"},
		{"an erase to the end of the row", 0, []string{"\x1b[31mabc\x1b[0m\rx\x1b[K\n"}, "x\n", "x\n"},
		{"an erase below", 0, []string{"\x1b[31ma\nb\x1b[0m\x1b[1A\r\x1b[J\n"}, "", ""},
		{"a wide rune keeps one style for both columns", 0, []string{"\x1b[35m失\x1b[0m败\n"}, "失败\n", "«35»失«»败\n"},
		{"a wide rune half written over leaves a blank of none", 0, []string{"\x1b[35m失\x1b[0m\rx\n"}, "x \n", "x \n"},
		{"the gap a cursor left has none", 0, []string{"a\x1b[31m\n", "\x1b[1Axyz\x1b[0m\n"}, "xyz\n", "«31»xyz«»\n"},
		{"a sequence split across writes", 0, []string{"\x1b[3", "1mx\x1b[0m\n"}, "x\n", "«31»x«»\n"},
		{"italic is shown as text", 0, []string{"\x1b[3mx\n"}, "^[[3mx\n", "^[[3mx\n"},
		{"a background is shown as text", 0, []string{"\x1b[41mx\n"}, "^[[41mx\n", "^[[41mx\n"},
		{"a colour of 24 bits is shown as text", 0, []string{"\x1b[38;2;1;2;3mx\n"}, "^[[38;2;1;2;3mx\n", "^[[38;2;1;2;3mx\n"},
		{"a colour of 256 colours with no index", 0, []string{"\x1b[38;5mx\n"}, "^[[38;5mx\n", "^[[38;5mx\n"},
		{"a colour of 256 colours past the last", 0, []string{"\x1b[38;5;256mx\n"}, "^[[38;5;256mx\n", "^[[38;5;256mx\n"},
		{"a colour index that is no number", 0, []string{"\x1b[38;5;xmx\n"}, "^[[38;5;xmx\n", "^[[38;5;xmx\n"},
		{"a code that is no number", 0, []string{"\x1b[1:2mx\n"}, "^[[1:2mx\n", "^[[1:2mx\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Screen{Width: tc.width, Styles: true}
			for _, w := range tc.writes {
				io.WriteString(s, w)
			}
			if got := s.String(); got != tc.text {
				t.Errorf("String:\n%q\nwant:\n%q", got, tc.text)
			}
			if got := s.Styled(); got != tc.styled {
				t.Errorf("Styled:\n%q\nwant:\n%q", got, tc.styled)
			}
		})
	}
}

// Without Styles, a sequence that sets attributes is shown as text, as any
// sequence the Screen does not know is, and Styled shows what String does.
func TestScreenShowsStylesAsTextUnlessAsked(t *testing.T) {
	t.Parallel()
	s := &Screen{}
	io.WriteString(s, "\x1b[31m✗\x1b[0m exe0007\n")
	want := "^[[31m✗^[[0m exe0007\n"
	if got := s.String(); got != want {
		t.Errorf("String: %q, want %q", got, want)
	}
	if got := s.Styled(); got != want {
		t.Errorf("Styled: %q, want %q", got, want)
	}
}

// Styled, like String, shows a sequence the output ends in the middle of as
// text, in the attributes then on, and the next write completes it.
func TestScreenStylesWhatItShowedUnfinished(t *testing.T) {
	t.Parallel()
	s := &Screen{Styles: true}
	io.WriteString(s, "\x1b[32mok\x1b[")
	if got, want := s.Styled(), "«32»ok^[[«»\n"; got != want {
		t.Errorf("Styled before the sequence ends: %q, want %q", got, want)
	}
	io.WriteString(s, "0m done\n")
	if got, want := s.Styled(), "«32»ok«» done\n"; got != want {
		t.Errorf("Styled after it ends: %q, want %q", got, want)
	}
}
