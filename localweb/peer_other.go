// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package localweb

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
)

// PeerListener returns a listener that accepts from l only the TCP
// connections that a process of the user uid opened, and closes every
// other one before a request is read: other users of the machine can
// connect to a loopback port too. refused, when it is not nil, is told why
// each connection was closed, for the program's log.
//
// The kernel tells whose a connection is. On Linux, PeerListener reads it
// from /proc/net/tcp and /proc/net/tcp6, and trusts only an established
// socket that a process holds. On other systems it returns an error that
// matches errors.ErrUnsupported, and the program decides: serve on a Unix
// socket (Listen), or refuse to start. A connection whose owner cannot be
// told is closed.
func PeerListener(net.Listener, int, func(error)) (net.Listener, error) {
	return nil, fmt.Errorf("the owner of a TCP connection cannot be told on this system: %w", errors.ErrUnsupported)
}

// ListenerOwner returns the user ID of the TCP socket that listens at
// addr, such as the one a program recorded when it started: before a
// program hands a browser the address and token of a server it did not
// start, it checks that the server is the user's own, as another user can
// listen on a port that was given up.
//
// On Linux, ListenerOwner reads it from /proc/net/tcp and /proc/net/tcp6,
// and trusts only a socket that a process holds, bound to addr itself. On
// other systems it returns an error that matches errors.ErrUnsupported.
func ListenerOwner(netip.AddrPort) (int, error) {
	return 0, fmt.Errorf("the owner of a TCP socket cannot be told on this system: %w", errors.ErrUnsupported)
}
