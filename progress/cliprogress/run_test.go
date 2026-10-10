// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// clock is a clock a test moves by hand.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)} }

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// terminal returns Options for a terminal of 80 columns and 24 rows that
// screen is, on a clock c, drawn by hand.
func terminal(screen *progresstest.Screen, c *clock) cliprogress.Options {
	return cliprogress.Options{
		Program:    "prog",
		Mode:       cliprogress.Setting{Flag: "--progress", Variable: "PROG_PROGRESS"},
		Log:        cliprogress.Setting{Flag: "--progress-log", Variable: "PROG_PROGRESS_LOG"},
		Stderr:     screen,
		OnTerminal: true,
		Size:       func() (int, int, error) { return 80, 24, nil },
		Foreground: func() bool { return true },
		Now:        c.now,
		Manual:     true,
	}
}

// command runs a command of two seconds under r, with a step that fails
// after one, and ends it with err.
func command(r *cliprogress.Run, c *clock, err error) {
	ctx, cmd := progress.Start(r.Context(), progress.KindCommand, "delete cluster")
	_, step := progress.Start(ctx, progress.KindStep, "mesh")
	c.add(time.Second)
	r.Draw()
	step.End(err)
	c.add(time.Second)
	cmd.End(err)
}

// Without a display or a log, a command runs with no Bus, and standard
// error holds what it would without the kit, byte for byte: the writers
// are the streams themselves, and Finish writes nothing.
func TestNothingShownIsNoBus(t *testing.T) {
	var stderr bytes.Buffer
	ctx := context.Background()
	asked := false
	r, err := cliprogress.Start(ctx, cliprogress.Options{
		Stderr: &stderr,
		Trace:  func() progress.TraceContext { asked = true; return progress.TraceContext{} },
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if r.Context() != ctx || r.Mode() != cliprogress.ModeNone || progress.BusFrom(r.Context()) != nil {
		t.Errorf("Start made a Bus or a display: mode %v", r.Mode())
	}
	if r.Writer(&stderr) != &stderr || r.Lines(&stderr) != &stderr {
		t.Error("a writer stands between the command and its streams without a display")
	}
	r.Draw()
	r.Finish(true)
	if stderr.Len() != 0 || asked {
		t.Errorf("standard error holds %q; the trace was asked for: %v", stderr.String(), asked)
	}
}

// A Bus the context carries is its maker's, such as a test's or a server's
// for one call: Start makes nothing, refuses nothing and writes nothing.
func TestABusInTheContextIsLeftToItsMaker(t *testing.T) {
	var stderr bytes.Buffer
	bus := progress.NewBus(progress.BusOptions{})
	defer bus.Close()
	ctx := progress.WithBus(context.Background(), bus)
	r, err := cliprogress.Start(ctx, cliprogress.Options{
		Stderr: &stderr,
		Mode:   cliprogress.Setting{Flag: "--progress", Given: true, Value: "bogus"},
	})
	if err != nil || r.Context() != ctx || r.Mode() != cliprogress.ModeNone || stderr.Len() != 0 {
		t.Errorf("Start = %v, %v; mode %v; standard error %q", r, err, r.Mode(), stderr.String())
	}
	r.Finish(true)
}

// What the flags ask for and cannot have refuses the command before
// anything is shown or written; a log the flag names is not created when
// the display is refused.
func TestTheFlagsAreRefusedBeforeAnythingIsWritten(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "progress.jsonl")
	tests := []struct {
		name string
		mode cliprogress.Setting
		log  string
		want string
		is   error
	}{
		{"a tree in a pipe", cliprogress.Setting{Flag: "--progress", Given: true, Value: "tty"}, log,
			"--progress asks for a live tree, but standard error is not a terminal; use none, or auto to draw one only where it can be", nil},
		{"a log in no directory", cliprogress.Setting{}, filepath.Join(dir, "missing", "progress.jsonl"),
			"--progress-log names a progress log that cannot be used: ", fs.ErrNotExist},
	}
	for _, tc := range tests {
		var stderr bytes.Buffer
		r, err := cliprogress.Start(context.Background(), cliprogress.Options{
			Stderr: &stderr,
			Mode:   tc.mode,
			Log:    cliprogress.Setting{Flag: "--progress-log", Given: true, Value: tc.log},
		})
		if r != nil || err == nil || !strings.HasPrefix(err.Error(), tc.want) || tc.is != nil && !errors.Is(err, tc.is) {
			t.Errorf("%s: Start = %v, %v; want the error %q", tc.name, r, err, tc.want)
		}
		if _, err := os.Stat(log); !errors.Is(err, fs.ErrNotExist) || stderr.Len() != 0 {
			t.Errorf("%s: the log was created (%v), or standard error holds %q", tc.name, err, stderr.String())
		}
	}
}

// What the variables ask for and cannot have fails no command: each says
// so in a line of its own, escaped, since a variable can hold anything.
func TestTheVariablesFailNoCommand(t *testing.T) {
	var stderr bytes.Buffer
	dir := t.TempDir()
	r, err := cliprogress.Start(context.Background(), cliprogress.Options{
		Program: "prog",
		Stderr:  &stderr,
		Mode:    cliprogress.Setting{Flag: "--progress", Variable: "PROG_PROGRESS", Env: "\x1b[2Jtty"},
		Log:     cliprogress.Setting{Flag: "--progress-log", Variable: "PROG_PROGRESS_LOG", Env: filepath.Join(dir, "missing", "log")},
	})
	if err != nil || r.Mode() != cliprogress.ModeNone || progress.BusFrom(r.Context()) != nil {
		t.Fatalf("Start = %v, %v", r, err)
	}
	want := `prog: PROG_PROGRESS is "\x1b[2Jtty"; it takes one of auto, tty, counter, plain, none; no progress is shown` + "\n" +
		"prog: PROG_PROGRESS_LOG names a progress log that cannot be used: open " + filepath.Join(dir, "missing", "log") +
		": no such file or directory; no log is written\n"
	if got := stderr.String(); got != want {
		t.Errorf("standard error holds\n%q\nwant\n%q", got, want)
	}
}

// The tree draws on the terminal and comes off before the command's own
// lines, which go through Writer, and before the lines from beside it,
// which go through Lines; it is taken off by Finish, which writes the
// summary when it is asked for.
func TestTheTreeComesOffForTheCommandsLines(t *testing.T) {
	for _, summary := range []bool{false, true} {
		screen := &progresstest.Screen{Width: 80}
		c := newClock()
		r, err := cliprogress.Start(context.Background(), terminal(screen, c))
		if err != nil || r.Mode() != cliprogress.ModeTTY {
			t.Fatalf("Start = %v, %v", r, err)
		}
		ctx, cmd := progress.Start(r.Context(), progress.KindCommand, "delete cluster")
		_, step := progress.Start(ctx, progress.KindStep, "mesh")
		c.add(time.Second)
		r.Draw()
		if got := screen.String(); !strings.Contains(got, "mesh") {
			t.Errorf("the tree shows\n%s", got)
		}
		fmt.Fprintln(r.Writer(screen), "a note")
		fmt.Fprintln(r.Lines(screen), "a log line")
		fmt.Fprintln(r.Writer(screen), "standard output on the same terminal")
		step.End(errors.New("boom"))
		c.add(time.Second)
		cmd.End(errors.New("boom"))
		r.Finish(summary)
		r.Finish(summary)
		want := "a note\na log line\nstandard output on the same terminal\n✗ mesh  1.0s\n"
		if summary {
			want += "prog: delete cluster: failed in 2.0s\n"
		}
		if got := screen.String(); got != want {
			t.Errorf("summary %v: the terminal shows\n%s\nwant\n%s", summary, got, want)
		}
	}
}

// The counter the flag asks for is drawn on a terminal, and comes off it
// once the command has ended, leaving nothing but the summary.
func TestTheCounterIsDrawnWhereTheFlagAsksForIt(t *testing.T) {
	screen := &progresstest.Screen{Width: 80}
	c := newClock()
	o := terminal(screen, c)
	o.Mode.Given, o.Mode.Value = true, "counter"
	r, err := cliprogress.Start(context.Background(), o)
	if err != nil || r.Mode() != cliprogress.ModeCounter {
		t.Fatalf("Start = %v, %v", r, err)
	}
	ctx, cmd := progress.Start(r.Context(), progress.KindCommand, "power cut")
	_, step := progress.Start(ctx, progress.KindStep, "nodes", progress.Total(2))
	c.add(time.Second)
	r.Draw()
	if got := screen.String(); !strings.Contains(got, "nodes") || !strings.Contains(got, "0:01.0") {
		t.Errorf("the counter shows\n%s", got)
	}
	step.End(nil)
	cmd.End(nil)
	r.Finish(true)
	if got, want := screen.String(), "prog: power cut: done in 1.0s\n"; got != want {
		t.Errorf("the terminal shows\n%q\nwant\n%q", got, want)
	}
}

// Notes, and the line that says the event log stops short, go to the
// program's diagnostics when it names them; the display and the summary
// stay on standard error.
func TestNotesGoToTheProgramsDiagnostics(t *testing.T) {
	screen := &progresstest.Screen{Width: 80}
	var notes bytes.Buffer
	c := newClock()
	o := terminal(screen, c)
	o.Notes = &notes
	o.Mode.Given, o.Mode.Value = true, "plain"
	o.Log.Env = filepath.Join(t.TempDir(), "missing", "log")
	r, err := cliprogress.Start(context.Background(), o)
	if err != nil || r.Mode() != cliprogress.ModePlain {
		t.Fatalf("Start = %v, %v", r, err)
	}
	command(r, c, errors.New("boom"))
	r.Finish(true)
	if got := notes.String(); !strings.HasPrefix(got, "prog: PROG_PROGRESS_LOG names a progress log that cannot be used: ") ||
		strings.Count(got, "\n") != 1 {
		t.Errorf("the diagnostics hold %q", got)
	}
	if got := screen.String(); !strings.Contains(got, "mesh: failed") || !strings.HasSuffix(got, "prog: delete cluster: failed in 2.0s\n") {
		t.Errorf("the terminal shows\n%s", got)
	}
}

// The event log records the command whatever is shown, none included, with
// the program's name and version on its first line; a summary needs a
// display, and a log alone writes nothing on standard error.
func TestTheEventLogNeedsNoDisplay(t *testing.T) {
	var stderr bytes.Buffer
	c := newClock()
	path := filepath.Join(t.TempDir(), "progress.jsonl")
	r, err := cliprogress.Start(context.Background(), cliprogress.Options{
		Program:    "prog",
		LogOptions: progress.LogOptions{Version: "1.2.3"},
		Stderr:     &stderr,
		Log:        cliprogress.Setting{Flag: "--progress-log", Variable: "PROG_PROGRESS_LOG", Env: path},
		Now:        c.now,
		Trace: func() progress.TraceContext {
			tc, _ := progress.ParseTraceContext("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "")
			return tc
		},
	})
	if err != nil || r.Mode() != cliprogress.ModeNone {
		t.Fatalf("Start = %v, %v", r, err)
	}
	command(r, c, nil)
	r.Finish(true)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(string(data), "\n")
	for _, want := range []string{`"program":"prog"`, `"version":"1.2.3"`, `4bf92f3577b34da6a3ce929d0e0e4736`} {
		if !strings.Contains(first, want) {
			t.Errorf("the log's first line %s holds no %s", first, want)
		}
	}
	if !strings.Contains(string(data), `"delete cluster"`) || stderr.Len() != 0 {
		t.Errorf("the log holds\n%s\nand standard error %q", data, stderr.String())
	}
}

// A log on the file the display draws on, as --progress-log /dev/stderr
// names it, goes through the Terminal's Lines, above the display and never
// into it, since it is written from a goroutine of its own.
func TestALogOnTheDisplaysFileGoesAboveIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr")
	stderr, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	c := newClock()
	o := terminal(nil, c)
	o.Stderr = stderr
	o.Log.Given, o.Log.Value = true, path
	r, err := cliprogress.Start(context.Background(), o)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	command(r, c, errors.New("boom"))
	r.Finish(true)
	data, _ := os.ReadFile(path)
	screen := &progresstest.Screen{Width: 80}
	_, _ = screen.Write(data)
	if got := screen.String(); !strings.Contains(got, `"type":"trace"`) || !strings.Contains(got, "prog: delete cluster: failed in 2") {
		t.Errorf("the terminal shows\n%s", got)
	}
}

// The stack of a display or a sink that panicked goes to the PanicLog the
// program gives, standard error without one, through the Terminal's Lines
// while a display is drawn.
func TestAPanicGoesToThePanicLog(t *testing.T) {
	for _, own := range []bool{false, true} {
		screen := &progresstest.Screen{Width: 80}
		var panics bytes.Buffer
		o := terminal(screen, newClock())
		if own {
			o.PanicLog = &panics
		}
		r, err := cliprogress.Start(context.Background(), o)
		if err != nil {
			t.Fatal(err)
		}
		bus := progress.BusFrom(r.Context())
		if got := bus.Program(); got != "prog" {
			t.Errorf("the Bus names %q", got)
		}
		_, _ = fmt.Fprintln(bus.PanicLog(), "a stack")
		r.Finish(false)
		got := screen.String()
		if own {
			got = panics.String()
		}
		if !strings.Contains(got, "a stack") {
			t.Errorf("own panic log %v: the stack went elsewhere: %q", own, got)
		}
	}
}

// Standard error is the process's when the program names none. The test
// swaps os.Stderr, so it does not run in parallel.
func TestStandardErrorIsTheProcesssWithoutOne(t *testing.T) {
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = wr
	_, err = cliprogress.Start(context.Background(), cliprogress.Options{
		Program: "prog",
		Mode:    cliprogress.Setting{Variable: "PROG_PROGRESS", Env: "bogus"},
	})
	os.Stderr = saved
	_ = wr.Close()
	got := new(bytes.Buffer)
	_, _ = got.ReadFrom(rd)
	if err != nil || !strings.HasPrefix(got.String(), `prog: PROG_PROGRESS is "bogus"`) {
		t.Errorf("Start: %v; the process's standard error holds %q", err, got.String())
	}
}

// The tree says the command was interrupted once its context is done, and
// a display started by Start draws on its own, from a goroutine Finish
// stops, in a theme the program picked.
func TestTheTreeDrawsOnItsOwnAndSeesTheInterrupt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		screen := &progresstest.Screen{Width: 80}
		o := terminal(screen, newClock())
		o.Now, o.Manual, o.Theme = nil, false, display.Classic.In(display.NoColours)
		ctx, cancel := context.WithCancel(context.Background())
		r, err := cliprogress.Start(ctx, o)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cmd := progress.Start(r.Context(), progress.KindCommand, "delete cluster")
		_, step := progress.Start(ctx, progress.KindStep, "mesh")
		time.Sleep(1500 * time.Millisecond)
		cancel()
		time.Sleep(time.Second)
		synctest.Wait()
		shown := screen.String()
		step.End(context.Canceled)
		cmd.End(context.Canceled)
		r.Finish(true)
		if want := "delete cluster ⋅ interrupting ⋅ 0:02.5\n  mesh  2.5s\n"; shown != want {
			t.Errorf("the tree drew\n%s\nwant\n%s", shown, want)
		}
		if got, want := screen.String(), "⊘ mesh  2.5s\nprog: ⊘ delete cluster: canceled in 2.5s\n"; got != want {
			t.Errorf("the terminal shows\n%s\nwant\n%s", got, want)
		}
	})
}
