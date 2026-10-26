// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package fsutil_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

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

// A program keeps a private file private, even when someone widened its
// mode since the last write.
func ExampleExactMode() {
	dir, _ := os.MkdirTemp("", "prog-")
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "token")
	_ = os.WriteFile(path, []byte("old"), 0o600)
	_ = os.Chmod(path, 0o644) // another program's doing

	if err := fsutil.WriteAtomic(path, []byte("new"), 0o600, fsutil.ExactMode()); err != nil {
		fmt.Println(err)
		return
	}
	info, _ := os.Stat(path)
	fmt.Println(info.Mode().Perm())
	// Output: -rw-------
}

// Several runs of a program count in one shared file, under its lock, and
// none of their counts is lost.
func ExampleUpdate() {
	dir, _ := os.MkdirTemp("", "prog-")
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "runs")

	count := func(current []byte) ([]byte, error) {
		n := 0
		if current != nil { // nil: there is no file yet
			var err error
			if n, err = strconv.Atoi(string(current)); err != nil {
				return nil, err // nothing is written
			}
		}
		return []byte(strconv.Itoa(n + 1)), nil
	}
	for range 3 {
		if err := fsutil.Update(context.Background(), path, 0o644, count); err != nil {
			fmt.Println(err)
			return
		}
	}
	data, _ := os.ReadFile(path)
	fmt.Println(string(data))
	// Output: 3
}

// A program holds the lock of a file while it works on it; another run
// gives up once the wait has passed, with an error that is a timeout and
// names the holder.
func ExampleLock() {
	dir, _ := os.MkdirTemp("", "prog-")
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "inventory")

	unlock, err := fsutil.Lock(context.Background(), path, fsutil.DefaultLockWait)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer unlock()

	_, err = fsutil.Lock(context.Background(), path, 100*time.Millisecond) // another run
	fmt.Println(errors.Is(err, context.DeadlineExceeded))
	// Output: true
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
