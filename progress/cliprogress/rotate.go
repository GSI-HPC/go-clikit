// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress

import "os"

// rotate moves the log at path to path+".1", in place of the copy there,
// when it is a regular file larger than limit bytes. It holds the lock of
// the rotation while it looks again and moves the log, so that of commands
// that start at once one moves it; one that finds the lock held leaves the
// log to the command that holds it. A log that cannot be moved stays where
// it is: the log is a convenience, and is appended to as it is.
func rotate(path string, limit int64) {
	if !larger(path, limit) {
		return
	}
	unlock, ok := lockRotation(path)
	if !ok {
		return
	}
	defer unlock()
	// Another command may have moved it since it was looked at.
	if larger(path, limit) {
		_ = os.Rename(path, path+".1")
	}
}

// larger reports whether path names a regular file, no link, larger than
// limit bytes.
func larger(path string, limit int64) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > limit
}
