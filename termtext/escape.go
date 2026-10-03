// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Escape makes untrusted text safe to write to a terminal as one line:
// untrusted text from a remote host or a container, say, whose control
// characters would otherwise act on the terminal. It replaces every rune
// the package's escape policy names with a visible escape such as \x1b or
// \u009b, and a byte that is not UTF-8 with one such as \xff. Newline and
// tab are escaped too, as \n and \t, so that a message, a log line or a
// table cell cannot start a line that seems to be another.
//
// Text that holds none of these is returned unchanged. Escaping is
// idempotent: Escape changes nothing in text it returned, so text escaped
// once, such as an event's text, can be escaped again safely.
func Escape(s string) string {
	return escape(s, true)
}

// EscapeLines is Escape for text whose lines are kept, such as the output
// of a program: newline and tab are written as they are, since they cannot
// move the cursor back over text already written. Text that has to stay on
// one line needs Escape. Escaping is idempotent: EscapeLines changes
// nothing in text it returned.
func EscapeLines(s string) string {
	return escape(s, false)
}

func escape(s string, oneLine bool) string {
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if needsEscape(r, size, oneLine) {
			break
		}
		i += size
	}
	if i == len(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 16)
	b.WriteString(s[:i])
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case !needsEscape(r, size, oneLine):
			b.WriteString(s[i : i+size])
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x80:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
		i += size
	}
	return b.String()
}

// needsEscape reports whether a rune may not reach a terminal as it is.
func needsEscape(r rune, size int, oneLine bool) bool {
	switch {
	case r == utf8.RuneError && size == 1:
		// A byte that is not UTF-8, such as a lone 0x9b, which some
		// terminals read as the start of a control sequence.
		return true
	case r == '\n' || r == '\t':
		return oneLine
	case r < 0x20, r == 0x7f:
		return true
	case r >= 0x80 && r <= 0x9f:
		return true
	case r == 0x061c, r == 0x200e, r == 0x200f,
		r >= 0x202a && r <= 0x202e,
		r >= 0x2066 && r <= 0x2069:
		// Bidirectional controls, which make a terminal show text in an
		// order other than the one it has.
		return true
	case r == 0x2028, r == 0x2029:
		// The Unicode line and paragraph separators.
		return true
	default:
		return false
	}
}
