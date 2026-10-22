// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package localweb

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// checkSocketPath refuses every socket: on a system that is not Unix,
// nothing checks who may connect to one or replace it.
func checkSocketPath(string) error {
	return fmt.Errorf("a Unix socket is not supported on this system: %w", errors.ErrUnsupported)
}

// listenSocket is never called: CheckAddr refuses every socket.
func listenSocket(context.Context, bindFunc, string) (net.Listener, string, error) {
	return nil, "", checkSocketPath("")
}

// addrInUse cannot tell a port in use on this system, so a port in use is
// an error.
func addrInUse(error) bool { return false }

// noSuchAddress cannot tell a missing address on this system, so a
// missing ::1 is an error.
func noSuchAddress(error) bool { return false }
