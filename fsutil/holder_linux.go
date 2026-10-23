// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fsutil

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// lockHolder names the process that holds the flock of the open lock file
// f, as /proc/locks tells it, and returns "" where it does not: the holder
// may run on another host that shares the file, or in another PID
// namespace.
func lockHolder(f *os.File) string {
	info, err := f.Stat()
	var locks []byte
	if err == nil {
		locks, err = os.ReadFile("/proc/locks")
	}
	if err != nil {
		return ""
	}
	st := info.Sys().(*syscall.Stat_t)
	dev := uint64(st.Dev) //nolint:unconvert // uint32 on some architectures
	pid := lockPID(string(locks), fmt.Sprintf("%02x:%02x:%d", unix.Major(dev), unix.Minor(dev), st.Ino))
	if pid == "" {
		return ""
	}
	return describeProcess(pid)
}

// lockPID returns the number of the process that holds a flock of the
// file, as major:minor:inode in hexadecimal, decimal, in locks, the
// content of /proc/locks, or "".
func lockPID(locks, file string) string {
	for line := range strings.Lines(locks) {
		// "1: FLOCK  ADVISORY  WRITE 4242 fd:01:1834 0 EOF"; a process
		// that waits for the lock has "->" after the number, and holds
		// nothing.
		f := strings.Fields(line)
		if len(f) < 6 || f[1] != "FLOCK" || f[5] != file {
			continue
		}
		if pid, err := strconv.Atoi(f[4]); err == nil && pid > 0 {
			return f[4]
		}
	}
	return ""
}

// describeProcess names a process by its number, and by its name and user
// as far as /proc tells them.
func describeProcess(pid string) string {
	var about []string
	if comm, err := os.ReadFile("/proc/" + pid + "/comm"); err == nil {
		// A process chooses its own name, and it may be another user's.
		about = append(about, termtext.Escape(strings.TrimSpace(string(comm))))
	}
	if info, err := os.Stat("/proc/" + pid); err == nil {
		uid, _, _ := owner(info)
		who := "uid " + strconv.Itoa(uid)
		if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
			who = termtext.Escape(u.Username)
		}
		about = append(about, "run by "+who)
	}
	if len(about) == 0 {
		return "process " + pid
	}
	return "process " + pid + " (" + strings.Join(about, ", ") + ")"
}
