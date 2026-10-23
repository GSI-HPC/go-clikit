// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// Words go on a line while the line, a space and the word fit in the width,
// the first line starts with first, and the others with rest.
func TestWrapFillsLinesToTheWidth(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		text        string
		cols        int
		first, rest string
		want        string
	}{
		{"fits on one line", "one two three", 13, "", "", "one two three\n"},
		{"one column short", "one two three", 12, "", "", "one two\nthree\n"},
		{"every word on its own line", "one two three", 5, "", "", "one\ntwo\nthree\n"},
		{"an indent counts", "one two three", 12, "  ", "    ", "  one two\n    three\n"},
		{"a hanging indent", "ab cd ef gh", 7, "-x ", "   ", "-x ab\n   cd\n   ef\n   gh\n"},
		{"newlines and runs of spaces collapse", " one \n\n two\t\tthree  ", 40, "", "", "one two three\n"},
		{"a word wider than the width stays whole", "a abcdefghij b", 4, "", "", "a\nabcdefghij\nb\n"},
		{"the first word stays on the first line", "abcdefghij b", 4, "> ", "", "> abcdefghij\nb\n"},
		{"wide runes take two columns", "ノード ノード", 6, "", "", "ノード\nノード\n"},
		{"wide runes that fit", "ノード ab", 9, "", "", "ノード ab\n"},
		{"len is not the width", "Grüße Grüße", 11, "", "", "Grüße Grüße\n"},
		{"cols of zero puts every word on one line", "one two three", 0, "> ", "  ", "> one two three\n"},
		{"negative cols too", "one two three", -3, "", "", "one two three\n"},
		{"no words", " \n\t ", 10, "> ", "  ", ""},
		{"no text", "", 10, "> ", "  ", ""},
		{"a selector widens the rune before it", "a b❤️", 4, "", "", "a\nb❤️\n"},
		{"a selector after a space widens the space", "a ️b", 3, "", "", "a\n️b\n"},
	} {
		if got := termtext.Wrap(tc.text, tc.cols, tc.first, tc.rest); got != tc.want {
			t.Errorf("%s: Wrap(%q, %d, %q, %q) = %q, want %q", tc.name, tc.text, tc.cols, tc.first, tc.rest, got, tc.want)
		}
	}
}

// No line is wider than the width, unless it is one word that is.
func TestWrapKeepsLinesWithinTheWidth(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("ノード exe0001 Grüße a ", 20) + "ααααααααααααααααααααα"
	for cols := 1; cols <= 40; cols++ {
		got := termtext.Wrap(text, cols, "  ", "    ")
		if !strings.HasSuffix(got, "\n") {
			t.Fatalf("Wrap at %d columns = %q, which does not end in a newline", cols, got)
		}
		for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
			if termtext.Width(line) > cols && len(strings.Fields(line)) != 1 {
				t.Errorf("Wrap at %d columns made the line %q, %d columns wide", cols, line, termtext.Width(line))
			}
		}
		if strings.Join(strings.Fields(got), " ") != strings.Join(strings.Fields(text), " ") {
			t.Errorf("Wrap at %d columns changed the words: %q", cols, got)
		}
	}
}

// FuzzWrap checks that Wrap ends every line with a newline, keeps the words
// in order, starts the lines with first and rest, and makes no line wider
// than cols columns unless it holds one word only.
func FuzzWrap(f *testing.F) {
	for _, seed := range []string{
		"", "one two three", "ノード exe0001 Grüße", "a ️b ❤️", "abcdefghij b", " \n\t ",
	} {
		f.Add(seed, 6, "> ", "  ")
	}
	f.Fuzz(func(t *testing.T, s string, cols int, first, rest string) {
		if !utf8.ValidString(s) || !utf8.ValidString(first) || !utf8.ValidString(rest) {
			t.Skip("Wrap takes text that is already escaped")
		}
		if strings.Contains(first+rest, "\n") || len(strings.Fields(first+rest)) != 0 {
			t.Skip("first and rest are indents")
		}
		got := termtext.Wrap(s, cols, first, rest)
		words := strings.Fields(s)
		if len(words) == 0 {
			if got != "" {
				t.Fatalf("Wrap(%q, %d, %q, %q) = %q, want no lines", s, cols, first, rest, got)
			}
			return
		}
		if !strings.HasSuffix(got, "\n") {
			t.Fatalf("Wrap(%q, %d, %q, %q) = %q, which does not end in a newline", s, cols, first, rest, got)
		}
		if again := strings.Fields(got); strings.Join(again, " ") != strings.Join(words, " ") {
			t.Fatalf("Wrap(%q, %d, %q, %q) = %q, which changed the words", s, cols, first, rest, got)
		}
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if cols <= 0 && len(lines) != 1 {
			t.Fatalf("Wrap(%q, %d, %q, %q) = %q, want one line", s, cols, first, rest, got)
		}
		for i, line := range lines {
			indent := rest
			if i == 0 {
				indent = first
			}
			if !strings.HasPrefix(line, indent) {
				t.Fatalf("Wrap(%q, %d, %q, %q) made the line %q, which does not start with %q", s, cols, first, rest, line, indent)
			}
			if cols > 0 && termtext.Width(line) > cols && len(strings.Fields(line)) != 1 {
				t.Fatalf("Wrap(%q, %d, %q, %q) made the line %q, %d columns wide", s, cols, first, rest, line, termtext.Width(line))
			}
		}
	})
}
