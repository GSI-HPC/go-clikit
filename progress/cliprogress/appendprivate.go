// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cliprogress

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// maxLinks bounds how many symbolic links appendPrivate follows, as the
// system bounds the lookup of a path.
const maxLinks = 40

// appendPrivate opens a file to append to that nobody but the user me may
// read, the event log, whose events name nodes and say why they failed,
// and creates it readable and writable by me alone when it is not there.
// me is this process's user, os.Geteuid, which a test makes another.
//
// A regular file that is there already has to be one me owns, under no
// other name, that nobody else can read or write: the lines appended would
// be read by whoever else can read it, and could be mixed with lines of
// theirs by whoever else can write it, and a hard link another user made
// to a file of me's would have the lines appended to that file. A named
// pipe has to be me's or root's, since whoever reads it reads the lines. A
// terminal or another device is opened as it is. A path that names one of
// this process's own descriptors, /dev/stderr, /dev/fd/3, is written
// through a duplicate of that descriptor, wherever the shell that started
// the process pointed it: the lines follow what the process writes there,
// not at the start of a file stderr was redirected to.
//
// The path is often in a directory others can write to, such as /tmp, so
// a symbolic link on the way to the file, the file's own or a directory's,
// has to be me's or root's too: one another user made would have the lines
// appended to a file of me's that it points at, or a file created where it
// points. A path whose last component is no link when it is looked at is
// opened without following one there, whatever links lead to its
// directory, so that none put there since takes the file's place. One
// whose last component is a link is opened through it, since a link to a
// pipe in /proc/<pid>/fd names no path to open in its place, and so
// through a link put where it points since.
func appendPrivate(path string, me int) (*os.File, error) {
	if f, ok, err := ownDescriptor(path); ok {
		return f, err
	}
	follow, err := trustedLinks(path, me)
	if err != nil {
		return nil, err
	}
	return openPrivate(path, follow, me)
}

// openPrivate opens path, which trustedLinks walked, as appendPrivate
// says, following a link as its last component only when follow says the
// walk found one there. It opens without blocking, so that a named pipe
// with no reader, which another user may have put at the path since,
// fails at once instead of holding the command up until someone reads it;
// one me or root owns is then waited on, as an open waits.
func openPrivate(path string, follow bool, me int) (*os.File, error) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND
	if !follow {
		flags |= syscall.O_NOFOLLOW
	}
	f, err := os.OpenFile(path, flags|syscall.O_NONBLOCK, 0o600)
	if errors.Is(err, syscall.ENXIO) {
		f, err = waitForReader(path, flags, me, err)
	}
	if err != nil {
		return nil, err
	}
	// The file opened is the one checked, whatever was at the path before.
	info, err := f.Stat()
	if err == nil {
		err = private(path, info, me)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	// Writes wait for a reader that is behind, as on a file opened without
	// O_NONBLOCK; Go would not wait for them on a named pipe on macOS,
	// which it leaves out of its poller.
	_ = control(f, func(fd int) error { return syscall.SetNonblock(fd, false) })
	return f, nil
}

// waitForReader opens path once something reads it, as an open without
// O_NONBLOCK does, when it is a named pipe me or root owns that had no
// reader. Anything else returns nxio, the error of the open that found
// none.
func waitForReader(path string, flags, me int, nxio error) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil || info.Mode()&fs.ModeNamedPipe == 0 {
		return nil, nxio
	}
	if err := trustedOwner(path, info, me); err != nil {
		return nil, err
	}
	return os.OpenFile(path, flags, 0o600)
}

// private refuses an open file that others than me could read the lines
// of, or write lines into, as appendPrivate says.
func private(path string, info fs.FileInfo, me int) error {
	switch mode := info.Mode(); {
	case mode&fs.ModeNamedPipe != 0:
		return trustedOwner(path, info, me)
	case !mode.IsRegular():
		return nil
	}
	if uid := owner(info); uid != me {
		return fmt.Errorf("%s is owned by uid %d, not by this user (uid %d)", path, uid, me)
	}
	if links := info.Sys().(*syscall.Stat_t).Nlink; links > 1 {
		return fmt.Errorf("%s has other names than this one (%d hard links)", path, links)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf("%s can be read or written by others than its owner (mode %v); run chmod 600 %s",
			path, perm, path)
	}
	return nil
}

// trustedLinks walks path one component at a time, as the system looks it
// up, and follows the symbolic links on the way, a directory's as well as
// the last component's, as far as they name paths that are there; it
// refuses one that neither me nor root owns. It reports whether path's own
// last component was a link, which the open has to follow; a link there
// that the walk did not see is one put there since, which it must not. A
// link that names no path, such as the one /proc/<pid>/fd/3 is to a pipe,
// ends the walk: the system follows it, and what it leads to is checked
// once it is open.
func trustedLinks(path string, me int) (follow bool, err error) {
	done := "."
	if filepath.IsAbs(path) {
		done = "/"
	}
	todo := strings.Split(path, "/")
	// own counts the components of path itself left in todo: they stay at
	// its end, behind those of the links followed, and the last of them is
	// the file's own name.
	own := len(todo)
	links := 0
	for len(todo) > 0 {
		name := todo[0]
		last := own == 1 && len(todo) == 1
		if len(todo) == own {
			own--
		}
		todo = todo[1:]
		if name == "" || name == "." {
			continue
		}
		// done holds no link, so its parent is the one ".." names.
		next := filepath.Join(done, name)
		info, err := os.Lstat(next)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// What is not there is for the open to create, or to fail on.
			return follow, nil
		case err != nil:
			return false, err
		case info.Mode()&fs.ModeSymlink == 0:
			done = next
			continue
		}
		if links++; links > maxLinks {
			return false, fmt.Errorf("%s: too many levels of symbolic links", path)
		}
		// A link that cannot be read, as one removed since Lstat, is
		// refused as one nobody trusts is.
		link := ""
		err = trustedOwner(next, info, me)
		if err == nil {
			link, err = os.Readlink(next)
		}
		if err != nil {
			return false, err
		}
		if filepath.IsAbs(link) {
			done = "/"
		}
		todo = append(strings.Split(link, "/"), todo...)
		follow = follow || last
	}
	return follow, nil
}

// trustedOwner refuses a file neither me nor root owns.
func trustedOwner(path string, info fs.FileInfo, me int) error {
	if uid := owner(info); uid != 0 && uid != me {
		return fmt.Errorf("%s is owned by uid %d, neither this user (uid %d) nor root", path, uid, me)
	}
	return nil
}

// owner returns the user that owns a file os.Stat or os.Lstat described.
func owner(info fs.FileInfo) int {
	return int(info.Sys().(*syscall.Stat_t).Uid)
}

// errNotWritable is the error of a descriptor not open for writing.
var errNotWritable = errors.New("not open for writing")

// ownDescriptor returns a descriptor of its own for the open file path
// names when that is one of this process's, /dev/stdout, /dev/stderr,
// /dev/fd/N or /proc/self/fd/N, and ok false for any other path. The
// descriptor shares the file's offset with the one the path names, as a
// shell's 2>&1 does, so that what either writes follows what the other
// wrote. One not open for writing is refused.
func ownDescriptor(path string) (f *os.File, ok bool, err error) {
	fd := -1
	switch p := filepath.Clean(path); p {
	case "/dev/stdout":
		fd = 1
	case "/dev/stderr":
		fd = 2
	default:
		for _, dir := range []string{"/dev/fd/", "/proc/self/fd/"} {
			if n, found := strings.CutPrefix(p, dir); found {
				if v, err := strconv.Atoi(n); err == nil && v >= 0 {
					fd = v
				}
			}
		}
	}
	if fd < 0 {
		return nil, false, nil
	}
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err == nil && flags&unix.O_ACCMODE == unix.O_RDONLY {
		err = errNotWritable
	}
	var dup int
	if err == nil {
		// Closed on exec as it is made, so that no child started in
		// between inherits it. AIX has no F_DUPFD_CLOEXEC; there the
		// constant is F_DUPFD's, and the duplicate is inherited.
		dup, err = unix.FcntlInt(uintptr(fd), syscall.F_DUPFD_CLOEXEC, 0)
	}
	if err != nil {
		return nil, true, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(dup), path), true, nil
}
