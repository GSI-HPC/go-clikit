// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext_test

import (
	"fmt"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// A line a remote host printed, which tries to retitle the terminal and to
// overwrite what was printed before it, is shown as it was sent.
func ExampleEscapeText() {
	remote := "backup done\x1b]0;owned\x07\rbackup FAILED\n\tsee the log\n"
	fmt.Print(termtext.EscapeText(remote))
	// Output:
	// backup done\x1b]0;owned\x07\rbackup FAILED
	// 	see the log
}

// A cell of a table stays on one line, with its newline and tab shown.
func ExampleEscapeCell() {
	fmt.Println(termtext.EscapeCell("rack 1\nrow\t2"))
	fmt.Println(termtext.EscapeCell("\u202eexe01"))
	// Output:
	// rack 1\nrow\t2
	// \u202eexe01
}

// Width counts the columns a terminal gives text, two for a wide rune.
func ExampleWidth() {
	for _, s := range []string{"exe0001", "Grüße", "ノード"} {
		fmt.Println(s, len(s), termtext.Width(s))
	}
	// Output:
	// exe0001 7 7
	// Grüße 7 5
	// ノード 9 6
}

// Truncate cuts text to a number of columns, and never splits a wide rune.
func ExampleTruncate() {
	fmt.Printf("%q\n", termtext.Truncate("dial tcp: i/o timeout", 8))
	fmt.Printf("%q\n", termtext.Truncate("ノード", 5))
	// Output:
	// "dial tcp"
	// "ノー"
}
