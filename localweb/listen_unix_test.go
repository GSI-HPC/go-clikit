// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package localweb

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
)

// fakeListener is a listener that holds an address and records its closing.
// step is what one call of a fakeNet's bind does: it fails with err, or
// returns a listener at addr (raw, when that is set).
type step struct {
	addr string
	raw  net.Addr
	err  error
}

// fakeNet is a Bind that follows a script, one step for each call.
type fakeNet struct {
	t     *testing.T
	steps []step

	mu        sync.Mutex
	calls     []string
	listeners []*fakeListener
}

func (f *fakeNet) bind(_ context.Context, network, address string) (net.Listener, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if network != "tcp" {
		f.t.Errorf("bind network = %q, want tcp", network)
	}
	n := len(f.calls)
	f.calls = append(f.calls, address)
	if n >= len(f.steps) {
		f.t.Errorf("unexpected bind call %d for %s", n, address)
		return nil, errors.New("unexpected call")
	}
	s := f.steps[n]
	if s.err != nil {
		return nil, s.err
	}
	l := &fakeListener{addr: s.raw}
	if l.addr == nil {
		l.addr = net.TCPAddrFromAddrPort(netip.MustParseAddrPort(s.addr))
	}
	f.listeners = append(f.listeners, l)
	return l, nil
}

// opErr wraps errno as net does for a failed bind.
func opErr(errno syscall.Errno) error {
	return &net.OpError{Op: "listen", Net: "tcp", Err: os.NewSyscallError("bind", errno)}
}

func TestListenRefusesAnUnacceptableAddressBeforeBinding(t *testing.T) {
	t.Parallel()

	f := &fakeNet{t: t}

	b, err := Listen(t.Context(), "0.0.0.0:80", ListenOptions{Bind: f.bind})

	if err == nil || b != nil {
		t.Fatalf("Listen = %v, %v, want an error", b, err)
	}
	if len(f.calls) != 0 {
		t.Errorf("bound %v, want nothing", f.calls)
	}
}

func TestListenLiteral(t *testing.T) {
	t.Parallel()

	inUse := opErr(syscall.EADDRINUSE)
	other := errors.New("boom")
	tests := []struct {
		name      string
		addr      string
		anyPort   bool
		steps     []step
		wantCalls []string
		wantAddrs []string
		wantErr   string
		wantIs    error
	}{
		{
			name:      "IPv4",
			addr:      "127.0.0.1:7000",
			steps:     []step{{addr: "127.0.0.1:7000"}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantAddrs: []string{"127.0.0.1:7000"},
		},
		{
			name:      "IPv6",
			addr:      "[::1]:7000",
			steps:     []step{{addr: "[::1]:7000"}},
			wantCalls: []string{"[::1]:7000"},
			wantAddrs: []string{"[::1]:7000"},
		},
		{
			name:      "an IPv4 address mapped into IPv6 is IPv4",
			addr:      "127.0.0.1:80",
			steps:     []step{{raw: &net.TCPAddr{IP: net.ParseIP("::ffff:127.0.0.1"), Port: 80}}},
			wantCalls: []string{"127.0.0.1:80"},
			wantAddrs: []string{"127.0.0.1:80"},
		},
		{
			name:      "a port in use is an error",
			addr:      "127.0.0.1:7000",
			steps:     []step{{err: inUse}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantErr:   "open the listener: ",
			wantIs:    syscall.EADDRINUSE,
		},
		{
			name:      "a port in use gives way to a free one",
			addr:      "127.0.0.1:7000",
			anyPort:   true,
			steps:     []step{{err: inUse}, {addr: "127.0.0.1:7001"}},
			wantCalls: []string{"127.0.0.1:7000", "127.0.0.1:0"},
			wantAddrs: []string{"127.0.0.1:7001"},
		},
		{
			name:      "no free port either",
			addr:      "127.0.0.1:7000",
			anyPort:   true,
			steps:     []step{{err: inUse}, {err: other}},
			wantCalls: []string{"127.0.0.1:7000", "127.0.0.1:0"},
			wantErr:   "open the listener on a free port: ",
			wantIs:    other,
		},
		{
			name:      "another error is not met with another port",
			addr:      "127.0.0.1:7000",
			anyPort:   true,
			steps:     []step{{err: other}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantErr:   "open the listener: ",
			wantIs:    other,
		},
		{
			name:      "bound on a network interface",
			addr:      "127.0.0.1:7000",
			steps:     []step{{addr: "192.0.2.10:7000"}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantErr:   "the listener is bound to 192.0.2.10, which is not a loopback address",
		},
		{
			name:      "bound on every interface",
			addr:      "127.0.0.1:7000",
			steps:     []step{{addr: "0.0.0.0:7000"}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantErr:   "the listener is bound to 0.0.0.0, which is not a loopback address",
		},
		{
			name:      "bound on a socket",
			addr:      "127.0.0.1:7000",
			steps:     []step{{raw: &net.UnixAddr{Name: "/run/x.sock", Net: "unix"}}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantErr:   "the listener is bound to /run/x.sock, which is not a TCP address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeNet{t: t, steps: tt.steps}

			b, err := Listen(t.Context(), tt.addr, ListenOptions{AnyPort: tt.anyPort, Bind: f.bind})

			checkCalls(t, f, tt.wantCalls)
			checkListen(t, b, err, tt.wantAddrs, tt.wantErr, tt.wantIs)
			if tt.wantErr != "" {
				for i, l := range f.listeners {
					if !l.isClosed() {
						t.Errorf("listener %d is left open", i)
					}
				}
			}
		})
	}
}

func TestListenLocalhost(t *testing.T) {
	t.Parallel()

	inUse := opErr(syscall.EADDRINUSE)
	other := errors.New("boom")
	var busy []step
	for range 1 + portTries {
		busy = append(busy, step{err: inUse})
	}
	busyCalls := []string{"127.0.0.1:7000"}
	for range portTries {
		busyCalls = append(busyCalls, "127.0.0.1:0")
	}
	tests := []struct {
		name      string
		anyPort   bool
		steps     []step
		wantCalls []string
		wantAddrs []string
		wantOpen  []bool // which listeners stay open, in order made
		wantErr   string
		wantIs    error
	}{
		{
			name:      "both addresses on one port",
			steps:     []step{{addr: "127.0.0.1:5000"}, {addr: "[::1]:5000"}},
			wantCalls: []string{"127.0.0.1:7000", "[::1]:5000"},
			wantAddrs: []string{"127.0.0.1:5000", "[::1]:5000"},
		},
		{
			name:      "no IPv6 loopback address",
			steps:     []step{{addr: "127.0.0.1:5000"}, {err: opErr(syscall.EADDRNOTAVAIL)}},
			wantCalls: []string{"127.0.0.1:7000", "[::1]:5000"},
			wantAddrs: []string{"127.0.0.1:5000"},
		},
		{
			name:      "no IPv6 at all",
			steps:     []step{{addr: "127.0.0.1:5000"}, {err: opErr(syscall.EAFNOSUPPORT)}},
			wantCalls: []string{"127.0.0.1:7000", "[::1]:5000"},
			wantAddrs: []string{"127.0.0.1:5000"},
		},
		{
			name:      "the port is taken on ::1",
			steps:     []step{{addr: "127.0.0.1:5000"}, {err: inUse}},
			wantCalls: []string{"127.0.0.1:7000", "[::1]:5000"},
			wantErr:   "open the listener: ",
			wantIs:    syscall.EADDRINUSE,
		},
		{
			name:      "another error on ::1",
			anyPort:   true,
			steps:     []step{{addr: "127.0.0.1:5000"}, {err: other}},
			wantCalls: []string{"127.0.0.1:7000", "[::1]:5000"},
			wantErr:   "open the listener: ",
			wantIs:    other,
		},
		{
			name:      "the port is taken on 127.0.0.1",
			steps:     []step{{err: inUse}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantErr:   "open the listener: ",
			wantIs:    syscall.EADDRINUSE,
		},
		{
			name:      "another error on 127.0.0.1",
			anyPort:   true,
			steps:     []step{{err: other}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantErr:   "open the listener: ",
			wantIs:    other,
		},
		{
			name:    "a port taken on ::1 gives way to one free on both",
			anyPort: true,
			steps: []step{
				{addr: "127.0.0.1:5000"}, {err: inUse},
				{addr: "127.0.0.1:6000"}, {addr: "[::1]:6000"},
			},
			wantCalls: []string{"127.0.0.1:7000", "[::1]:5000", "127.0.0.1:0", "[::1]:6000"},
			wantAddrs: []string{"127.0.0.1:6000", "[::1]:6000"},
		},
		{
			name:    "a port taken on 127.0.0.1 gives way to one free on both",
			anyPort: true,
			steps: []step{
				{err: inUse},
				{addr: "127.0.0.1:6000"}, {addr: "[::1]:6000"},
			},
			wantCalls: []string{"127.0.0.1:7000", "127.0.0.1:0", "[::1]:6000"},
			wantAddrs: []string{"127.0.0.1:6000", "[::1]:6000"},
		},
		{
			name:    "the second try fails otherwise",
			anyPort: true,
			steps: []step{
				{err: inUse},
				{err: other},
			},
			wantCalls: []string{"127.0.0.1:7000", "127.0.0.1:0"},
			wantErr:   "open the listener on a free port: ",
			wantIs:    other,
		},
		{
			name:      "no free port in the tries",
			anyPort:   true,
			steps:     busy,
			wantCalls: busyCalls,
			wantErr:   "open the listener: no port was free on both loopback addresses in 16 tries: ",
			wantIs:    syscall.EADDRINUSE,
		},
		{
			name:      "the address of a listener is no host and port",
			steps:     []step{{raw: &net.UnixAddr{Name: "x", Net: "unix"}}},
			wantCalls: []string{"127.0.0.1:7000"},
			wantErr:   "open the listener: address x: missing port in address",
		},
		{
			name:      "bound on a network interface",
			steps:     []step{{addr: "192.0.2.10:5000"}, {addr: "[::1]:5000"}},
			wantCalls: []string{"127.0.0.1:7000", "[::1]:5000"},
			wantErr:   "the listener is bound to 192.0.2.10, which is not a loopback address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeNet{t: t, steps: tt.steps}

			b, err := Listen(t.Context(), "localhost:7000", ListenOptions{AnyPort: tt.anyPort, Bind: f.bind})

			checkCalls(t, f, tt.wantCalls)
			checkListen(t, b, err, tt.wantAddrs, tt.wantErr, tt.wantIs)
			if tt.wantErr != "" {
				for i, l := range f.listeners {
					if !l.isClosed() {
						t.Errorf("listener %d is left open", i)
					}
				}
			}
		})
	}
}

func checkCalls(t *testing.T, f *fakeNet, want []string) {
	t.Helper()
	if strings.Join(f.calls, " ") != strings.Join(want, " ") {
		t.Errorf("bound %q, want %q", f.calls, want)
	}
}

func checkListen(t *testing.T, b *Bound, err error, wantAddrs []string, wantErr string, wantIs error) {
	t.Helper()
	if wantErr != "" {
		if err == nil || b != nil {
			t.Fatalf("Listen = %v, %v, want an error", b, err)
		}
		if !strings.HasPrefix(err.Error(), wantErr) {
			t.Errorf("error = %q, want it to start %q", err, wantErr)
		}
		if wantIs != nil && !errors.Is(err, wantIs) {
			t.Errorf("error = %v, want it to wrap %v", err, wantIs)
		}
		return
	}
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	var got []string
	for _, a := range b.Addrs {
		got = append(got, a.String())
	}
	if strings.Join(got, " ") != strings.Join(wantAddrs, " ") {
		t.Errorf("Addrs = %q, want %q", got, wantAddrs)
	}
	if len(b.Listeners) != len(wantAddrs) {
		t.Errorf("%d listeners, want %d", len(b.Listeners), len(wantAddrs))
	}
	if b.Socket != "" {
		t.Errorf("Socket = %q, want none", b.Socket)
	}
}

func TestListenOnThisMachine(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"127.0.0.1:0", "localhost:0"} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()

			b, err := Listen(t.Context(), addr, ListenOptions{})
			if err != nil {
				t.Fatalf("Listen: %v", err)
			}
			defer func() { _ = b.Close() }()

			if len(b.Addrs) == 0 || len(b.Addrs) != len(b.Listeners) {
				t.Fatalf("Addrs = %v for %d listeners", b.Addrs, len(b.Listeners))
			}
			for i, a := range b.Addrs {
				if !a.Addr().IsLoopback() || a.Port() == 0 {
					t.Errorf("Addrs[%d] = %v, want a loopback address and a port", i, a)
				}
				if a.Port() != b.Addrs[0].Port() {
					t.Errorf("Addrs = %v, want one port", b.Addrs)
				}
			}
			if got, want := b.Host(), b.Addrs[0].String(); got != want {
				t.Errorf("Host() = %q, want %q", got, want)
			}
			if !b.Answers(b.Host()) {
				t.Errorf("Answers(%q) = false", b.Host())
			}
		})
	}
}

func TestListenWithAPortInUse(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"127.0.0.1", "localhost"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			held, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("hold a port: %v", err)
			}
			defer func() { _ = held.Close() }()
			port := held.Addr().(*net.TCPAddr).Port
			addr := net.JoinHostPort(name, strconv.Itoa(port))

			b, err := Listen(t.Context(), addr, ListenOptions{})
			if err == nil {
				_ = b.Close()
				t.Fatalf("Listen(%q) succeeded on a port in use", addr)
			}
			if !errors.Is(err, syscall.EADDRINUSE) {
				t.Errorf("error = %v, want it to wrap EADDRINUSE", err)
			}

			b, err = Listen(t.Context(), addr, ListenOptions{AnyPort: true})
			if err != nil {
				t.Fatalf("Listen(%q) with AnyPort: %v", addr, err)
			}
			defer func() { _ = b.Close() }()
			if got := int(b.Addrs[0].Port()); got == port || got == 0 {
				t.Errorf("port = %d, want a free one other than %d", got, port)
			}
		})
	}
}

func TestAddrInUseAndNoSuchAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err           error
		inUse, noSuch bool
	}{
		{err: opErr(syscall.EADDRINUSE), inUse: true},
		{err: opErr(syscall.EADDRNOTAVAIL), noSuch: true},
		{err: opErr(syscall.EAFNOSUPPORT), noSuch: true},
		{err: opErr(syscall.EACCES)},
		{err: errors.New("other")},
	}
	for _, tt := range tests {
		if got := addrInUse(tt.err); got != tt.inUse {
			t.Errorf("addrInUse(%v) = %v, want %v", tt.err, got, tt.inUse)
		}
		if got := noSuchAddress(tt.err); got != tt.noSuch {
			t.Errorf("noSuchAddress(%v) = %v, want %v", tt.err, got, tt.noSuch)
		}
	}
}
