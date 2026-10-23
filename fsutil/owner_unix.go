// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package fsutil

import (
	"io/fs"
	"os"
	"syscall"
)

// owner returns the user and group that own the file info describes.
func owner(info fs.FileInfo) (uid, gid int, ok bool) {
	st := info.Sys().(*syscall.Stat_t)
	return int(st.Uid), int(st.Gid), true
}

// ensureDir is EnsureDir for the user me, which is os.Geteuid and which a
// test makes another.
func ensureDir(dir string, me int) error {
	err := os.MkdirAll(dir, 0o700)
	var info fs.FileInfo
	if err == nil {
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return err
	}
	uid, _, _ := owner(info)
	switch perm := info.Mode().Perm(); {
	case !info.IsDir():
		return untrusted("%s is no directory of its own: a symbolic link to one is refused", dir)
	case uid != me:
		return untrusted("%s is owned by uid %d, not by this user (uid %d)", dir, uid, me)
	case perm&0o022 != 0:
		return untrusted("%s can be written by others than its owner (mode %v); run chmod go-w %s", dir, perm, dir)
	}
	return nil
}

// syncDir makes a change to the entries of the directory dir durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err == nil {
		err = d.Sync()
		if cerr := d.Close(); err == nil {
			err = cerr
		}
	}
	return err
}
