// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"
)

type fakeListener struct {
	addr     net.Addr
	closeErr error

	mu     sync.Mutex
	closed bool
}

func (f *fakeListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }

func (f *fakeListener) Addr() net.Addr { return f.addr }

func (f *fakeListener) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return f.closeErr
}

func (f *fakeListener) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func TestCheckAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		ok   bool
	}{
		{addr: "127.0.0.1:7374", ok: true},
		{addr: "127.0.0.1:0", ok: true},
		{addr: "localhost:7374", ok: true},
		{addr: "[::1]:7374", ok: true},
		{addr: "127.0.0.2:65535", ok: true},
		{addr: "unix:/run/user/1000/prog/prog.sock", ok: true},
		{addr: "unix:/s", ok: true},

		{addr: "0.0.0.0:7374"},
		{addr: "[::]:7374"},
		// An empty host binds every interface.
		{addr: ":7374"},
		{addr: "192.0.2.10:7374"},
		{addr: "example.org:7374"},
		// Only the one fixed name is taken.
		{addr: "localhost.localdomain:7374"},
		{addr: "127.0.0.1"},
		{addr: "localhost"},
		{addr: "127.0.0.1:"},
		{addr: "localhost:"},
		{addr: "127.0.0.1:http"},
		{addr: "127.0.0.1:65536"},
		{addr: "127.0.0.1:-1"},
		{addr: ""},
		{addr: "unix:"},
		{addr: "unix:prog.sock"},
		{addr: "unix:./prog.sock"},
		{addr: "UNIX:/run/user/1000/prog/prog.sock"},
		{addr: "unix///run/user/1000/prog/prog.sock"},
		{addr: "tcp:127.0.0.1:7374"},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			t.Parallel()

			err := CheckAddr(tt.addr)

			if tt.ok && err != nil {
				t.Errorf("CheckAddr(%q) = %v, want nil", tt.addr, err)
			}
			if !tt.ok && err == nil {
				t.Errorf("CheckAddr(%q) = nil, want an error", tt.addr)
			}
		})
	}
}

func TestAnswers(t *testing.T) {
	t.Parallel()

	v4 := netip.MustParseAddrPort("127.0.0.1:7374")
	v6 := netip.MustParseAddrPort("[::1]:7374")
	tests := []struct {
		name string
		b    Bound
		yes  []string
		no   []string
	}{
		{
			name: "one address",
			b:    Bound{Addrs: []netip.AddrPort{v4}},
			yes:  []string{"127.0.0.1:7374", "127.0.0.1:7374"},
			no: []string{
				// The name leads to ::1 first, which this server does not hold.
				"localhost:7374", "[::1]:7374", "127.0.0.1", "127.0.0.1:7375", "127.0.0.1:", "", "example.org",
				"127.0.0.1:7374.example.org",
			},
		},
		{
			name: "one IPv6 address",
			b:    Bound{Addrs: []netip.AddrPort{v6}},
			yes:  []string{"[::1]:7374"},
			no:   []string{"::1", "::1:7374", "localhost:7374", "127.0.0.1:7374"},
		},
		{
			name: "both addresses on one port",
			b:    Bound{Addrs: []netip.AddrPort{v4, v6}},
			yes:  []string{"127.0.0.1:7374", "[::1]:7374", "localhost:7374", "LocalHost:7374", "LOCALHOST:7374", "[::1]:7374"},
			no:   []string{"localhost", "localhost:7375", "127.0.0.1", "localhost:", "foo.localhost:7374"},
		},
		{
			name: "the addresses the other way round",
			b:    Bound{Addrs: []netip.AddrPort{v6, v4}},
			yes:  []string{"localhost:7374"},
		},
		{
			name: "two ports",
			b:    Bound{Addrs: []netip.AddrPort{v4, netip.MustParseAddrPort("[::1]:7375")}},
			yes:  []string{"127.0.0.1:7374", "[::1]:7375"},
			no:   []string{"localhost:7374", "localhost:7375", "127.0.0.1:7375", "[::1]:7374"},
		},
		{
			name: "another address of the loopback network",
			b:    Bound{Addrs: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.2:7374"), v6}},
			yes:  []string{"127.0.0.2:7374", "[::1]:7374"},
			no:   []string{"localhost:7374", "127.0.0.1:7374"},
		},
		{
			name: "port 80 is left out of Host",
			b: Bound{Addrs: []netip.AddrPort{
				netip.MustParseAddrPort("127.0.0.1:80"), netip.MustParseAddrPort("[::1]:80"),
			}},
			yes: []string{"127.0.0.1", "127.0.0.1:80", "[::1]", "[::1]:80", "localhost", "localhost:80", "LOCALHOST"},
			no:  []string{"::1", "localhost:8080", "127.0.0.1:81"},
		},
		{
			name: "no address",
			b:    Bound{},
			no:   []string{"", "localhost", "localhost:80", "127.0.0.1"},
		},
		{
			name: "a socket",
			b:    Bound{Socket: "/run/user/1000/prog/prog.sock"},
			yes: []string{
				"localhost", "localhost:8080", "127.0.0.1", "127.0.0.1:1", "[::1]", "[::1]:9", "LocalHost:5174",
				"LOCALHOST", "localhost:0", "localhost:65535",
			},
			no: []string{
				"", "example.org", "example.org:80", "localhost.example.org", "localhost:", "localhost:abc", "localhost:99999",
				"localhost:+80", "localhost:8 0", "localhost:80:80", "[localhost]:80", "::1", "[::1", "127.0.0.1:",
				"0.0.0.0", "127.0.0.2", ":80", "localhost:١٢",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, host := range tt.yes {
				if !tt.b.Answers(host) {
					t.Errorf("Answers(%q) = false, want true", host)
				}
			}
			for _, host := range tt.no {
				if tt.b.Answers(host) {
					t.Errorf("Answers(%q) = true, want false", host)
				}
			}
		})
	}
}

func TestHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		b    Bound
		want string
	}{
		{name: "the first address", b: Bound{Addrs: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:80"), netip.MustParseAddrPort("[::1]:80")}}, want: "127.0.0.1:80"},
		{name: "IPv6", b: Bound{Addrs: []netip.AddrPort{netip.MustParseAddrPort("[::1]:8080")}}, want: "[::1]:8080"},
		{name: "a socket", b: Bound{Socket: "/run/x.sock"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.b.Host(); got != tt.want {
				t.Errorf("Host() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCloseJoinsTheErrors(t *testing.T) {
	t.Parallel()

	e1, e2 := errors.New("first"), errors.New("second")
	a := &fakeListener{closeErr: e1}
	b := &fakeListener{}
	c := &fakeListener{closeErr: e2}

	err := (&Bound{Listeners: []net.Listener{a, b, c}}).Close()

	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Errorf("Close() = %v, want both errors", err)
	}
	for i, l := range []*fakeListener{a, b, c} {
		if !l.isClosed() {
			t.Errorf("listener %d is not closed", i)
		}
	}
	if err := (&Bound{Listeners: []net.Listener{&fakeListener{}}}).Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if err := (&Bound{}).Close(); err != nil {
		t.Errorf("Close() of nothing = %v, want nil", err)
	}
}
