// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package launch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/GSI-HPC/go-clikit/localweb"
)

// Acquire returns the instance of program in runtimeDir, such as
// $XDG_RUNTIME_DIR, and takes its lock: it waits while another start of
// the program holds it, so that two starts cannot both find no server
// running and both start one. The lock is held until Publish or Release.
// When ctx ends before the lock is free, Acquire returns the context's
// error, wrapped; a lock that is free it takes even then.
//
// The instance lives in the directory runtimeDir/program, which Acquire
// makes for the user alone. A directory that is there already must be
// private: a directory, not a link to one, of the user's, of mode 0700, as
// another user could read or replace what is in it otherwise.
func Acquire(ctx context.Context, runtimeDir, program string) (*Instance, error) {
	return acquire(ctx, runtimeDir, program, unix.Flock)
}

// lockPoll is how often a start tries again to take the lock that another
// start holds.
const lockPoll = 50 * time.Millisecond

// acquire is Acquire with the lock taken by flock.
func acquire(ctx context.Context, runtimeDir, program string, flock func(fd, how int) error) (*Instance, error) {
	if !filepath.IsAbs(runtimeDir) {
		return nil, fmt.Errorf("the runtime directory %q is no absolute path", runtimeDir)
	}
	if err := checkProgram(program); err != nil {
		return nil, err
	}
	dir := filepath.Join(runtimeDir, program)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("make the runtime directory: %w", err)
	}
	if err := checkPrivateDir(dir, os.Getuid()); err != nil {
		return nil, err
	}
	i := &Instance{
		dir: dir, pid: os.Getpid(), uid: os.Getuid(),
		flock: flock, alive: alive, dial: dial, owner: localweb.ListenerOwner,
	}
	if err := i.lock(ctx); err != nil {
		return nil, err
	}
	return i, nil
}

// lock takes the lock, and tries again every lockPoll while another
// process holds it, until ctx ends. It tries once before it looks at ctx,
// so that a program can retire with the context that stopped its server.
// It does nothing when this instance holds the lock already: a second lock
// of the same file would wait for the first.
func (i *Instance) lock(ctx context.Context) error {
	if i.lockf != nil {
		return nil
	}
	f, err := os.OpenFile(i.lockPath(), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("open the lock of the runtime directory: %w", err)
	}
	poll := time.NewTicker(lockPoll)
	defer poll.Stop()
	for {
		err := i.flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) //nolint:gosec // G115: a file descriptor fits an int
		switch {
		case err == nil:
			i.lockf = f
			return nil
		case !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR):
			_ = f.Close()
			return fmt.Errorf("lock the runtime directory: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return fmt.Errorf("wait for the lock of the runtime directory: %w", ctx.Err())
		case <-poll.C:
		}
	}
}

// read returns the record, when it is a regular file of the user's that
// no other user can read or write, and holds a valid record. It never
// follows a link, and never waits on a named pipe.
func (i *Instance) read() (Record, bool) {
	f, err := os.OpenFile(i.recordPath(), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return Record{}, false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || owner(info) != i.uid {
		return Record{}, false
	}
	return decodeRecord(f)
}

// checkPrivateDir returns an error unless dir is a directory, not a link
// to one, of the user uid, of mode 0700.
func checkPrivateDir(dir string, uid int) error {
	info, err := os.Lstat(dir)
	switch {
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("%s is no directory", dir)
	case owner(info) != uid:
		return fmt.Errorf("%s belongs to another user", dir)
	case info.Mode().Perm() != 0o700:
		return fmt.Errorf("%s has mode %04o; it must be 0700", dir, info.Mode().Perm())
	}
	return nil
}

// owner returns the user ID of the file that info describes.
func owner(info os.FileInfo) int {
	return int(info.Sys().(*syscall.Stat_t).Uid)
}

// alive reports whether the process pid runs and is the user's: signal 0
// checks both and sends nothing. A process of another user that took the
// ID after the server ended answers EPERM.
func alive(pid int) bool {
	return unix.Kill(pid, 0) == nil
}
