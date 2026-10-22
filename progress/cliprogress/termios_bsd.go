// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package cliprogress

import "golang.org/x/sys/unix"

// ioctlReadTermios reads the settings of a terminal, which only a terminal
// has.
const ioctlReadTermios = unix.TIOCGETA
