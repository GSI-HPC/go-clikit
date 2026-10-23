// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package fsutil writes and checks the files of a program the way a tool
// that several users, or several runs of it, share has to (decision 37):
// a file is replaced whole or not at all, and a file or directory that
// another user could have written is refused.
//
// [WriteAtomic] replaces a file through a temporary file and a rename.
// [EnsureDir] makes a private directory of the user's, and
// [CheckTrusted] refuses a file that a user other than this one or root
// could have written; both refuse with an error that matches
// [ErrUntrusted].
//
// [ErrUntrusted] is an interface of the kit; the texts of the errors are
// not. The package reads no environment and keeps no state.
package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrUntrusted is matched by errors.Is for every file or directory that
// is refused because a user other than this one or root could have
// written it.
var ErrUntrusted = errors.New("a user other than this one or root could have written it")

// untrustedError says which file was refused, and why, and matches
// ErrUntrusted.
type untrustedError struct{ msg string }

func (e *untrustedError) Error() string        { return e.msg }
func (e *untrustedError) Is(target error) bool { return target == ErrUntrusted }

// untrusted returns an error that matches ErrUntrusted.
func untrusted(format string, args ...any) error {
	return &untrustedError{fmt.Sprintf(format, args...)}
}

// maxLinks bounds how many symbolic links a lookup follows, as the system
// bounds the lookup of a path.
const maxLinks = 40

// WriteAtomic writes data to path through a temporary file in the same
// directory: it writes the temporary file, syncs it, closes it, renames it
// over path and syncs the directory. A reader never sees a file written in
// part, and a write that fails leaves the file as it was and removes the
// temporary file. The directory is made, of mode 0700, when it is missing.
//
// A file that exists keeps its mode and its group, so that a file several
// administrators share stays writable by all of them, and its owner too
// when root writes it; write permission for others is never kept. A new
// file gets exactly perm, set after it is made, whatever the umask.
//
// A symbolic link at path is written through, not replaced, for at most
// 40 links. A link that neither this user nor root owns is refused with an
// error that matches [ErrUntrusted]: whoever put it there would choose
// which file is written.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	return writeAtomic(path, data, perm, os.Geteuid(), os.Rename)
}

// writeAtomic is WriteAtomic for the user me, which is os.Geteuid and
// which a test makes another, with the rename that a test makes fail.
func writeAtomic(path string, data []byte, perm os.FileMode, me int, rename func(from, to string) error) error {
	file, info, err := resolve(path, me)
	if err == nil {
		err = replace(file, info, data, perm, me, rename)
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// replace writes data over the file path, whose information is info, or
// nil when there is no file yet, as WriteAtomic says.
func replace(path string, info fs.FileInfo, data []byte, perm os.FileMode, me int, rename func(from, to string) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	uid, gid := -1, -1
	if info != nil {
		perm = info.Mode().Perm() &^ 0o002
		if u, g, ok := owner(info); ok {
			gid = g
			if me == 0 {
				uid = u
			}
		}
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	// The owner first, as a change of owner may clear the mode's setgid
	// bit; the rename only once the content is on the disk.
	err = keepOwner(tmp, uid, gid)
	if err == nil {
		err = tmp.Chmod(perm)
	}
	if err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if err == nil {
		err = tmp.Close()
	}
	if err == nil {
		err = rename(tmp.Name(), path)
	}
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	// And the rename is only on the disk once the directory is.
	return syncDir(dir)
}

// keepOwner gives f the group gid, and the owner uid unless that is -1,
// unless gid is -1 or f has them already.
func keepOwner(f *os.File, uid, gid int) error {
	info, err := f.Stat()
	if err != nil || gid < 0 {
		return err
	}
	if u, g, _ := owner(info); g == gid && (uid < 0 || u == uid) {
		return nil
	}
	return f.Chown(uid, gid)
}

// resolve follows the symbolic links that path is, so that a write
// through a link changes the file it points at, and returns that file and
// its information, which is nil when there is no file. A link that
// neither me nor root owns is refused.
func resolve(path string, me int) (string, fs.FileInfo, error) {
	for range maxLinks {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return path, nil, nil
		case err != nil:
			return "", nil, err
		case info.Mode()&fs.ModeSymlink == 0:
			return path, info, nil
		}
		err = trustedOwner(path, info, me)
		if err == nil {
			path, err = linkTarget(path)
		}
		if err != nil {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("%s: too many levels of symbolic links", path)
}

// linkTarget returns the path the symbolic link path points at. A
// relative one is read against the directory the link is really in, as
// the system reads it: behind a linked directory, ../x is beside the
// directory the link leads to, not beside its name.
func linkTarget(path string) (string, error) {
	link, err := os.Readlink(path)
	if err != nil || filepath.IsAbs(link) {
		return link, err
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	return filepath.Join(dir, link), err
}

// EnsureDir makes the directory dir, and the directories above it that
// are missing, of mode 0700, for the private state of a program, and
// checks one that is there already. It refuses, with an error that matches
// [ErrUntrusted], a dir that is a symbolic link, even to a directory, that
// another user owns, or that its group or others can write: whoever else
// can write it can replace what the program keeps in it. It does not
// repair a mode that is too wide; the error says what to run. A file that
// is no directory is an error of the system, as os.MkdirAll returns it.
//
// On a system that is not Unix it returns an error that matches
// errors.ErrUnsupported: it tells a private directory by the owner and
// mode of Unix.
func EnsureDir(dir string) error {
	return ensureDir(dir, os.Geteuid())
}

// CheckTrusted refuses a file or directory that a user other than this
// one or root could have written: one that another user owns, or that its
// group or others can write, and one whose directory, or any directory
// above it, is so. A directory above it may be writable by its group or
// others when its sticky bit is set, as /tmp is, since nobody can then
// replace or remove what another user put in it. Every refusal matches
// [ErrUntrusted]; a file that is missing is an error that matches
// fs.ErrNotExist.
//
// The path is looked up a component at a time, as the system looks it
// up. A symbolic link on the way is followed, and has to be this user's or
// root's itself, and so do the file and the directories it leads to.
//
// Where files have no owner of Unix, only the modes the system reports
// are checked. Windows reports every directory as writable by all, so
// there CheckTrusted refuses every path.
func CheckTrusted(path string) error {
	return checkTrusted(path, os.Geteuid())
}

// checkTrusted is CheckTrusted for the user me, which is os.Geteuid and
// which a test makes another.
func checkTrusted(path string, me int) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	sep := string(filepath.Separator)
	root := filepath.VolumeName(abs) + sep
	todo := append([]string{root}, strings.Split(abs[len(root):], sep)...)
	// walked holds the directories looked up so far, from the root, none
	// of them a link; the last is where a name is looked up next.
	type entry struct {
		path string
		info fs.FileInfo
	}
	var walked []entry
	links := 0
	for len(todo) > 0 {
		name := todo[0]
		todo = todo[1:]
		switch name {
		case "", ".":
			continue
		case "..":
			if len(walked) > 1 {
				walked = walked[:len(walked)-1]
			}
			continue
		}
		next := name
		if len(walked) > 0 {
			next = filepath.Join(walked[len(walked)-1].path, name)
		}
		info, err := os.Lstat(next)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			if err := trustedDir(next, info, me); err != nil {
				return err
			}
			walked = append(walked, entry{next, info})
			continue
		}
		if links++; links > maxLinks {
			return fmt.Errorf("%s: too many levels of symbolic links", path)
		}
		link := ""
		err = trustedOwner(next, info, me)
		if err == nil {
			link, err = os.Readlink(next)
		}
		if err != nil {
			return err
		}
		if filepath.IsAbs(link) {
			walked = walked[:1]
		}
		todo = append(strings.Split(link, sep), todo...)
	}
	last := walked[len(walked)-1]
	if perm := last.info.Mode().Perm(); perm&0o022 != 0 {
		return untrusted("%s can be written by others than its owner (mode %v); run chmod go-w %s",
			last.path, perm, last.path)
	}
	return nil
}

// trustedDir refuses a file neither me nor root owns, or one its group or
// others can write without the sticky bit, which a directory above a file
// may have.
func trustedDir(path string, info fs.FileInfo, me int) error {
	if err := trustedOwner(path, info, me); err != nil {
		return err
	}
	if perm := info.Mode().Perm(); perm&0o022 != 0 && info.Mode()&fs.ModeSticky == 0 {
		return untrusted("%s can be written by others than its owner (mode %v); run chmod go-w %s",
			path, perm, path)
	}
	return nil
}

// trustedOwner refuses a file neither me nor root owns.
func trustedOwner(path string, info fs.FileInfo, me int) error {
	if uid, _, ok := owner(info); ok && uid != 0 && uid != me {
		return untrusted("%s is owned by uid %d, neither this user (uid %d) nor root", path, uid, me)
	}
	return nil
}
