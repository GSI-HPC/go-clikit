// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
)

// logged returns what the file at path holds, "" for none.
func logged(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// The default log is written where neither the flag nor the variable names
// a log, and only there: a flag given empty, or a variable that names
// another file, wins.
func TestTheDefaultLogIsWrittenWhereNoneIsNamed(t *testing.T) {
	dir := t.TempDir()
	def, named := filepath.Join(dir, "activity.jsonl"), filepath.Join(dir, "named.jsonl")
	for _, tc := range []struct {
		name      string
		log       cliprogress.Setting
		def, file bool
	}{
		{"none named", cliprogress.Setting{Flag: "--progress-log", Variable: "PROG_PROGRESS_LOG"}, true, false},
		{"the flag given empty", cliprogress.Setting{Flag: "--progress-log", Given: true, Variable: "PROG_PROGRESS_LOG", Env: named}, false, false},
		{"the flag", cliprogress.Setting{Flag: "--progress-log", Given: true, Value: named}, false, true},
		{"the variable", cliprogress.Setting{Flag: "--progress-log", Variable: "PROG_PROGRESS_LOG", Env: named}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(def)
			_ = os.Remove(named)
			var stderr bytes.Buffer
			c := newClock()
			r, err := cliprogress.Start(context.Background(), cliprogress.Options{
				Program: "prog", Stderr: &stderr, Log: tc.log, DefaultLog: def, Now: c.now,
			})
			if err != nil {
				t.Fatal(err)
			}
			command(r, c, nil)
			r.Finish(true)
			if got := strings.Contains(logged(t, def), `"delete cluster"`); got != tc.def {
				t.Errorf("the default log records the command: %v, want %v", got, tc.def)
			}
			if got := strings.Contains(logged(t, named), `"delete cluster"`); got != tc.file {
				t.Errorf("the named log records the command: %v, want %v", got, tc.file)
			}
			if stderr.Len() != 0 {
				t.Errorf("standard error holds %q", stderr.String())
			}
		})
	}
}

// A default log that cannot be opened is left out without a note, and a
// command with nothing else to show then has no Bus.
func TestADefaultLogThatCannotBeUsedSaysNothing(t *testing.T) {
	var stderr bytes.Buffer
	r, err := cliprogress.Start(context.Background(), cliprogress.Options{
		Program:    "prog",
		Stderr:     &stderr,
		DefaultLog: filepath.Join(t.TempDir(), "missing", "activity.jsonl"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Context() != context.Background() || stderr.Len() != 0 {
		t.Errorf("Start made a Bus, or wrote %q", stderr.String())
	}
	r.Finish(true)
}

// The default log is moved aside once it is larger than RotateAt, and kept
// where it is while it is not, or when RotateAt is 0.
func TestTheDefaultLogIsRotated(t *testing.T) {
	for _, tc := range []struct {
		name     string
		size     int64
		rotateAt int64
		moved    bool
	}{
		{"larger", 101, 100, true},
		{"as large", 100, 100, false},
		{"never", 101, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			def := filepath.Join(dir, "activity.jsonl")
			old := strings.Repeat("x", int(tc.size))
			if err := os.WriteFile(def, []byte(old), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(def+".1", []byte("older"), 0o600); err != nil {
				t.Fatal(err)
			}
			c := newClock()
			r, err := cliprogress.Start(context.Background(), cliprogress.Options{DefaultLog: def, RotateAt: tc.rotateAt, Now: c.now})
			if err != nil {
				t.Fatal(err)
			}
			command(r, c, nil)
			r.Finish(false)
			now, aside := logged(t, def), logged(t, def+".1")
			if moved := aside == old && !strings.HasPrefix(now, "x"); moved != tc.moved {
				t.Errorf("moved %v, want %v: the log starts %.10q, the copy %.10q", moved, tc.moved, now, aside)
			}
			if !strings.Contains(now, `"delete cluster"`) {
				t.Errorf("the log %.40q does not record the command", now)
			}
		})
	}
}

// A log the flag or the variable names is the user's, and never moved.
func TestANamedLogIsNotRotated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "named.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 200)), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := cliprogress.Start(context.Background(), cliprogress.Options{
		Log:        cliprogress.Setting{Flag: "--progress-log", Given: true, Value: path},
		DefaultLog: path, RotateAt: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	r.Finish(false)
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Errorf("the named log was moved: %v", err)
	}
}
