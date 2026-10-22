// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix || aix

package cliprogress

// lockRotation takes no lock where the system has no flock, nor on AIX,
// for which golang.org/x/sys has none: the second look rotate takes keeps
// commands that start at once from moving the log twice, all but in a
// narrow window.
func lockRotation(string) (unlock func(), ok bool) { return func() {}, true }
