// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
)

// queueListener hands out the connections it was given, then fails with
// net.ErrClosed.
type queueListener struct {
	mu    sync.Mutex
	conns []net.Conn
}

func (q *queueListener) Accept() (net.Conn, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.conns) == 0 {
		return nil, net.ErrClosed
	}
	c := q.conns[0]
	q.conns = q.conns[1:]
	return c, nil
}

func (q *queueListener) Close() error { return nil }

func (q *queueListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80} }

// addrConn is a connection that claims the addresses it is given.
type addrConn struct {
	net.Conn
	local, remote net.Addr
}

func (c *addrConn) LocalAddr() net.Addr  { return c.local }
func (c *addrConn) RemoteAddr() net.Addr { return c.remote }

// pipeConn returns a connection with the given addresses, and the other
// end of its pipe.
func pipeConn(local, remote net.Addr) (*addrConn, net.Conn) {
	a, b := net.Pipe()
	return &addrConn{Conn: a, local: local, remote: remote}, b
}

// isClosedBy reports whether the other end of a pipe sees it closed.
func isClosedBy(peer net.Conn) bool {
	_, err := peer.Read(make([]byte, 1))
	return err != nil
}

func tcpAddr(s string) *net.TCPAddr { return net.TCPAddrFromAddrPort(netip.MustParseAddrPort(s)) }

func TestPeerListenerAccept(t *testing.T) {
	t.Parallel()

	const me = 1000
	server, client := tcpAddr("127.0.0.1:7374"), tcpAddr("127.0.0.1:50000")
	mapped := &net.TCPAddr{IP: net.ParseIP("::ffff:127.0.0.1"), Port: 50000}
	boom := errors.New("boom")
	type owned struct {
		uid int
		err error
	}
	tests := []struct {
		name          string
		local, remote net.Addr
		owner         owned
		refuse        bool // pass a refused callback
		accepted      bool
		wantErr       string
		wantIs        error
	}{
		{name: "the user's", local: server, remote: client, owner: owned{uid: me}, accepted: true},
		{name: "the user's, mapped", local: server, remote: mapped, owner: owned{uid: me}, accepted: true},
		{name: "another user's", local: server, remote: client, owner: owned{uid: 1001}, refuse: true, wantErr: "closed the connection from 127.0.0.1:50000: it is user 1001's"},
		{name: "another user's, nobody told", local: server, remote: client, owner: owned{uid: 1001}},
		{name: "root's", local: server, remote: client, owner: owned{uid: 0}, refuse: true, wantErr: "closed the connection from 127.0.0.1:50000: it is user 0's"},
		{name: "no owner found", local: server, remote: client, owner: owned{err: boom}, refuse: true, wantErr: "closed the connection from 127.0.0.1:50000: boom", wantIs: boom},
		{name: "no owner found, nobody told", local: server, remote: client, owner: owned{err: boom}},
		{
			name: "no TCP address", local: &net.UnixAddr{Name: "/x", Net: "unix"}, remote: &net.UnixAddr{Name: "/y", Net: "unix"},
			owner: owned{uid: me}, refuse: true, wantErr: "closed a connection: /y is not a TCP address\n/x is not a TCP address",
		},
		{
			name: "a client address that is no TCP address", local: server, remote: &net.UnixAddr{Name: "/y", Net: "unix"},
			owner: owned{uid: me}, refuse: true, wantErr: "closed a connection: /y is not a TCP address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bad, badPeer := pipeConn(tt.local, tt.remote)
			good, goodPeer := pipeConn(server, client)
			var asked []netip.AddrPort
			l := &peerListener{
				Listener: &queueListener{conns: []net.Conn{bad, good}},
				uid:      me,
				owner: func(local, remote netip.AddrPort) (int, error) {
					asked = append(asked, local, remote)
					if len(asked) > 2 {
						return me, nil // the second connection
					}
					return tt.owner.uid, tt.owner.err
				},
			}
			var refusals []error
			if tt.refuse {
				l.refused = func(err error) { refusals = append(refusals, err) }
			}

			c, err := l.Accept()
			if err != nil {
				t.Fatalf("Accept: %v", err)
			}

			if tt.accepted {
				if c != net.Conn(bad) {
					t.Errorf("Accept returned another connection than the user's")
				}
				if len(refusals) != 0 {
					t.Errorf("refused %v", refusals)
				}
				// Mapped addresses are asked for as IPv4.
				if want := netip.MustParseAddrPort("127.0.0.1:50000"); len(asked) < 2 || asked[0] != want || asked[1] != netip.MustParseAddrPort("127.0.0.1:7374") {
					t.Errorf("asked %v", asked)
				}
				_ = bad.Close()
				_ = goodPeer.Close()
				return
			}
			if c != net.Conn(good) {
				t.Errorf("Accept returned another connection than the second")
			}
			if !isClosedBy(badPeer) {
				t.Error("the refused connection is left open")
			}
			if tt.refuse {
				if len(refusals) != 1 || refusals[0].Error() != tt.wantErr {
					t.Errorf("refusals = %v, want %q", refusals, tt.wantErr)
				} else if tt.wantIs != nil && !errors.Is(refusals[0], tt.wantIs) {
					t.Errorf("refusal %v does not wrap %v", refusals[0], tt.wantIs)
				}
			}
			_ = good.Close()
		})
	}
}

// unixListener is a listener with the address of a Unix socket.
type unixListener struct{ queueListener }

func (*unixListener) Addr() net.Addr {
	return &net.UnixAddr{Name: "/run/user/1000/prog.sock", Net: "unix"}
}

// A listener that is not TCP is refused, on every system, as each of its
// connections would be closed.
func TestPeerListenerRefusesAListenerThatIsNotTCP(t *testing.T) {
	t.Parallel()

	pl, err := PeerListener(&unixListener{}, 0, nil)

	if pl != nil || err == nil || !strings.Contains(err.Error(), "/run/user/1000/prog.sock is not TCP") ||
		errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("PeerListener = %v, %v, want an error that the listener is not TCP", pl, err)
	}
}

func TestPeerListenerAcceptFailure(t *testing.T) {
	t.Parallel()

	l := &peerListener{Listener: &queueListener{}, uid: 1}

	c, err := l.Accept()

	if c != nil || !errors.Is(err, net.ErrClosed) {
		t.Errorf("Accept = %v, %v, want net.ErrClosed", c, err)
	}
}

func TestAddrPort(t *testing.T) {
	t.Parallel()

	got, err := addrPort(&net.TCPAddr{IP: net.ParseIP("::ffff:10.1.2.3"), Port: 99})
	if want := netip.MustParseAddrPort("10.1.2.3:99"); err != nil || got != want {
		t.Errorf("addrPort = %v, %v, want %v", got, err, want)
	}
	got, err = addrPort(tcpAddr("[::1]:5"))
	if want := netip.MustParseAddrPort("[::1]:5"); err != nil || got != want {
		t.Errorf("addrPort = %v, %v, want %v", got, err, want)
	}
	if _, err := addrPort(&net.IPAddr{IP: net.IPv4(1, 2, 3, 4)}); err == nil || !strings.Contains(err.Error(), "is not a TCP address") {
		t.Errorf("addrPort of an IP address = %v", err)
	}
}
