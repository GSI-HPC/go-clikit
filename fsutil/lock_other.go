// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix || aix

package fsutil

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Lock takes an exclusive lock of the file at path and returns the
// function that lets go of it. On Unix it is a flock(2) of the lock file
// .NAME.lock beside it.
//
// On this system it returns an error that matches errors.ErrUnsupported:
// the kit takes the lock with flock(2), which this system, or
// golang.org/x/sys for it, does not have.
func Lock(_ context.Context, path string, _ time.Duration) (unlock func(), err error) {
	return nil, fmt.Errorf("lock %s: %w", path, errors.ErrUnsupported)
}
