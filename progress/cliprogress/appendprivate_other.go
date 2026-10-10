// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package cliprogress

import "os"

// appendPrivate opens a file to append to, and creates it readable and
// writable by its owner alone where modes say so. Where files have no Unix
// owner, it checks nothing more: not who owns the file, its links or a
// pipe, and it opens /dev/stderr, if there is one, as a file.
func appendPrivate(path string, _ int) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
}
