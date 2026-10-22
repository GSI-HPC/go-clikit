// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package cliprogress

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// lockRotation takes the lock of the rotation of the log at path, an
// exclusive flock of path+".lock", which it creates readable and writable
// by this user alone, without following a link put at its name. It waits
// for no other command: ok is false when the lock is held, or cannot be
// taken.
func lockRotation(path string) (unlock func(), ok bool) {
	lock, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, false
	}
	if unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		_ = lock.Close()
		return nil, false
	}
	// Closing the file lets the lock go.
	return func() { _ = lock.Close() }, true
}
