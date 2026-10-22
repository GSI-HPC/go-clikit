// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
)

// peerListener is the listener PeerListener returns.
type peerListener struct {
	net.Listener
	uid int
	// owner returns the user of the socket at local connected to remote.
	owner   func(local, remote netip.AddrPort) (int, error)
	refused func(error)
}

// Accept returns the next connection of the user, and closes the others.
func (l *peerListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		err = l.check(c)
		if err == nil {
			return c, nil
		}
		_ = c.Close()
		if l.refused != nil {
			l.refused(err)
		}
	}
}

// check looks up the owner of the client's end of c.
func (l *peerListener) check(c net.Conn) error {
	client, err1 := addrPort(c.RemoteAddr())
	server, err2 := addrPort(c.LocalAddr())
	if err := errors.Join(err1, err2); err != nil {
		return fmt.Errorf("closed a connection: %w", err)
	}
	uid, err := l.owner(client, server)
	if err != nil {
		return fmt.Errorf("closed the connection from %s: %w", client, err)
	}
	if uid != l.uid {
		return fmt.Errorf("closed the connection from %s: it is user %d's", client, uid)
	}
	return nil
}

// addrPort returns the IP address and port of a, without an IPv4 address
// mapped into IPv6.
func addrPort(a net.Addr) (netip.AddrPort, error) {
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("%s is not a TCP address", a)
	}
	ap := tcp.AddrPort()
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), nil
}
