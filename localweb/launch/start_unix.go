// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package launch

import (
	"os/exec"
	"syscall"
)

// Detached runs name with args and returns once it has started, without
// waiting for it: the start of a desktop's opener, which hands a file to
// the browser and exits. It is waited for in the background, so that it
// does not stay behind as a zombie, and its output is discarded, so that a
// browser's chatter does not land in the terminal.
//
// Stopping the program must not take away a browser that the opener
// happens to still be the parent of. So the command is bound to no
// context, and it runs in a session of its own: Ctrl-C reaches every
// process of the terminal's foreground group, and closing the terminal
// every process of its session, and the opener and what it started are in
// neither.
func Detached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// The opener's exit status says nothing the program could act on: the
	// address is printed in any case.
	go func() { _ = cmd.Wait() }()
	return nil
}
