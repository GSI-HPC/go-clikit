// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package cliprogress is the command line's side of progress: the words a
// flag such as --progress takes, the rule that picks a display from them
// and from what the program found out about its streams, and a Run, which
// makes the Bus, the display, the summary and the event log of one command
// and takes them down again, in order, once the command has ended.
//
// The words are auto, tty, counter, plain and none, and the event log is a
// file:
//
//	--progress auto|tty|counter|plain|none
//	--progress-log FILE
//
// auto, which holds when neither the flag nor the variable that stands in
// for it names a word, draws the live tree on a standard error that is a
// terminal, and not a dumb one, while standard output goes into no pipe,
// and nothing elsewhere. What the flag asks for and cannot have refuses
// the command: Choose and Start return an error, which the program reports
// as it reports its other usage errors. What the variable asks for fails
// no command, since it is set once, in a profile, and inherited by scripts
// and CI jobs that asked for nothing: it shows nothing, with a note where
// it names no word or no log that can be used.
//
// The package finds out nothing for itself and reads no environment. The
// program names its flags and variables and passes what they hold, in a
// Setting each, since the package imports no package of flags; it says
// whether standard error is a terminal, and a dumb one, how large the
// terminal is, whether the process is in its foreground, whether standard
// output goes into a pipe, whether the locale shows UTF-8, and which Theme
// to draw in. IsPipe, TerminalSize and InForeground ask the system, and
// UTF8Locale reads the getenv it is given, for the program to call. Nor
// does the package set anything process-wide: a program that hands its
// children no trace context takes TRACEPARENT out of its environment in
// Options.Trace, which Start calls only once it makes a Bus, and the
// interrupt is the end of the command's context, not a signal the package
// catches.
//
// A program calls Start as a command begins and runs the command under
// Run.Context, in a span of its own. Start writes its notes, picks the
// display, opens the event log and makes the Bus; it makes none when
// nothing is shown and no log is written, so that standard error then
// holds what it would without the kit, byte for byte. While a display is
// drawn, the program puts its own writers of standard error through
// Run.Writer, standard output too where it shows on the same terminal, and
// those of goroutines beside the command, such as a logger's, through
// Run.Lines. Once the command's span has ended, Run.Finish closes the Bus,
// takes the display off the terminal, closes the event log and writes the
// summary; the program then puts its writers back and writes the command's
// error.
//
// The event log is appended to a file this user alone may read, created
// with mode 0600. A regular file that is there already has to be one this
// user owns, under no other name, that nobody else can read or write, and a
// symbolic link on the way to it, a directory's as well as its own, or a
// named pipe, has to be this user's or root's; a terminal or another device
// is opened as it is. /dev/stdout, /dev/stderr, /dev/fd/N and
// /proc/self/fd/N are written through a duplicate of that descriptor of the
// process's. Where files have no Unix owner, as on Windows, none of this is
// checked.
package cliprogress

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
)

// Mode is a way to show the progress of a command, named by one of the
// words a flag such as --progress takes.
type Mode uint8

const (
	// ModeAuto is the live tree where it can be drawn, and nothing
	// elsewhere. Choose never returns it.
	ModeAuto Mode = iota + 1
	// ModeTTY is the live tree, display.Tree: a few rows at the bottom of
	// the terminal, redrawn as the work goes on, or the counter's line on
	// a terminal too small for them.
	ModeTTY
	// ModeCounter is one line at the bottom of the terminal,
	// display.Counter, redrawn as the work goes on.
	ModeCounter
	// ModePlain is a line on standard error for each thing worth one,
	// display.Plain, for a log as much as a terminal.
	ModePlain
	// ModeNone shows nothing: standard error carries what it would
	// without progress, byte for byte.
	ModeNone
)

// modeWords are the words of the modes, in the order Modes lists them.
var modeWords = [...]string{ModeAuto: "auto", ModeTTY: "tty", ModeCounter: "counter", ModePlain: "plain", ModeNone: "none"}

// Modes returns every Mode, in a new slice: auto, tty, counter, plain and
// none, each named by its String, for a flag's help and its completion.
func Modes() []Mode {
	return []Mode{ModeAuto, ModeTTY, ModeCounter, ModePlain, ModeNone}
}

// String returns the word for m, such as "tty", or "mode(n)" for a value
// this package does not define.
func (m Mode) String() string {
	if m == 0 || int(m) >= len(modeWords) {
		return fmt.Sprintf("mode(%d)", uint8(m))
	}
	return modeWords[m]
}

// modeOf returns the Mode word names, as String writes it; 0 for none.
func modeOf(word string) Mode {
	for _, m := range Modes() {
		if m.String() == word {
			return m
		}
	}
	return 0
}

// Setting is how the command line and the environment set a setting of
// the program's: by a flag, or, when the command line does not give it, by
// a variable that stands in for it. A program without the variable leaves
// Variable and Env empty.
type Setting struct {
	// Flag names the flag as the command line gives it, such as
	// "--progress", in the errors that quote it.
	Flag string
	// Given says the command line gave the flag, with an empty value or
	// not, and Value is what it gave.
	Given bool
	Value string
	// Variable names the variable, such as "SIND_PROGRESS", in the notes
	// that quote it, and Env is its value, as the program read it: empty
	// for a variable that is not set. Neither is read when Given is set.
	Variable string
	Env      string
}

// value returns what s says, where it said it, and whether that was the
// variable, which fails no command.
func (s Setting) value() (value, from string, ambient bool) {
	if s.Given {
		return s.Value, s.Flag, false
	}
	return s.Env, s.Variable, true
}

// Options configure Choose and Start. The zero Options shows nothing and
// writes no log.
type Options struct {
	// Program names the program in front of the notes and the summary
	// that Start and Finish write, "prog: …", so that none is read as the
	// command's own, and on the Bus and the Terminal, and so on the event
	// log; empty leaves the name out.
	Program string
	// Mode is how the command line and the environment ask for the
	// progress to be shown, in the words Modes lists; with neither asking,
	// it is auto.
	Mode Setting
	// Log is the file the event log is appended to, as the command line
	// and the environment name it; with neither naming one, or with an
	// empty name, no log is written.
	Log Setting
	// LogOptions configure the event log, as progress.NewLog has them: the
	// program's version, above all. An empty LogOptions.Program takes
	// Program, through the Bus.
	LogOptions progress.LogOptions

	// Stderr is the command's standard error: the display draws on it,
	// and the summary is written to it; nil is os.Stderr.
	Stderr io.Writer
	// Notes receives the notes Start writes, and the line Finish writes
	// when the event log stops short: the program's diagnostics; nil is
	// Stderr.
	Notes io.Writer
	// OnTerminal says Stderr is a terminal, and Dumb that it is one that
	// cannot draw the tree or the counter, as TERM=dumb says.
	OnTerminal bool
	Dumb       bool
	// IntoPipe says standard output goes into a pipe or a socket, as
	// IsPipe tells, whose reader, such as grep or less, may write to the
	// terminal the tree would be drawn on, where nothing keeps the two
	// apart: auto draws nothing then.
	IntoPipe bool
	// Size and Foreground are the terminal's, as display.TerminalOptions
	// has them, and are asked at each frame: TerminalSize and InForeground
	// of Stderr, say.
	Size       func() (cols, rows int, err error)
	Foreground func() bool
	// ASCII draws the displays and the summary with ASCII marks alone,
	// for a locale that is not UTF-8, as UTF8Locale tells.
	ASCII bool
	// Theme is how the displays and the summary look; the zero Theme
	// draws them in no colour. Whether to draw in colour, and in how many
	// colours, is the program's to decide: where Stderr is no terminal, or
	// a dumb one, plain lines go to a log, which wants the zero Theme.
	Theme display.Theme
	// Noun says how many targets a step expects in a plain line, as
	// display.PlainOptions has it; nil is "1 target" and "%d targets".
	Noun func(n int) string

	// Now is the clock of the Bus and of the display; nil is time.Now.
	Now func() time.Time
	// Trace returns the trace another program handed on, as
	// progress.ParseTraceContext reads it, and is called only when Start
	// makes a Bus: a program that hands its children no trace context
	// takes TRACEPARENT and TRACESTATE out of its environment there. Nil,
	// or a zero trace, draws one at random.
	Trace func() progress.TraceContext
	// Classify is the Bus's fallback for the class of an error, as
	// progress.BusOptions has it, such as the program's rule for its exit
	// codes.
	Classify func(error) progress.Class
	// PanicLog receives the stack of a sink or a display that panicked,
	// through the Terminal's Lines while a display is drawn; nil is
	// Stderr.
	PanicLog io.Writer
	// Manual makes the display without starting its drawing, so that it
	// draws a frame only when Run.Draw is called, as a test does on a
	// clock it moves.
	Manual bool
}

// prefix returns what the notes start with: the program's name and a
// colon, or nothing for a program with no name.
func (o Options) prefix() string {
	if o.Program == "" {
		return ""
	}
	return o.Program + ": "
}

// Choose returns the Mode o.Mode asks for, as the terminal o describes can
// show it, with a note for standard error, or an error that refuses the
// command. It writes nothing, and never returns ModeAuto.
//
// The flag's word comes before the variable's, and auto before neither.
// auto is the live tree on a standard error that is a terminal, and not a
// dumb one, while standard output goes into no pipe, and none elsewhere:
// in a pipe, a file, a CI log or an agent's tool output, standard error is
// left as it would be without progress. tty and counter are drawn wherever
// they can be, standard output in a pipe or not; where they cannot be, the
// flag's are refused, so that the one who asked finds out why nothing
// shows, and the variable's show nothing. plain and none are as asked. A
// word that is none of these is refused from the flag; from the variable
// it shows nothing, and the note says so.
//
// A program calls Choose before any command runs, to refuse a flag no
// command could have, as Start would; Start calls it too.
func Choose(o Options) (Mode, string, error) {
	word, from, ambient := o.Mode.value()
	if ambient && word == "" {
		word = ModeAuto.String()
	}
	unfit := ""
	switch {
	case !o.OnTerminal:
		unfit = "standard error is not a terminal"
	case o.Dumb:
		unfit = "the terminal cannot draw one (TERM is dumb)"
	}
	switch mode := modeOf(word); mode {
	case ModeAuto:
		if unfit != "" || o.IntoPipe {
			return ModeNone, "", nil
		}
		return ModeTTY, "", nil
	case ModeTTY, ModeCounter:
		if unfit == "" {
			return mode, "", nil
		}
		if ambient {
			return ModeNone, "", nil
		}
		what := "a live tree"
		if mode == ModeCounter {
			what = "a counter"
		}
		return ModeNone, "", fmt.Errorf("%s asks for %s, but %s; use none, or auto to draw one only where it can be",
			from, what, unfit)
	case ModePlain, ModeNone:
		return mode, "", nil
	}
	words := make([]string, 0, len(modeWords))
	for _, m := range Modes() {
		words = append(words, m.String())
	}
	unknown := fmt.Sprintf("%s is %q; it takes one of %s", from, word, strings.Join(words, ", "))
	if ambient {
		return ModeNone, o.prefix() + unknown + "; no progress is shown", nil
	}
	return ModeNone, "", errors.New(unknown)
}

// UTF8Locale reports whether the locale getenv names writes text as UTF-8,
// which the marks of the displays need: LC_ALL, else LC_CTYPE, else LANG,
// the way the C library reads them. With none of them set the locale is C,
// which does not. The program passes os.Getenv; the package reads no
// environment of its own.
func UTF8Locale(getenv func(string) string) bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if locale := strings.ToLower(getenv(name)); locale != "" {
			return strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8")
		}
	}
	return false
}
