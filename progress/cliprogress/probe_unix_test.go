// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cliprogress_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
)

// What is no terminal has no size, and is taken to be in the foreground,
// so that a display drawn on it, as a test draws one, is drawn at the
// width a terminal that does not say has; a pipe or a socket on standard
// output is what auto draws nothing for.
func TestWhatIsNoTerminalHasNoSize(t *testing.T) {
	dir := t.TempDir()
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	defer wr.Close()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	socket := os.NewFile(uintptr(fds[0]), "socket")
	defer socket.Close()
	_ = syscall.Close(fds[1])
	regular, err := os.Create(filepath.Join(dir, "regular"))
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	closed, err := os.Create(filepath.Join(dir, "closed"))
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	tests := []struct {
		name string
		w    io.Writer
		pipe bool
	}{
		{"a buffer", &bytes.Buffer{}, false},
		{"a pipe", wr, true},
		{"a socket", socket, true},
		{"a regular file", regular, false},
		{"a closed file", closed, false},
		{"no file", (*os.File)(nil), false},
	}
	for _, tc := range tests {
		if got := cliprogress.IsPipe(tc.w); got != tc.pipe {
			t.Errorf("%s: IsPipe = %v, want %v", tc.name, got, tc.pipe)
		}
		if cols, rows, err := cliprogress.TerminalSize(tc.w); err == nil {
			t.Errorf("%s: TerminalSize = %d, %d, a terminal's", tc.name, cols, rows)
		}
		if !cliprogress.InForeground(tc.w) {
			t.Errorf("%s: InForeground = false", tc.name)
		}
	}
}
