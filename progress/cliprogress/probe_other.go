// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package cliprogress

import "io"

// TerminalSize returns how many columns and rows the terminal w is open on
// has, as Options.Size asks of standard error. On a system that is not
// Unix it cannot say: a display drawn there takes the terminal to be 80
// columns wide.
func TerminalSize(io.Writer) (cols, rows int, err error) { return 0, 0, errNoTerminal }

// InForeground reports whether the process is the job in the foreground of
// the terminal w is open on, as Options.Foreground asks of standard error.
// On a system that is not Unix, with no jobs to tell apart, it always is.
func InForeground(io.Writer) bool { return true }
