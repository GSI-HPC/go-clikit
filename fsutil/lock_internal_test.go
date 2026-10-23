// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package fsutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A flock that fails for another reason than a lock held elsewhere is
// not waited on.
func TestLockFailsWithTheFlock(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state")
	unlock, err := lock(context.Background(), path, time.Hour, os.Geteuid(), func(int, int) error {
		return syscall.ENOLCK
	})
	if unlock != nil || !errors.Is(err, syscall.ENOLCK) {
		t.Errorf("lock = %v, %v, want ENOLCK", unlock != nil, err)
	}
}

// shareLock changes only a lock file this user owns, gives it the group
// it is meant to have, and does nothing with a file it cannot look at.
func TestShareLock(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		me     func(t *testing.T, name string) int
		closed bool
		group  int // added to the group the lock has
		want   os.FileMode
	}{
		{"its own lock", func(*testing.T, string) int { return os.Geteuid() }, false, 0, 0o660},
		{"its own lock of another group", func(*testing.T, string) int { return os.Geteuid() }, false, 1, 0o660},
		{"another user's lock", stranger, false, 0, 0o600},
		{"a lock it cannot look at", func(*testing.T, string) int { return os.Geteuid() }, true, 0, 0o600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name := filepath.Join(t.TempDir(), ".state.lock")
			f, err := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			me := tc.me(t, name)
			if tc.closed {
				_ = f.Close()
			}
			_, gid := ids(t, name)
			shareLock(f, 0o660, gid+tc.group, me)
			info, err := os.Stat(name)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != tc.want {
				t.Errorf("mode = %v, want %v", got, tc.want)
			}
		})
	}
}
