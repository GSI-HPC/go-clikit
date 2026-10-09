// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GSI-HPC/go-clikit/termtext"
)

func TestCutReadsColoursAsTakingNoColumn(t *testing.T) {
	const red, green = "\x1b[31m", "\x1b[38;5;42m"
	for _, tc := range []struct {
		name string
		row  string
		cols int
		want string
	}{
		{"plain text is cut as termtext cuts it", "abcdef", 3, "abc"},
		{"a row that fits is kept whole", red + "ok" + reset, 2, red + "ok" + reset},
		{"colours take no column", red + "✓" + reset + " exe0001", 9, red + "✓" + reset + " exe0001"},
		{"a cut inside a colour sets it back", red + "abcdef" + reset, 3, red + "abc" + reset},
		{"a cut row ends with reset", red + "a" + reset + "bcdef", 3, red + "a" + reset + "bc" + reset},
		{"the sequences after the cut are left out", red + "ab" + reset + green + "cd" + reset, 2, red + "ab" + reset},
		{"a colour of 256 colours is one sequence", green + "abc" + reset, 1, green + "a" + reset},
		{"ESC [ m is a sequence too", red + "a\x1b[mbcd", 2, red + "a\x1b[mb" + reset},
		{"a wide rune that would reach past is left out", red + "a界" + reset, 2, red + "a" + reset},
		{"no columns keep nothing", red + "abc" + reset, 0, reset},
		{"an escape that starts no colour counts as text", "\x1b]0;x" + red + "y" + reset, 3, "\x1b]0" + reset},
		{"a colour after an escape that starts none is read", "\x1b]" + red + "abc" + reset, 3, "\x1b]" + red + "a" + reset},
		{"an unfinished sequence counts as text", "a\x1b[31", 3, "a\x1b[" + reset},
		{"a sequence of other parameters counts as text", "\x1b[?25lab", 3, "\x1b[?" + reset},
		// U+FE0F after a rune of one column makes it two, though a
		// colour stands between them, as termtext counts the text whole.
		{"a U+FE0F after a colour counts with the rune before it", "  " + red + "\ufe0fpower on" + reset, 4, "  " + red + "\ufe0fp" + reset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cut(tc.row, tc.cols); got != tc.want {
				t.Errorf("cut(%q, %d) = %q, want %q", tc.row, tc.cols, got, tc.want)
			}
		})
	}
}

// Text with no colours is cut exactly as termtext.Truncate cuts it, so that
// a display drawn with no theme writes what it wrote before.
func TestCutOfPlainTextIsTruncate(t *testing.T) {
	for _, row := range []string{"", "✓ exe[0001-0006]", "界界界", "a❤️b", "provision reinstall · 4:12.6"} {
		for cols := range 12 {
			if got, want := cut(row, cols), termtext.Truncate(row, cols); got != want {
				t.Errorf("cut(%q, %d) = %q, Truncate gives %q", row, cols, got, want)
			}
		}
	}
}

// The Terminal cuts a row by the columns it shows, so that a row drawn in
// colour fills the width without wrapping, and sets the colour back where
// it cut one short. A frame drawn in colour starts from none, so that a
// colour the command left on does not tint it.
func TestTheTerminalCutsAColouredRowByWhatItShows(t *testing.T) {
	var b strings.Builder
	term := NewTerminal(&b, TerminalOptions{Size: func() (int, int, error) { return 6, 10, nil }})
	term.attach(nil)
	term.draw([]string{"\x1b[32m✓\x1b[0m exe0001", "\x1b[31m✗ exe0002\x1b[0m"})
	want := eraseLine + reset + "\x1b[32m✓\x1b[0m exe\x1b[0m\n\x1b[31m✗ exe\x1b[0m"
	if got := b.String(); got != want {
		t.Errorf("the terminal wrote %q, want %q", got, want)
	}
}

// FuzzCut checks that cut keeps of a row's text what termtext.Truncate
// keeps of it, with the row's sequences of attributes whole and none after
// the cut, and that a row holding an escape that it shortened ends with
// reset, so that no colour runs on.
func FuzzCut(f *testing.F) {
	for _, seed := range []string{
		"", "exe0001", "\x1b[31m✗\x1b[0m exe0007", "\x1b[1;35mprovision\x1b[0m\x1b[2m · \x1b[0m4:12.3",
		"  \x1b[1m\ufe0fpower on\x1b[0m", "\x1b[38;5;42m界界\x1b[0m", "a\x1b[31", "\x1b]0;x\x1b[mok",
	} {
		f.Add(seed, 4)
	}
	f.Fuzz(func(t *testing.T, row string, cols int) {
		if !utf8.ValidString(row) {
			t.Skip("a display's rows are UTF-8")
		}
		got := cut(row, cols)
		if want := termtext.Truncate(visible(row), cols); visible(got) != want {
			t.Fatalf("cut(%q, %d) shows %q, want %q", row, cols, visible(got), want)
		}
		if got == row || !strings.Contains(row, "\x1b") {
			// Text with no escape is cut by Truncate alone, which the
			// first check holds it to.
			return
		}
		kept, ok := strings.CutSuffix(got, reset)
		if !ok || !strings.HasPrefix(row, kept) {
			t.Fatalf("cut(%q, %d) = %q, not a prefix of the row ended by %q", row, cols, got, reset)
		}
		if len(visible(got)) == len(visible(row)) {
			t.Fatalf("cut(%q, %d) = %q, changed a row whose text fits", row, cols, got)
		}
	})
}
