// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package cliprogress

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// big makes a log of 200 bytes at a new path, and returns the path.
func big(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "activity.jsonl")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 200)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// moved reports whether rotate moved the log at path aside.
func moved(path string) bool {
	_, err := os.Stat(path + ".1")
	return err == nil
}

func TestRotate(t *testing.T) {
	path := big(t)
	rotate(path, 100)
	if !moved(path) {
		t.Fatal("a log larger than the limit was not moved")
	}
	if fi, err := os.Stat(path + ".lock"); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the lock: %v, %v; want a file of mode 0600", fi, err)
	}
}

// A command that finds the lock held leaves the log to the one that holds
// it, and waits for none.
func TestRotateLeavesALogWhoseLockIsHeld(t *testing.T) {
	path := big(t)
	lock, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	rotate(path, 100)
	if moved(path) {
		t.Error("a log whose lock is held was moved")
	}
}

// What is no regular file, and a log whose lock cannot be made, are left
// as they are.
func TestRotateLeavesWhatItCannotMove(t *testing.T) {
	t.Run("a link", func(t *testing.T) {
		target := big(t)
		path := filepath.Join(t.TempDir(), "activity.jsonl")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		rotate(path, 100)
		if moved(path) {
			t.Error("a link was moved")
		}
	})
	t.Run("a lock that cannot be made", func(t *testing.T) {
		path := big(t)
		if err := os.Mkdir(path+".lock", 0o700); err != nil {
			t.Fatal(err)
		}
		rotate(path, 100)
		if moved(path) {
			t.Error("the log was moved without its lock")
		}
	})
}
