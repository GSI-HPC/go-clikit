// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/termtext"
)

// Run is the progress of one command: the Bus Start made for it, the
// display that shows it and the event log that records it, or none of
// them. Its methods are not safe for concurrent use, but for the writers
// they return, which are.
type Run struct {
	ctx      context.Context
	mode     Mode
	prefix   string
	stderr   io.Writer
	notes    io.Writer
	bus      *progress.Bus
	term     *display.Terminal
	shown    shown
	summary  *display.Summary
	log      *progress.Log
	logFile  *os.File
	finished bool
}

// shown is a display: the Tree, the Counter or Plain.
type shown interface {
	progress.Sink
	Start()
	Draw()
	Close()
}

// Start returns the Run of a command whose context is ctx, as o asks for:
// a display when o.Mode has one drawn, an event log when o.Log names one,
// and the Bus they are the sinks of. It writes the notes Choose and the
// event log have, escaped, to o.Notes first.
//
// A Run with neither has no Bus: its Context is ctx, its writers are the
// streams they are given, and Finish does nothing. So is the Run of a ctx
// that carries a Bus already, such as a test's or a server's for one call,
// whose Bus is its maker's: Start then reads o not at all, writes nothing
// and refuses nothing.
//
// An error is the command line's, as Choose's is: a word the flag gives
// that names no Mode, a display the terminal cannot show, or a log the
// flag names that cannot be used, which the error wraps. Nothing is
// written or created then. What the variables ask for fails no command.
//
// The display is drawn on o.Stderr, and the tree stops drawing what runs,
// and says the command was interrupted, once ctx is done.
func Start(ctx context.Context, o Options) (*Run, error) {
	r := &Run{ctx: ctx, mode: ModeNone}
	if progress.BusFrom(ctx) != nil {
		return r, nil
	}
	mode, modeNote, err := Choose(o)
	if err != nil {
		return nil, err
	}
	logFile, logNote, err := openLog(o)
	if err != nil {
		return nil, err
	}
	stderr := o.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	notes := o.Notes
	if notes == nil {
		notes = stderr
	}
	for _, note := range []string{modeNote, logNote} {
		if note != "" {
			// A note that cannot be written changes nothing about the
			// command.
			_, _ = fmt.Fprintln(notes, termtext.Escape(note))
		}
	}
	if mode == ModeNone && logFile == nil {
		return r, nil
	}

	r.prefix, r.stderr, r.notes, r.logFile = o.prefix(), stderr, notes, logFile
	panicLog := o.PanicLog
	if panicLog == nil {
		panicLog = stderr
	}
	var sinks []progress.Sink
	if mode != ModeNone {
		r.mode = mode
		r.term = display.NewTerminal(stderr, display.TerminalOptions{
			Size:       o.Size,
			Foreground: o.Foreground,
			PanicLog:   o.PanicLog,
			Program:    o.Program,
		})
		r.shown = newDisplay(mode, r.term, o, ctx.Done())
		if !o.Manual {
			r.shown.Start()
		}
		r.summary = &display.Summary{Theme: o.Theme, ASCII: o.ASCII}
		sinks = append(sinks, r.shown, r.summary)
		panicLog = r.term.Lines(panicLog)
	}
	if logFile != nil {
		r.log = progress.NewLog(r.logWriter(), o.LogOptions)
		sinks = append(sinks, r.log)
	}
	var trace progress.TraceContext
	if o.Trace != nil {
		trace = o.Trace()
	}
	r.bus = progress.NewBus(progress.BusOptions{
		Sinks:    sinks,
		Now:      o.Now,
		Trace:    trace,
		PanicLog: panicLog,
		Program:  o.Program,
		Classify: o.Classify,
	})
	r.ctx = progress.WithBus(ctx, r.bus)
	return r, nil
}

// newDisplay makes the display of mode on term, which says the command was
// interrupted once interrupted is closed.
func newDisplay(mode Mode, term *display.Terminal, o Options, interrupted <-chan struct{}) shown {
	switch mode {
	case ModeTTY:
		return display.NewTree(term, display.TreeOptions{Now: o.Now, ASCII: o.ASCII, Theme: o.Theme, Interrupted: interrupted})
	case ModeCounter:
		return display.NewCounter(term, display.CounterOptions{Now: o.Now, ASCII: o.ASCII, Theme: o.Theme})
	}
	return display.NewPlain(term, display.PlainOptions{Now: o.Now, ASCII: o.ASCII, Theme: o.Theme, Noun: o.Noun})
}

// openLog opens the event log o.Log names, to append to; nil when it
// names none. A log the flag names that cannot be used is an error; the
// variable's fails no command, and the note returned says no log is
// written.
func openLog(o Options) (f *os.File, note string, err error) {
	path, from, ambient := o.Log.value()
	if path == "" {
		return nil, "", nil
	}
	f, err = appendPrivate(path, os.Geteuid())
	switch {
	case err == nil:
		return f, "", nil
	case ambient:
		return nil, fmt.Sprintf("%s%s names a progress log that cannot be used: %v; no log is written", o.prefix(), from, err), nil
	}
	return nil, "", fmt.Errorf("%s names a progress log that cannot be used: %w", from, err)
}

// logWriter returns what the event log is written through: its file,
// unless that is the file of standard error, on which the display draws,
// as --progress-log /dev/stderr names it. The log writes from a goroutine
// of its own, so its lines then go through the Terminal's Lines, above the
// display, and never into it.
func (r *Run) logWriter() io.Writer {
	if r.term != nil && sameFile(r.logFile, r.stderr) {
		return r.term.Lines(r.stderr)
	}
	return r.logFile
}

// sameFile reports whether w is a file open on the file f is open on, as a
// duplicate of standard error is.
func sameFile(f *os.File, w io.Writer) bool {
	g, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, ferr := f.Stat()
	gi, gerr := g.Stat()
	return ferr == nil && gerr == nil && os.SameFile(fi, gi)
}

// Context returns the context the command runs under: the one Start was
// given, with the Bus.
func (r *Run) Context() context.Context { return r.ctx }

// Mode returns how the progress is shown: ModeTTY, ModeCounter or
// ModePlain, or ModeNone without a display.
func (r *Run) Mode() Mode { return r.mode }

// Writer returns a writer to w, a stream that shows on the terminal the
// display draws on, such as standard error, or standard output when it is
// the same terminal, that takes the display off before it writes, as
// display.Terminal.Writer does; w itself without a display.
func (r *Run) Writer(w io.Writer) io.Writer {
	if r.term == nil {
		return w
	}
	return r.term.Writer(w)
}

// Lines returns a writer to w for the lines of goroutines beside the
// command, such as a logger's, which holds them while a question is asked,
// as display.Terminal.Lines does; w itself without a display.
func (r *Run) Lines(w io.Writer) io.Writer {
	if r.term == nil {
		return w
	}
	return r.term.Lines(w)
}

// Draw draws a frame of the display, for a test that set Options.Manual
// and moves its clock by hand; without a display it does nothing.
func (r *Run) Draw() {
	if r.shown != nil {
		r.shown.Draw()
	}
}

// Finish takes down what Start made, once the command's span has ended:
// it closes the Bus, takes the display off the terminal, writes out the
// event log and closes its file, and then, when summary is set, writes the
// summary to standard error, "prog: delete cluster: failed in 3.0s", for a
// command a display showed for a second or longer. A program that leaves
// the summary only after a command that failed passes whether it did. A log
// that could not be written fails nothing: a note says once that it stops
// short.
//
// The writers the Run returned pass their bytes through from then on. The
// program puts its own writers back, and then writes the command's error.
// A second Finish does nothing.
func (r *Run) Finish(summary bool) {
	if r.bus == nil || r.finished {
		return
	}
	r.finished = true
	r.bus.Close()
	if r.shown != nil {
		r.shown.Close()
	}
	if r.log != nil {
		err := r.log.Close()
		if closeErr := r.logFile.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			// The log is a record of the work, not part of it: a log
			// that cannot be written fails nothing, and is reported once.
			_, _ = fmt.Fprintln(r.notes, termtext.Escape(fmt.Sprintf(
				"%sthe progress log %s stops short: %v", r.prefix, r.logFile.Name(), err)))
		}
	}
	if summary && r.summary != nil {
		if line := r.summary.Line(); line != "" {
			// The summary is a courtesy; a line that cannot be written
			// changes nothing about the command. It is not escaped: a
			// Theme draws it in colour, and the Bus sanitised the names
			// in it as their spans began.
			_, _ = fmt.Fprintln(r.stderr, r.prefix+line)
		}
	}
}
