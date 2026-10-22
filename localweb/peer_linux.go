// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
func PeerListener(l net.Listener, uid int, refused func(error)) (net.Listener, error) {
	return &peerListener{Listener: l, uid: uid, owner: procOwner("/proc", tcpEstablished), refused: refused}, nil
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
func ListenerOwner(addr netip.AddrPort) (int, error) {
	return procOwner("/proc", tcpListen)(addr, netip.AddrPort{})
}

// The states of a socket in /proc/net/tcp.
const (
	tcpEstablished = "01"
	tcpListen      = "0A"
)

// procOwner returns a lookup of the owner of a socket in state in the
// tables net/tcp and net/tcp6 below procRoot, which is /proc on a host. An
// IPv4 socket is in net/tcp, or in net/tcp6 when it is one of both
// families; an IPv6 one is in net/tcp6.
func procOwner(procRoot, state string) func(local, remote netip.AddrPort) (int, error) {
	return func(local, remote netip.AddrPort) (int, error) {
		var errs []error
		for _, table := range []string{"tcp", "tcp6"} {
			if table == "tcp" && !local.Addr().Is4() {
				continue
			}
			uid, err := tableOwner(filepath.Join(procRoot, "net", table), state, local, remote)
			if err == nil {
				return uid, nil
			}
			errs = append(errs, err)
		}
		return 0, errors.Join(errs...)
	}
}

// tableOwner returns the owner of the socket in state at local connected
// to remote in the table at path.
func tableOwner(path, state string, local, remote netip.AddrPort) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	uid, err := socketOwner(f, state, local, remote)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	return uid, nil
}

// socketOwner reads a table in the format of /proc/net/tcp or tcp6 and
// returns the uid of the socket in state that is bound to local and
// connected to remote; a listening socket is connected to nothing, and
// remote is not looked at. Only a socket with an inode counts: one in
// TIME_WAIT, or one that no process has accepted yet, has none, and its uid
// is not that of a process.
func socketOwner(r io.Reader, state string, local, remote netip.AddrPort) (int, error) {
	sc := bufio.NewScanner(r)
	sc.Scan() // the header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 10 || f[3] != state || f[9] == "0" {
			continue
		}
		l, err1 := parseProcAddr(f[1])
		r, err2 := parseProcAddr(f[2])
		if err1 != nil || err2 != nil || l != local || state != tcpListen && r != remote {
			continue
		}
		return strconv.Atoi(f[7])
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("no such socket")
}

// parseProcAddr parses an address and port as /proc/net/tcp and tcp6
// write them: the address as 32-bit words in host byte order, one for IPv4
// and four for IPv6, the port in big-endian hex, such as 0100007F:1F90 for
// 127.0.0.1:8080 on x86. An IPv4 address mapped into IPv6 is returned as
// IPv4.
func parseProcAddr(s string) (netip.AddrPort, error) {
	ip, port, ok := strings.Cut(s, ":")
	b, err := hex.DecodeString(ip)
	if !ok || err != nil || len(b) != 4 && len(b) != 16 {
		return netip.AddrPort{}, fmt.Errorf("bad address %q", s)
	}
	p, err := strconv.ParseUint(port, 16, 16)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("bad port in %q", s)
	}
	for i := 0; i < len(b); i += 4 {
		binary.BigEndian.PutUint32(b[i:], binary.NativeEndian.Uint32(b[i:]))
	}
	a, _ := netip.AddrFromSlice(b)
	return netip.AddrPortFrom(a.Unmap(), uint16(p)), nil
}
