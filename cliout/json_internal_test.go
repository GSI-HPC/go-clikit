// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliout

import "testing"

// A rune beyond the Basic Multilingual Plane, which termtext may come to
// escape, is written as a surrogate pair, which JSON asks for.
func TestAppendEscape(t *testing.T) {
	for r, want := range map[rune]string{
		0x7f:    `\u007f`,
		0x202e:  `\u202e`,
		0xe0001: `\udb40\udc01`,
	} {
		if got := string(appendEscape(nil, r)); got != want {
			t.Errorf("appendEscape(%U) = %s, want %s", r, got, want)
		}
	}
}
