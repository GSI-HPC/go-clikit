// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// hostile is what a compromised node answered in the review: a carriage
// return and cursor-up to overwrite the line printed for another node, and
// OSC 52 to write the clipboard.
const hostile = "ok\r\x1b[1Aexe0001: \x1b]52;c;ZXZpbA==\x07evil"

func TestEscapeLines(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"plain text", "plain text"},
		{"two\nlines\n", "two\nlines\n"},
		{"tab\tseparated", "tab\tseparated"},
		// A carriage return just before a newline ends the line with it
		// (decision 42); any other one is escaped.
		{"crlf\r\nlines\r\n", "crlf\r\nlines\r\n"},
		{"10%\r20%\r\n", `10%\r20%` + "\r\n"},
		{"two\r\r\n", `two\r` + "\r\n"},
		{"cr at the end\r", `cr at the end\r`},
		{"cr then lf\n\r", "cr then lf\n" + `\r`},
		{"\r\n", "\r\n"},
		{hostile, `ok\r\x1b[1Aexe0001: \x1b]52;c;ZXZpbA==\x07evil`},
		{"nul\x00del\x7f", `nul\x00del\x7f`},
		{"c1 \u009b31m", `c1 \u009b31m`},
		{"bidi \u202eevil", `bidi \u202eevil`},
		{"lines\u2028and\u2029paragraphs", `lines\u2028and\u2029paragraphs`},
		{"zero\u200bwidth", "zero\u200bwidth"},
		{"bad utf-8 \xff\xfe", `bad utf-8 \xff\xfe`},
		{"grüße", "grüße"},
	}
	for _, tc := range tests {
		if got := termtext.EscapeLines(tc.in); got != tc.want {
			t.Errorf("EscapeLines(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEscape(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"firmware update, ticket 4711", "firmware update, ticket 4711"},
		{"bad\nexe0002  idle   fine", `bad\nexe0002  idle   fine`},
		{"cr\rtab\t", `cr\rtab\t`},
		{"crlf\r\n", `crlf\r\n`},
		{hostile, `ok\r\x1b[1Aexe0001: \x1b]52;c;ZXZpbA==\x07evil`},
		// A backslash is written as it is, so a literal escape looks like
		// one the escaper wrote; the package comment says why.
		{`C:\temp\x1b`, `C:\temp\x1b`},
	}
	for _, tc := range tests {
		if got := termtext.Escape(tc.in); got != tc.want {
			t.Errorf("Escape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// FuzzEscape checks what the escapers promise of any input: the result is
// valid UTF-8 and holds no rune the policy escapes, save the carriage
// return of a CRLF line end in what EscapeLines returns, which keeps every
// one of them, Escape's result is one line, and text that needs no escape
// comes back unchanged.
func FuzzEscape(f *testing.F) {
	for _, seed := range []string{"", "plain", hostile, "a\nb\tc", "\xff\xfe", "\u009b", "\u2028", "\u202e", "失败", "a\r\nb\r\r\n\r", "\r\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		line, lines := termtext.Escape(in), termtext.EscapeLines(in)
		for name, got := range map[string]string{"Escape": line, "EscapeLines": lines} {
			if !utf8.ValidString(got) {
				t.Fatalf("%s(%q) = %q, not UTF-8", name, in, got)
			}
			for i, r := range got {
				if r == '\r' && name == "EscapeLines" && strings.HasPrefix(got[i:], "\r\n") {
					continue
				}
				if r < 0x20 && r != '\n' && r != '\t' || r >= 0x7f && r <= 0x9f ||
					r == 0x061c || r == 0x200e || r == 0x200f || r >= 0x202a && r <= 0x202e ||
					r >= 0x2066 && r <= 0x2069 || r == 0x2028 || r == 0x2029 {
					t.Fatalf("%s(%q) = %q, which holds %U", name, in, got, r)
				}
			}
		}
		if n := strings.Count(in, "\r\n"); strings.Count(lines, "\r\n") != n {
			t.Fatalf("EscapeLines(%q) = %q, which does not keep its %d CRLF line ends", in, lines, n)
		}
		if strings.ContainsAny(line, "\n\t") {
			t.Fatalf("Escape(%q) = %q, not one line", in, line)
		}
		if termtext.Escape(line) != line {
			t.Fatalf("Escape(%q) = %q, which Escape changes again", in, line)
		}
		if termtext.EscapeLines(lines) != lines {
			t.Fatalf("EscapeLines(%q) = %q, which EscapeLines changes again", in, lines)
		}
		if line == in && lines != in {
			t.Fatalf("EscapeLines(%q) = %q, though Escape changes nothing", in, lines)
		}
	})
}
