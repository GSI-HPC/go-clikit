// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix && !linux

package fsutil

import "os"

// lockHolder does not know who holds a lock where no /proc/locks tells it.
func lockHolder(*os.File) string { return "" }
