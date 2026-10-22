// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// UnixPrefix begins an address that names a Unix socket by its path, in
// place of a host and a port: "unix:/run/user/1000/prog/prog.sock".
const UnixPrefix = "unix:"

// portTries bounds the search for a port that is free on both loopback
// addresses. The system hands out a port that is free on 127.0.0.1, and
// only a listener on that very number on ::1 makes a try fail; the bound
// turns a machine on which something is badly wrong into an error and not
// a loop.
const portTries = 16

// CheckAddr reports why addr cannot be the address of a local server, or
// returns nil. It takes host:port with a numeric port, where the host is
// localhost or a loopback IP address, or UnixPrefix and the absolute path
// of a socket that fits a socket address.
//
// Anything else is refused before it is bound. An empty host or 0.0.0.0
// would offer the user's identity to the network, and a name other than
// localhost may resolve to another address tomorrow.
func CheckAddr(addr string) error {
	if path, ok := strings.CutPrefix(addr, UnixPrefix); ok {
		return checkSocketPath(path)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%q is not host:port: %w", addr, err)
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("%q has no port number between 0 and 65535", addr)
	}
	if host == "localhost" {
		return nil
	}
	if ip, err := netip.ParseAddr(host); err != nil || !ip.IsLoopback() {
		return fmt.Errorf("%q is not a loopback address", addr)
	}
	return nil
}

// ListenOptions configures Listen.
type ListenOptions struct {
	// AnyPort binds a free port when the port asked for is in use, on the
	// same addresses, so that a second instance, or anything else on the
	// port, does not keep the program from starting. The program compares
	// the port of Bound.Addrs with the one it asked for and says so.
	// Without it, a port in use is an error.
	AnyPort bool
	// Bind opens a listener, as net.ListenConfig.Listen does; nil binds on
	// this machine. A test stands in with it for a machine that the one it
	// runs on is not: one without an IPv6 loopback address, or one on
	// which a port is taken.
	Bind func(ctx context.Context, network, address string) (net.Listener, error)
}

// Bound is what Listen bound, for one handler to serve.
type Bound struct {
	// Listeners are the listeners to serve: two for the name localhost,
	// on 127.0.0.1 and ::1, and one otherwise.
	Listeners []net.Listener
	// Addrs are the TCP addresses of the listeners, in their order; a
	// Unix socket has none.
	Addrs []netip.AddrPort
	// Socket is the path of the Unix socket, with the symbolic links on
	// the way to its directory resolved, or "" on TCP.
	Socket string
}

// Listen binds addr, which CheckAddr must accept, and returns what it
// bound: the socket of a Unix address, both loopback addresses for the
// name localhost, or the one address of a literal. Everything is bound
// when Listen returns, so the program can print the address before it
// serves: a browser that is faster waits in the listener's queue. ctx
// bounds the binding only.
//
// The name localhost is bound as the two literals, and no resolver is
// asked: the hosts file, or DNS with its search domains, could give the
// name another address. Both are bound on one port number, because that
// is what lets the server answer to the name: a browser tries ::1 before
// 127.0.0.1, and a port held on one of them alone can be held by another
// user's process on the other (Bound.Answers). A machine without an IPv6
// loopback address is served on 127.0.0.1 alone.
//
// A Unix socket is for a machine that other users share. Nobody but the
// user may connect to it, and it lies where nobody else can replace it: in
// a directory of the user's that nobody else may write to, below
// directories that only the user or the system may change. Listen makes
// the directory if it is missing, and refuses the address otherwise.
// Listen replaces a socket that nobody answers on, which a killed server
// leaves behind, and refuses one that answers. The socket is bound under
// a temporary name, given mode 0600 and renamed into its place, so it is
// never open to others; it is removed when its listener closes. A browser
// cannot open a socket: the user puts a forward in front of it. Unix
// sockets are not supported on systems that are not Unix.
func Listen(ctx context.Context, addr string, o ListenOptions) (*Bound, error) {
	if err := CheckAddr(addr); err != nil {
		return nil, err
	}
	bind := o.Bind
	if bind == nil {
		bind = systemBind
	}
	if path, ok := strings.CutPrefix(addr, UnixPrefix); ok {
		l, path, err := listenSocket(ctx, bind, path)
		if err != nil {
			return nil, err
		}
		return &Bound{Listeners: []net.Listener{l}, Socket: path}, nil
	}

	// An address that CheckAddr accepts splits.
	host, port, _ := net.SplitHostPort(addr)
	var ls []net.Listener
	var err error
	if host == "localhost" {
		ls, err = listenLocalhost(ctx, bind, port, o.AnyPort)
	} else {
		ls, err = listenLiteral(ctx, bind, host, port, o.AnyPort)
	}
	if err != nil {
		return nil, err
	}
	b := &Bound{Listeners: ls}
	for _, l := range ls {
		a, err := loopbackAddr(l.Addr())
		if err != nil {
			_ = b.Close()
			return nil, err
		}
		b.Addrs = append(b.Addrs, a)
	}
	return b, nil
}

// systemBind binds on this machine.
func systemBind(ctx context.Context, network, address string) (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(ctx, network, address)
}

// loopbackAddr returns the address a listener is bound to, or an error when
// that is anything but a TCP port on a loopback address. CheckAddr refuses
// every other address before it is bound; this holds what was bound to the
// same rule, whatever the system or a Bind made of the request.
func loopbackAddr(a net.Addr) (netip.AddrPort, error) {
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("the listener is bound to %s, which is not a TCP address", a)
	}
	ap := tcp.AddrPort()
	if !ap.Addr().Unmap().IsLoopback() {
		return netip.AddrPort{}, fmt.Errorf("the listener is bound to %s, which is not a loopback address", ap.Addr())
	}
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port()), nil
}

// listenLiteral binds port on host, a loopback IP address, and nothing
// beside it. With anyPort, a port in use gives way to a free one.
func listenLiteral(ctx context.Context, bind bindFunc, host, port string, anyPort bool) ([]net.Listener, error) {
	l, err := bind(ctx, "tcp", net.JoinHostPort(host, port))
	if err == nil {
		return []net.Listener{l}, nil
	}
	if !anyPort || !addrInUse(err) {
		return nil, fmt.Errorf("open the listener: %w", err)
	}
	l, err = bind(ctx, "tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return nil, fmt.Errorf("open the listener on a free port: %w", err)
	}
	return []net.Listener{l}, nil
}

// listenLocalhost binds port on 127.0.0.1 and ::1. With anyPort, a port in
// use on either gives way to one that is free on both.
func listenLocalhost(ctx context.Context, bind bindFunc, port string, anyPort bool) ([]net.Listener, error) {
	ls, err := bindLoopback(ctx, bind, port)
	if err == nil {
		return ls, nil
	}
	if !anyPort || !addrInUse(err) {
		return nil, fmt.Errorf("open the listener: %w", err)
	}
	for range portTries {
		ls, err = bindLoopback(ctx, bind, "0")
		if err == nil {
			return ls, nil
		}
		if !addrInUse(err) {
			return nil, fmt.Errorf("open the listener on a free port: %w", err)
		}
	}
	return nil, fmt.Errorf("open the listener: no port was free on both loopback addresses in %d tries: %w", portTries, err)
}

// bindLoopback binds port on 127.0.0.1 and then the same number on ::1; for
// port 0 that is the number the system chose for the first. It returns both
// listeners, or the first alone when the machine has no IPv6 loopback
// address. When ::1 cannot be had for any other reason, the port in use
// among them, the first is closed and the error returned: half of
// localhost on a port is not what was asked for.
func bindLoopback(ctx context.Context, bind bindFunc, port string) ([]net.Listener, error) {
	v4, err := bind(ctx, "tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return nil, err
	}
	_, number, err := net.SplitHostPort(v4.Addr().String())
	if err == nil {
		var v6 net.Listener
		v6, err = bind(ctx, "tcp", net.JoinHostPort("::1", number))
		if err == nil {
			return []net.Listener{v4, v6}, nil
		}
		if noSuchAddress(err) {
			return []net.Listener{v4}, nil
		}
	}
	_ = v4.Close()
	return nil, err
}

// bindFunc is the type of ListenOptions.Bind.
type bindFunc = func(ctx context.Context, network, address string) (net.Listener, error)

// Host returns the host and port of the address a browser opens: the
// first address bound, 127.0.0.1 for the name localhost. A Unix socket has
// none, and Host returns "": the program names the near end of the
// forward that the user puts in front of it.
func (b *Bound) Host() string {
	if len(b.Addrs) == 0 {
		return ""
	}
	return b.Addrs[0].String()
}

// Answers reports whether host, the Host header of a request, names this
// server, for Hosts. On TCP the names are each bound address with its
// port, and the name localhost with that port once Listen holds the port
// on both 127.0.0.1 and ::1. With one of them held, neither the name nor
// the other address is answered, although both lead to this machine: a
// browser asked for localhost tries ::1 first, and on a shared machine
// another user's process can hold the port there, would be handed the
// launch token, and would share an origin with this server.
//
// On a Unix socket the browser connects to the near end of a forward, on
// a port the server never learns, and names that, so the names are
// localhost, 127.0.0.1 and [::1] with any port or none. A page that
// reaches either by DNS rebinding names its own domain and is not
// answered.
//
// Case does not count, as in a host name.
func (b *Bound) Answers(host string) bool {
	host = strings.ToLower(host)
	if b.Socket != "" {
		return forwardedHost(host)
	}
	var v4, v6 netip.AddrPort
	for _, a := range b.Addrs {
		if names(host, hostOnly(a.Addr()), a.Port()) {
			return true
		}
		switch a.Addr() {
		case netip.AddrFrom4([4]byte{127, 0, 0, 1}):
			v4 = a
		case netip.IPv6Loopback():
			v6 = a
		}
	}
	return v4.IsValid() && v6.IsValid() && v4.Port() == v6.Port() && names(host, "localhost", v4.Port())
}

// names reports whether host is name with port, as a Host header writes
// it: without the port when it is 80, the default of HTTP.
func names(host, name string, port uint16) bool {
	return host == name+":"+strconv.Itoa(int(port)) || port == 80 && host == name
}

// hostOnly returns a as a Host header writes it, in brackets for IPv6.
func hostOnly(a netip.Addr) string {
	if a.Is6() {
		return "[" + a.String() + "]"
	}
	return a.String()
}

// forwardedHost reports whether host, in lower case, is a name of the
// loopback interface, with a port or without.
func forwardedHost(host string) bool {
	name := host
	if h, port, err := net.SplitHostPort(host); err == nil {
		// Joined again, the two must be the header as it came:
		// SplitHostPort also takes brackets around a name and an empty
		// port, which no browser writes.
		if !isPort(port) || net.JoinHostPort(h, port) != host {
			return false
		}
		name = strings.TrimSuffix(host, ":"+port)
	}
	return name == "localhost" || name == "127.0.0.1" || name == "[::1]"
}

// isPort reports whether port is a port number as a Host header writes it:
// decimal digits and nothing else. localweb/session names its cookie after
// it.
func isPort(port string) bool {
	if port == "" || strings.Trim(port, "0123456789") != "" {
		return false
	}
	_, err := strconv.ParseUint(port, 10, 16)
	return err == nil
}

// Close closes every listener and returns their errors joined.
func (b *Bound) Close() error {
	var errs []error
	for _, l := range b.Listeners {
		errs = append(errs, l.Close())
	}
	return errors.Join(errs...)
}
