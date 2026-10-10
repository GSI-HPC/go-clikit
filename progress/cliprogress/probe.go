// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress

import (
	"errors"
	"io"
	"io/fs"
)

// errNoTerminal is the error TerminalSize returns for a writer that is no
// file the system can be asked about, and for any on a system that is not
// Unix.
var errNoTerminal = errors.New("not a terminal")

// IsPipe reports whether w is a file open on a pipe or a socket, as
// Options.IntoPipe asks of standard output. What is no file is neither.
func IsPipe(w io.Writer) bool {
	f, ok := w.(interface{ Stat() (fs.FileInfo, error) })
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&(fs.ModeNamedPipe|fs.ModeSocket) != 0
}
