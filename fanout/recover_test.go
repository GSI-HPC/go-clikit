// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
)

// Without a log, the stack of a panic goes to the process's standard
// error. The test swaps os.Stderr, so it does not run in parallel.
func TestRecoveredWritesToStandardErrorWithoutALog(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	perr := fanout.Recovered(nil, "sind", "worker-1", "boom")
	os.Stderr = saved
	_ = w.Close()
	written, _ := io.ReadAll(r)
	if !strings.HasPrefix(string(written), `sind: panic while working on worker-1: "boom"`) {
		t.Errorf("standard error reads %q", written)
	}
	if want := `sind panicked; this is a bug, please report it: "boom"`; perr == nil || perr.Error() != want {
		t.Errorf("Recovered = %v, want %q", perr, want)
	}
	if fanout.Recovered(nil, "sind", "worker-1", nil) != nil {
		t.Error("Recovered returned an error when nothing panicked")
	}
}

// A target is named by text from elsewhere, so the line that names it shows
// its control characters as escapes rather than writing them to the
// terminal.
func TestRecoveredEscapesTheTarget(t *testing.T) {
	t.Parallel()

	var log strings.Builder
	_ = fanout.Recovered(&log, "prog", "evil\x1b]0;pwned\x07\x1b[2J\nnext", "boom")
	line, _, _ := strings.Cut(log.String(), "\n")
	if want := `prog: panic while working on evil\x1b]0;pwned\x07\x1b[2J\nnext: "boom"`; line != want {
		t.Errorf("the log's first line reads %q, want %q", line, want)
	}
}

// A PanicError keeps what the panic was called with and the target, but
// wraps nothing: a panic with the context's error, a skip or an error of
// a class of its own is a bug, which fails its item as any other panic.
func TestPanicErrorKeepsTheValueAndWrapsNothing(t *testing.T) {
	t.Parallel()

	for _, v := range []error{context.Canceled, progress.Skip("not now"), unreachable{errors.New("no route")}} {
		err := fanout.Recovered(io.Discard, "prog", "exe1", v)
		var p *fanout.PanicError
		if !errors.As(err, &p) || p.Value != any(v) || p.Target != "exe1" || p.Program != "prog" {
			t.Errorf("Recovered(%v) = %#v, want a PanicError that keeps it", v, err)
		}
		if errors.Is(err, v) || errors.Is(err, progress.ErrSkipped) {
			t.Errorf("errors.Is finds %v in the panic's error", v)
		}
		if got := progress.Classify(err, nil); got != progress.ClassTarget {
			t.Errorf("a panic with %v is classed %s, want target", v, got)
		}
		if want := `prog panicked; this is a bug, please report it: "` + v.Error() + `"`; err.Error() != want {
			t.Errorf("Error() = %q, want %q", err, want)
		}
	}
}

// The text of a PanicError is fixed when the panic is recovered: a value
// the program changes afterwards changes neither the error nor the log.
// A PanicError made by hand prints its Value.
func TestPanicErrorFixesItsTextWhenRecovered(t *testing.T) {
	t.Parallel()

	state := map[string]int{"done": 1}
	var log strings.Builder
	err := fanout.Recovered(&log, "prog", "exe1", state)
	state["done"] = 2
	if want := `prog panicked; this is a bug, please report it: "map[done:1]"`; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err, want)
	}
	if !strings.Contains(log.String(), `"map[done:1]"`) {
		t.Errorf("the log reads %q", log.String())
	}
	byHand := &fanout.PanicError{Value: "boom"}
	if want := `the program panicked; this is a bug, please report it: "boom"`; byHand.Error() != want {
		t.Errorf("Error() = %q, want %q", byHand, want)
	}
}
