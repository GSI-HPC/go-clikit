// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress_test

import (
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
)

// flag is the Setting of a --progress the command line gave.
func flag(word string) cliprogress.Setting {
	return cliprogress.Setting{Flag: "--progress", Given: true, Value: word, Variable: "PROG_PROGRESS", Env: "plain"}
}

// variable is the Setting of PROG_PROGRESS, with no flag given.
func variable(word string) cliprogress.Setting {
	return cliprogress.Setting{Flag: "--progress", Value: "counter", Variable: "PROG_PROGRESS", Env: word}
}

// What the flag asks for and cannot have refuses the command, so that the
// one who asked finds out why nothing shows; what the variable asks for,
// set once in a profile and inherited by scripts that never asked, fails
// none. auto draws the tree only where it can be drawn and nothing reads
// standard output on the same terminal.
func TestTheFlagIsRefusedWhereTheVariableShowsNothing(t *testing.T) {
	const notTerminal = "standard error is not a terminal"
	tests := []struct {
		name           string
		mode           cliprogress.Setting
		err, dumb, out bool
		want           cliprogress.Mode
		note, refused  string
	}{
		{"nothing asked, on a terminal", cliprogress.Setting{}, true, false, false, cliprogress.ModeTTY, "", ""},
		{"nothing asked, in a pipe", cliprogress.Setting{}, false, false, false, cliprogress.ModeNone, "", ""},
		{"auto on a dumb terminal", flag("auto"), true, true, false, cliprogress.ModeNone, "", ""},
		{"auto while standard output goes into a pipe", flag("auto"), true, false, true, cliprogress.ModeNone, "", ""},
		{"the variable's auto", variable("auto"), true, false, false, cliprogress.ModeTTY, "", ""},
		{"tty on a terminal", flag("tty"), true, false, true, cliprogress.ModeTTY, "", ""},
		{"counter on a terminal", flag("counter"), true, false, false, cliprogress.ModeCounter, "", ""},
		{"tty in a pipe", flag("tty"), false, false, false, cliprogress.ModeNone, "",
			"--progress asks for a live tree, but " + notTerminal + "; use none, or auto to draw one only where it can be"},
		{"counter in a pipe", flag("counter"), false, false, false, cliprogress.ModeNone, "",
			"--progress asks for a counter, but " + notTerminal + "; use none, or auto to draw one only where it can be"},
		{"tty on a dumb terminal", flag("tty"), true, true, false, cliprogress.ModeNone, "",
			"--progress asks for a live tree, but the terminal cannot draw one (TERM is dumb); use none, or auto to draw one only where it can be"},
		{"counter on a dumb terminal", flag("counter"), true, true, false, cliprogress.ModeNone, "",
			"--progress asks for a counter, but the terminal cannot draw one (TERM is dumb); use none, or auto to draw one only where it can be"},
		{"the variable's tty in a pipe", variable("tty"), false, false, false, cliprogress.ModeNone, "", ""},
		{"plain in a pipe", flag("plain"), false, false, false, cliprogress.ModePlain, "", ""},
		{"none on a terminal", flag("none"), true, false, false, cliprogress.ModeNone, "", ""},
		{"a word the flag does not take", flag("tree"), true, false, false, cliprogress.ModeNone, "",
			`--progress is "tree"; it takes one of auto, tty, counter, plain, none`},
		{"an empty flag", flag(""), true, false, false, cliprogress.ModeNone, "",
			`--progress is ""; it takes one of auto, tty, counter, plain, none`},
		{"a word the variable does not take", variable("plian"), true, false, false, cliprogress.ModeNone,
			`prog: PROG_PROGRESS is "plian"; it takes one of auto, tty, counter, plain, none; no progress is shown`, ""},
	}
	for _, tc := range tests {
		got, note, err := cliprogress.Choose(cliprogress.Options{
			Program: "prog", Mode: tc.mode, OnTerminal: tc.err, Dumb: tc.dumb, IntoPipe: tc.out,
		})
		refused := ""
		if err != nil {
			refused = err.Error()
		}
		if got != tc.want || note != tc.note || refused != tc.refused {
			t.Errorf("%s: Choose = %v, %q, %q; want %v, %q, %q", tc.name, got, note, refused, tc.want, tc.note, tc.refused)
		}
	}
}

// A program with no name of its own writes notes that name none.
func TestANoteNamesNoProgramWithoutOne(t *testing.T) {
	_, note, _ := cliprogress.Choose(cliprogress.Options{Mode: variable("bogus")})
	if want := `PROG_PROGRESS is "bogus"; it takes one of auto, tty, counter, plain, none; no progress is shown`; note != want {
		t.Errorf("note = %q, want %q", note, want)
	}
}

// The words are what a flag's help and completion list, in this order.
func TestModesAreNamedByTheirWords(t *testing.T) {
	var words []string
	for _, m := range cliprogress.Modes() {
		words = append(words, m.String())
	}
	if got, want := strings.Join(words, ","), "auto,tty,counter,plain,none"; got != want {
		t.Errorf("Modes = %s, want %s", got, want)
	}
	for m, want := range map[cliprogress.Mode]string{0: "mode(0)", cliprogress.ModeNone + 1: "mode(6)"} {
		if got := m.String(); got != want {
			t.Errorf("Mode(%d).String() = %q, want %q", m, got, want)
		}
	}
}

// The locale is read the way the C library reads it, LC_ALL first, so
// that a display draws the marks the terminal can show.
func TestTheLocaleIsReadAsTheCLibraryReadsIt(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{}, false},
		{map[string]string{"LANG": "en_GB.UTF-8"}, true},
		{map[string]string{"LANG": "de_DE.utf8"}, true},
		{map[string]string{"LANG": "en_GB.UTF-8", "LC_CTYPE": "C"}, false},
		{map[string]string{"LC_CTYPE": "C", "LC_ALL": "C.UTF-8"}, true},
		{map[string]string{"LANG": "POSIX"}, false},
	}
	for _, tc := range tests {
		if got := cliprogress.UTF8Locale(func(name string) string { return tc.env[name] }); got != tc.want {
			t.Errorf("UTF8Locale(%v) = %v, want %v", tc.env, got, tc.want)
		}
	}
}
