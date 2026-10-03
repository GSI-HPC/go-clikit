// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"

	"github.com/GSI-HPC/go-clikit/progress"
)

// panicLogMu keeps the stacks of panics in workers running side by side
// from interleaving.
var panicLogMu sync.Mutex

// Recovered turns a panic in the work for one target into that target's
// error, and writes the stack to log, the front end's diagnostics, or the
// process's standard error when log is nil. It is called as
// Recovered(log, program, name, recover()) in a function deferred by the
// goroutine doing the work, and returns a *PanicError that holds program,
// target and v, or nil when nothing panicked. The target is named by text
// from elsewhere, so the line shows it through progress.Sanitize, its
// control characters as escapes. program names the program, in front of
// the line in the log, as "prog: ", and in the error, which asks for the
// bug to be reported; empty leaves the name out of the line and calls it
// "the program" in the error.
//
// A library that reports to a Bus calls it as
// Recovered(b.PanicLog(), b.Program(), name, recover()), with
// b := progress.BusFrom(ctx), so that it names the program and writes the
// stack where the Bus does.
//
// recover only stops a panic in its own goroutine, so each worker of a
// fan-out needs its own; without it, a panic on one target ends the
// process, and with it every other target's work.
func Recovered(log io.Writer, program, target string, v any) error {
	if v == nil {
		return nil
	}
	if log == nil {
		log = os.Stderr
	}
	prefix := ""
	if program != "" {
		prefix = program + ": "
	}
	text := fmt.Sprint(v)
	panicLogMu.Lock()
	// The log is a courtesy; a write that fails changes nothing.
	_, _ = fmt.Fprintf(log, "%spanic while working on %s: %q\n%s", prefix, progress.Sanitize(target, 0), text, debug.Stack())
	panicLogMu.Unlock()
	return &PanicError{Program: program, Target: target, Value: v, text: text, fixed: true}
}

// PanicError is the error a panic in the work for an item became, which
// tells it from an error the work returned.
//
// It wraps nothing, even when the panic was called with an error: a panic
// is a bug, which fails its item whatever it was called with, so errors.Is
// and errors.As do not find the context's error, a skip or a Classifier in
// it, which would end the item canceled, skipped or of another class.
// Value holds what the panic was called with, for a program to look at.
// The error's text is fixed when Recovered recovers the panic, as the log
// line shows it, so a value the program goes on changing changes neither
// the text nor is read again, from another goroutine, by Error.
type PanicError struct {
	// Program is the program that panicked, as Recovered was told it.
	Program string
	// Target names the target whose work panicked, as Recovered was told
	// it, unescaped; empty when it was told none.
	Target string
	// Value is what the panic was called with, as recover returned it.
	Value any

	// text is Value as fmt.Sprint printed it when the panic was recovered,
	// and fixed says it was, since a value can print as "".
	text  string
	fixed bool
}

// Error says that the program panicked, that this is a bug to report, and
// what the panic was called with, as fmt.Sprint printed it when the panic
// was recovered, quoted. A PanicError that Recovered did not make prints
// Value as it is when Error is called.
func (e *PanicError) Error() string {
	text := e.text
	if !e.fixed {
		text = fmt.Sprint(e.Value)
	}
	return fmt.Sprintf("%s panicked; this is a bug, please report it: %q", cmp.Or(e.Program, "the program"), text)
}
