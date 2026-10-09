// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"strings"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// reset sets every attribute of the text that follows back, the colours
// included (SGR 0).
const reset = "\x1b[0m"

// cut shortens row to at most cols columns, as termtext.Truncate does, but
// reads the sequences that set colours and other attributes of text
// (SGR, ESC [ … m) as what they do: they take no column, and none is cut
// in two. The text of the row, without its sequences, is cut whole, as
// Truncate cuts it, so that a U+FE0F after a sequence counts as it would
// without one. A row that held an escape and was shortened ends with
// reset, so that no colour runs on into the rows below or the command's
// output; the sequences after the cut are left out.
//
// Any escape in a row is the display's own, since the Bus escapes every
// text of an event (progress.Sanitize), so the escapes cut finds are those
// a theme draws with. An escape that starts no such sequence is text, as
// termtext counts it.
func cut(row string, cols int) string {
	if strings.IndexByte(row, 0x1b) < 0 {
		return termtext.Truncate(row, cols)
	}
	var text strings.Builder
	for i := 0; i < len(row); {
		if seq := sgr(row[i:]); seq != "" {
			i += len(seq)
			continue
		}
		text.WriteByte(row[i])
		i++
	}
	keep := len(termtext.Truncate(text.String(), cols))
	if keep == text.Len() {
		return row
	}
	var b strings.Builder
	for i, n := 0, 0; n < keep; {
		if seq := sgr(row[i:]); seq != "" {
			b.WriteString(seq)
			i += len(seq)
			continue
		}
		b.WriteByte(row[i])
		i++
		n++
	}
	return b.String() + reset
}

// sgr returns the sequence of attributes s starts with, ESC [, parameters of
// digits, ";" and ":", and m; "" when s starts with none.
func sgr(s string) string {
	if !strings.HasPrefix(s, "\x1b[") {
		return ""
	}
	for i := 2; i < len(s); i++ {
		switch c := s[i]; {
		case c == 'm':
			return s[:i+1]
		case c >= '0' && c <= '9' || c == ';' || c == ':':
		default:
			return ""
		}
	}
	return ""
}
