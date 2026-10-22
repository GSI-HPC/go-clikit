// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package launch

import "os/exec"

// Detached runs name with args and returns once it has started, without
// waiting for it: the start of a desktop's opener, which hands a file to
// the browser and exits. It is waited for in the background, so that it
// does not stay behind as a zombie, and its output is discarded, so that a
// browser's chatter does not land in the terminal. On Unix it runs in a
// session of its own.
func Detached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
