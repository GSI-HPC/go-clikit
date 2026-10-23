// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package cliprogress

// IsTerminal reports whether v is open on a terminal, as
// Options.OnTerminal asks of standard error. On a system that is not Unix
// it cannot say, and reports none: a program that draws a display there
// finds out with a library of its own.
func IsTerminal(any) bool { return false }

// TerminalSize returns how many columns and rows the terminal v is open on
// has, as Options.Size asks of standard error. On a system that is not
// Unix it cannot say: a display drawn there takes the terminal to be 80
// columns wide.
func TerminalSize(any) (cols, rows int, err error) { return 0, 0, errNoTerminal }

// InForeground reports whether the process is the job in the foreground of
// the terminal v is open on, as Options.Foreground asks of standard error.
// On a system that is not Unix, with no jobs to tell apart, it always is.
func InForeground(any) bool { return true }
