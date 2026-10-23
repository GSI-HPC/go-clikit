// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package fsutil

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Lock takes an exclusive lock of the file at path and returns the
// function that lets go of it, which may be called more than once. The
// lock is a flock(2) of the lock file .NAME.lock beside it, so that the
// file itself can still be replaced by a rename; the directory is made, of
// mode 0700, when it is missing.
//
// Lock waits while another process holds the lock: until it holds it,
// until ctx ends, when it returns the context's error wrapped, or until
// wait passes, when it returns an error that matches
// context.DeadlineExceeded and names the file and, where the system tells,
// as /proc/locks on Linux does, the process that holds the lock. A wait of
// zero or less waits until ctx ends.
//
// The lock file is made readable, which is all that taking the lock needs,
// by whoever can write the file: by its group too when the file, or while
// there is none its directory, is writable by the group, and it gets the
// file's group. A lock that only its first user could open would lock
// every other administrator of a shared file out for good. A symbolic link
// at the name of the lock file is refused.
//
// On a system that is not Unix, or on AIX, it returns an error that
// matches errors.ErrUnsupported.
func Lock(ctx context.Context, path string, wait time.Duration) (unlock func(), err error) {
	return lock(ctx, path, wait, os.Geteuid(), unix.Flock)
}

// lockPoll is how often Lock tries again to take a lock that is held.
const lockPoll = 50 * time.Millisecond

// lock is Lock for the user me, which is os.Geteuid and which a test makes
// another, with the flock that a test makes fail.
func lock(ctx context.Context, path string, wait time.Duration, me int, flock func(fd, how int) error) (func(), error) {
	dir := filepath.Dir(path)
	name := filepath.Join(dir, "."+filepath.Base(path)+".lock")
	err := os.MkdirAll(dir, 0o700)
	perm, gid := lockMode(path)
	var f *os.File
	if err == nil {
		f, err = os.OpenFile(name, os.O_RDONLY|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, perm)
	}
	if err != nil {
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	shareLock(f, perm, gid, me)

	var timeout <-chan time.Time
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		timeout = timer.C
	}
	poll := time.NewTicker(lockPoll)
	defer poll.Stop()
	for {
		err := flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		switch {
		case err == nil:
			return sync.OnceFunc(func() { _ = f.Close() }), nil
		case !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR):
			_ = f.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("wait for the lock of %s: %w", path, ctx.Err())
		case <-timeout:
			holder := cmp.Or(lockHolder(f), "another process")
			_ = f.Close()
			return nil, &lockTimeout{fmt.Sprintf(
				"%s is locked by %s, which has not let go of it in %s (the lock is %s); try again once it is done",
				path, holder, wait, name)}
		case <-poll.C:
		}
	}
}

// lockTimeout is a lock that was not let go of in time. It is the timeout
// that the end of the wait was, without the words of a deadline, which say
// nothing of the lock.
type lockTimeout struct{ msg string }

func (e *lockTimeout) Error() string { return e.msg }
func (e *lockTimeout) Unwrap() error { return context.DeadlineExceeded }

// lockMode returns the mode and group that the lock of path is made with:
// the group may read and write it when it may write the file, or its
// directory when there is no file yet, and the group is theirs.
func lockMode(path string) (os.FileMode, int) {
	info, err := os.Stat(path)
	if err != nil {
		info, err = os.Stat(filepath.Dir(path))
	}
	perm, gid := os.FileMode(0o600), -1
	if err == nil {
		_, gid, _ = owner(info)
		if info.Mode().Perm()&0o020 != 0 {
			perm |= 0o060
		}
	}
	return perm, gid
}

// shareLock gives the lock file f, when me owns it, the mode and the group
// it is meant to have, which the umask and a directory without the setgid
// bit take away. It is best effort: the lock is held either way, and only
// another administrator's run can be stopped without it.
func shareLock(f *os.File, perm os.FileMode, gid, me int) {
	info, err := f.Stat()
	if err != nil {
		return
	}
	uid, g, _ := owner(info)
	if uid != me {
		return
	}
	if gid >= 0 && g != gid {
		_ = f.Chown(-1, gid)
	}
	if info.Mode().Perm() != perm {
		_ = f.Chmod(perm)
	}
}
