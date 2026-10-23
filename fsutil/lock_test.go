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
	"runtime"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fsutil"
)

// The lock file is .NAME.lock beside the file, made readable by whoever
// may write the file, so that every administrator of a shared file can
// take its lock (decision 37).
func TestLockFile(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		dir  os.FileMode
		file os.FileMode // 0 for no file
		want os.FileMode
	}{
		{"a private file", 0o700, 0o600, 0o600},
		{"a file its group may write", 0o755, 0o664, 0o660},
		{"no file in a directory its group may write", 0o775, 0, 0o660},
		{"no file in a private directory", 0o700, 0, 0o600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := mkdir(t, filepath.Join(t.TempDir(), "shared"), tc.dir)
			path := filepath.Join(dir, "known_hosts")
			if tc.file != 0 {
				write(t, path, "", tc.file)
			}
			unlock, err := fsutil.Lock(context.Background(), path, time.Second)
			if err != nil {
				t.Fatalf("Lock: %v", err)
			}
			unlock()
			unlock() // a second call does nothing
			if got := mode(t, filepath.Join(dir, ".known_hosts.lock")); got != tc.want {
				t.Errorf("the lock has mode %v, want %v", got, tc.want)
			}
		})
	}
}

// Lock takes the lock again once its holder lets go of it, and the
// second unlock of the first holder does not let go of the second's.
func TestLockIsTakenOnceLetGo(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "new", "file")
	unlock, err := fsutil.Lock(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		unlock()
	}()
	again, err := fsutil.Lock(context.Background(), path, time.Minute)
	if err != nil {
		t.Fatalf("Lock after the holder let go: %v", err)
	}
	defer again()
	unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := fsutil.Lock(ctx, path, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a lock let go of twice was taken: err = %v", err)
	}
}

// A lock held elsewhere is waited for as long as Lock is told, and the
// error then says which file is locked and, where the system tells, which
// process holds it: this one, through another descriptor. It is a
// timeout, and the end of the context while it waits is the context's.
func TestLockWaits(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "known_hosts")
	unlock, err := fsutil.Lock(context.Background(), path, 0)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer unlock()

	for _, tc := range []struct {
		name    string
		wait    time.Duration
		cancel  time.Duration // when the context is cancelled
		timeout time.Duration // when the context's deadline is
		want    error
		waited  time.Duration
	}{
		{"until the wait passes", fsutil.DefaultLockWait, time.Hour, 0, context.DeadlineExceeded, 30 * time.Second},
		{"until the context is cancelled", time.Minute, time.Second, 0, context.Canceled, time.Second},
		{"until the context's deadline", time.Minute, 0, time.Second, context.DeadlineExceeded, time.Second},
		{"without a wait, until the context ends", 0, 0, time.Hour, context.DeadlineExceeded, time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tc.timeout > 0 {
					ctx, cancel = context.WithTimeout(ctx, tc.timeout)
					defer cancel()
				}
				if tc.cancel > 0 {
					time.AfterFunc(tc.cancel, cancel)
				}
				start := time.Now()
				_, err := fsutil.Lock(ctx, path, tc.wait)
				if waited := time.Since(start); waited != tc.waited {
					t.Errorf("waited %v, want %v", waited, tc.waited)
				}
				if !errors.Is(err, tc.want) {
					t.Fatalf("err = %v, want %v", err, tc.want)
				}
				msg := err.Error()
				if !strings.Contains(msg, path) {
					t.Errorf("error %q does not name the file", msg)
				}
				timedOut := tc.waited == tc.wait
				if timedOut && strings.Contains(msg, "context deadline exceeded") {
					t.Errorf("error %q names the deadline of a context", msg)
				}
				holder := fmt.Sprintf("is locked by process %d (", os.Getpid())
				if !timedOut || runtime.GOOS != "linux" {
					holder = "is locked by"
				}
				if timedOut != strings.Contains(msg, holder) {
					t.Errorf("error %q names the holder %q, or not", msg, holder)
				}
			})
		})
	}
}

func TestLockFails(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path func(t *testing.T, dir string) string
		want error
	}{
		{"a file where the directory should be", func(t *testing.T, dir string) string {
			return filepath.Join(write(t, filepath.Join(dir, "file"), "", 0o600), "state")
		}, syscall.ENOTDIR},
		{"a symbolic link at the name of the lock", func(t *testing.T, dir string) string {
			symlink(t, "elsewhere", filepath.Join(dir, ".state.lock"))
			return filepath.Join(dir, "state")
		}, syscall.ELOOP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			unlock, err := fsutil.Lock(context.Background(), tc.path(t, dir), time.Second)
			if unlock != nil || !errors.Is(err, tc.want) {
				t.Errorf("Lock = %v, %v, want %v", unlock != nil, err, tc.want)
			}
			if _, err := os.Lstat(filepath.Join(dir, "elsewhere")); err == nil {
				t.Error("the lock was made where the link points")
			}
		})
	}
}
