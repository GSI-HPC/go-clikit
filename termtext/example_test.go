// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext_test

import (
	"fmt"
	"os"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// A line a remote host printed, which tries to retitle the terminal and to
// overwrite what was printed before it, is shown as it was sent.
func ExampleEscapeLines() {
	remote := "backup done\x1b]0;owned\x07\rbackup FAILED\n\tsee the log\n"
	fmt.Print(termtext.EscapeLines(remote))
	// Output:
	// backup done\x1b]0;owned\x07\rbackup FAILED
	// 	see the log
}

// A value stays on one line, with its newline and tab shown.
func ExampleEscape() {
	fmt.Println(termtext.Escape("rack 1\nrow\t2"))
	fmt.Println(termtext.Escape("\u202eexe01"))
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

// Wrap fills a paragraph to a number of columns, with an indent for its
// first line and for the others, and never cuts a word.
func ExampleWrap() {
	text := "Run the command on every host in the set, one after the other, and stop at the first failure."
	fmt.Print(termtext.Wrap(text, 36, "  -x, --exit  ", "              "))
	// Output:
	//   -x, --exit  Run the command on
	//               every host in the set,
	//               one after the other,
	//               and stop at the first
	//               failure.
}

// The output of a program is written escaped, line by line, as it comes,
// whatever the writes were cut as.
func ExampleNewEscapeWriter() {
	w := termtext.NewEscapeWriter(os.Stdout)
	fmt.Fprint(w, "copying\x1b]0;owned\x07 20%\r")
	fmt.Fprint(w, "copying 100%\n\tdone")
	w.Close()
	// Output:
	// copying\x1b]0;owned\x07 20%\rcopying 100%
	// 	done
}
