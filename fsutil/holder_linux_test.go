// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fsutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLockPID(t *testing.T) {
	t.Parallel()

	const file = "fd:01:1834"
	for _, tc := range []struct {
		name, locks, want string
	}{
		{"the holder", "1: FLOCK  ADVISORY  WRITE 4242 fd:01:1834 0 EOF\n", "4242"},
		{"the holder after others", "1: POSIX  ADVISORY  WRITE 7 fd:01:1834 0 EOF\n" +
			"2: FLOCK  ADVISORY  WRITE 8 fd:01:99 0 EOF\n" +
			"3: FLOCK  ADVISORY  WRITE 4242 fd:01:1834 0 EOF\n", "4242"},
		{"one that waits holds nothing", "1: -> FLOCK  ADVISORY  WRITE 4243 fd:01:1834 0 EOF\n", ""},
		{"a holder in another namespace", "1: FLOCK  ADVISORY  WRITE 0 fd:01:1834 0 EOF\n", ""},
		{"a number that is none", "1: FLOCK  ADVISORY  WRITE x fd:01:1834 0 EOF\n", ""},
		{"a short line", "1: FLOCK\n", ""},
		{"no locks", "", ""},
	} {
		if got := lockPID(tc.locks, file); got != tc.want {
			t.Errorf("%s: lockPID = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestLockHolder(t *testing.T) {
	t.Parallel()

	f, err := os.Create(filepath.Join(t.TempDir(), ".state.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if got := lockHolder(f); got != "" {
		t.Errorf("a file nobody locks is held by %q", got)
	}
	_ = f.Close()
	if got := lockHolder(f); got != "" {
		t.Errorf("a closed file is held by %q", got)
	}
}

func TestDescribeProcess(t *testing.T) {
	t.Parallel()

	pid := strconv.Itoa(os.Getpid())
	if got, want := describeProcess(pid), "process "+pid+" ("; !strings.HasPrefix(got, want) || !strings.Contains(got, ", run by ") {
		t.Errorf("describeProcess = %q, want it to begin with %q and name the user", got, want)
	}
	// No process has the number 0, and /proc tells nothing of it.
	if got, want := describeProcess("0"), "process 0"; got != want {
		t.Errorf("describeProcess = %q, want %q", got, want)
	}
}
