// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/fanout"
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
