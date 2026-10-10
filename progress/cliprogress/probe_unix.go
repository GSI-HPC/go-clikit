// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cliprogress

import (
	"io"
	"syscall"

	"golang.org/x/sys/unix"
)

// TerminalSize returns how many columns and rows the terminal w is open on
// has, as Options.Size asks of standard error, afresh at each call, so that
// a window resized is seen. What is no terminal is an error, and so is
// anything on a system that is not Unix.
func TerminalSize(w io.Writer) (cols, rows int, err error) {
	var ws *unix.Winsize
	err = control(w, func(fd int) (err error) {
		ws, err = unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	return int(ws.Col), int(ws.Row), nil
}

// InForeground reports whether the process is the job in the foreground of
// the terminal w is open on, as Options.Foreground asks of standard error:
// the job the shell hands the terminal to, and not one started with & or
// sent there by Ctrl-Z and bg. A terminal that cannot say, as one that is
// not the process's controlling terminal cannot, and what is no terminal,
// are taken to be the process's; so is everything on a system that is not
// Unix.
func InForeground(w io.Writer) bool {
	var group int
	err := control(w, func(fd int) (err error) {
		// Only a terminal says which job its foreground is. A pipe
		// answers TIOCGPGRP on macOS too, with the process group its
		// SIGIO goes to, none unless one was set; TIOCGWINSZ, which no
		// pipe or socket answers, tells a terminal first.
		if _, err = unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ); err != nil {
			return err
		}
		group, err = unix.IoctlGetInt(fd, unix.TIOCGPGRP)
		return err
	})
	// TIOCGPGRP writes a pid_t, four bytes, at the start of the int
	// IoctlGetInt reads, which on a big-endian system of 64 bits, s390x or
	// ppc64, are its upper half. The two halves, or'd, hold it either way.
	v := uint64(group)
	// Getpgid of the process itself fails for no process.
	own, _ := unix.Getpgid(0)
	return err != nil || int(uint32(v|v>>32)) == own
}

// control calls fn with the descriptor w is open on, and returns what fn
// returned. It asks for the descriptor without putting it into blocking
// mode, as os.File.Fd would.
func control(w io.Writer, fn func(fd int) error) error {
	c, ok := w.(syscall.Conn)
	if !ok {
		return errNoTerminal
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var fnErr error
	if err := raw.Control(func(fd uintptr) { fnErr = fn(int(fd)) }); err != nil {
		return err
	}
	return fnErr
}
