// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix || aix

package launch

import (
	"errors"
	"fmt"
)

// Acquire returns the instance of program in runtimeDir, such as
// $XDG_RUNTIME_DIR, and takes its lock: it waits while another start of
// the program holds it, so that two starts cannot both find no server
// running and both start one. The lock is held until Publish or Release.
//
// On this system it returns an error that matches errors.ErrUnsupported:
// the kit takes the lock with flock(2) and tells a private directory by
// the mode of Unix.
func Acquire(string, string) (*Instance, error) {
	return nil, fmt.Errorf("a single instance cannot be kept on this system: %w", errors.ErrUnsupported)
}

// read finds no record: Acquire returns no instance on this system.
func (*Instance) read() (Record, bool) { return Record{}, false }

// lock fails: Acquire returns no instance on this system.
func (*Instance) lock() error { return errors.ErrUnsupported }

// checkPrivateDir fails: a private directory cannot be told on this
// system.
func checkPrivateDir(string, int) error {
	return fmt.Errorf("a private directory cannot be told on this system: %w", errors.ErrUnsupported)
}
