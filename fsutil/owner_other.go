// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package fsutil

import (
	"errors"
	"fmt"
	"io/fs"
)

// owner is not known where files have no owner of Unix.
func owner(fs.FileInfo) (uid, gid int, ok bool) {
	return -1, -1, false
}

// ensureDir fails: a private directory cannot be told on this system.
func ensureDir(dir string, _ int) error {
	return fmt.Errorf("make the private directory %s: %w", dir, errors.ErrUnsupported)
}

// syncDir does nothing: a directory cannot be synced on every system that
// is not Unix, as Windows opens none for writing, and the rename is as
// durable as the system makes it.
func syncDir(string) error { return nil }
