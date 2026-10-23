// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package fsutil_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GSI-HPC/go-clikit/fsutil"
)

// A program replaces its state whole: a reader sees the old state or the
// new one, never a part of either.
func ExampleWriteAtomic() {
	dir, _ := os.MkdirTemp("", "prog-") // the program's state directory
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "state.json")

	if err := fsutil.WriteAtomic(path, []byte(`{"runs":1}`), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	data, _ := os.ReadFile(path)
	fmt.Println(string(data))
	// Output: {"runs":1}
}

// A program keeps its private state in a directory that only its user
// can write, and refuses one that others can.
func ExampleEnsureDir() {
	base, _ := os.MkdirTemp("", "prog-")
	defer func() { _ = os.RemoveAll(base) }()
	dir := filepath.Join(base, "state", "prog")

	fmt.Println(fsutil.EnsureDir(dir))
	_ = os.Chmod(dir, 0o777) // another user's doing
	fmt.Println(errors.Is(fsutil.EnsureDir(dir), fsutil.ErrUntrusted))
	// Output:
	// <nil>
	// true
}

// A program reads configuration that names commands to run only when no
// user other than its own or root could have written it.
func ExampleCheckTrusted() {
	dir, _ := os.MkdirTemp("", "prog-")
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "config.toml")
	_ = os.WriteFile(path, []byte("ssh = \"ssh\"\n"), 0o644)

	fmt.Println(fsutil.CheckTrusted(path))
	_ = os.Chmod(path, 0o666)
	fmt.Println(errors.Is(fsutil.CheckTrusted(path), fsutil.ErrUntrusted))
	// Output:
	// <nil>
	// true
}
