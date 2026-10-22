// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package localweb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

// procAddr writes an address as the kernel does in /proc/net/tcp and
// tcp6: 32-bit words in host byte order, the port in big-endian hex. as6
// writes an IPv4 address as the IPv6 address it is mapped to.
func procAddr(ap netip.AddrPort, as6 bool) string {
	a := ap.Addr()
	var b []byte
	if a.Is4() && !as6 {
		a4 := a.As4()
		b = a4[:]
	} else {
		a16 := a.As16()
		b = a16[:]
	}
	var s strings.Builder
	for i := 0; i < len(b); i += 4 {
		fmt.Fprintf(&s, "%08X", binary.NativeEndian.Uint32(b[i:]))
	}
	return fmt.Sprintf("%s:%04X", s.String(), ap.Port())
}

const procHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

// procRow writes a row of the table.
func procRow(local, remote, state string, uid int, inode string) string {
	return fmt.Sprintf("   0: %s %s %s 00000000:00000000 00:00000000 00000000 %5d        0 %s 1 0000000000000000 100 0 0 10 0\n",
		local, remote, state, uid, inode)
}

func TestProcAddrIsWhatTheKernelWrites(t *testing.T) {
	t.Parallel()

	// 127.0.0.1:8080 on x86, as the documentation of the table has it.
	if binary.NativeEndian.Uint16([]byte{1, 0}) == 1 {
		if got := procAddr(netip.MustParseAddrPort("127.0.0.1:8080"), false); got != "0100007F:1F90" {
			t.Errorf("procAddr = %q, want 0100007F:1F90", got)
		}
	}
}

func TestParseProcAddr(t *testing.T) {
	t.Parallel()

	v4 := netip.MustParseAddrPort("127.0.0.1:8080")
	v6 := netip.MustParseAddrPort("[::1]:443")
	v6b := netip.MustParseAddrPort("[2001:db8::1:2]:65535")
	tests := []struct {
		name string
		in   string
		want netip.AddrPort
		err  string
	}{
		{name: "IPv4", in: procAddr(v4, false), want: v4},
		{name: "IPv6 loopback", in: procAddr(v6, false), want: v6},
		{name: "IPv6", in: procAddr(v6b, false), want: v6b},
		{name: "IPv4 mapped into IPv6", in: procAddr(v4, true), want: v4},
		{name: "any address", in: "00000000:0000", want: netip.MustParseAddrPort("0.0.0.0:0")},
		{name: "no port", in: "0100007F", err: `bad address "0100007F"`},
		{name: "not hex", in: "ZZZZZZZZ:1F90", err: `bad address "ZZZZZZZZ:1F90"`},
		{name: "odd hex", in: "0100007:1F90", err: `bad address "0100007:1F90"`},
		{name: "three bytes", in: "010000:1F90", err: `bad address "010000:1F90"`},
		{name: "empty", in: "", err: `bad address ""`},
		{name: "bad port", in: "0100007F:ZZ", err: `bad port in "0100007F:ZZ"`},
		{name: "empty port", in: "0100007F:", err: `bad port in "0100007F:"`},
		{name: "port too large", in: "0100007F:10000", err: `bad port in "0100007F:10000"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseProcAddr(tt.in)

			if tt.err != "" {
				if err == nil || err.Error() != tt.err {
					t.Errorf("parseProcAddr(%q) = %v, %v, want the error %q", tt.in, got, err, tt.err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("parseProcAddr(%q) = %v, %v, want %v", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestSocketOwner(t *testing.T) {
	t.Parallel()

	server := netip.MustParseAddrPort("127.0.0.1:7374")
	client := netip.MustParseAddrPort("127.0.0.1:50000")
	other := netip.MustParseAddrPort("127.0.0.1:50001")
	s, c, o := procAddr(server, false), procAddr(client, false), procAddr(other, false)
	listening := procRow(s, procAddr(netip.MustParseAddrPort("0.0.0.0:0"), false), "0A", 0, "111")
	tests := []struct {
		name string
		in   string
		uid  int
		err  string
	}{
		{name: "the client's socket", in: procHeader + listening + procRow(c, s, "01", 1000, "222"), uid: 1000},
		{name: "after others", in: procHeader + procRow(o, s, "01", 1001, "223") + procRow(c, s, "01", 1000, "222"), uid: 1000},
		{name: "the first of two", in: procHeader + procRow(c, s, "01", 1000, "222") + procRow(c, s, "01", 1002, "224"), uid: 1000},
		{name: "root's", in: procHeader + procRow(c, s, "01", 0, "222"), uid: 0},
		{name: "another connection", in: procHeader + procRow(o, s, "01", 1001, "223"), err: "no such socket"},
		{name: "the other direction", in: procHeader + procRow(s, c, "01", 1000, "222"), err: "no such socket"},
		{name: "another remote port", in: procHeader + procRow(c, o, "01", 1000, "222"), err: "no such socket"},
		{name: "TIME_WAIT", in: procHeader + procRow(c, s, "06", 0, "0"), err: "no such socket"},
		{name: "not established", in: procHeader + procRow(c, s, "02", 1000, "222"), err: "no such socket"},
		{name: "no inode", in: procHeader + procRow(c, s, "01", 0, "0"), err: "no such socket"},
		{name: "a short row", in: procHeader + "   0: " + c + " " + s + " 01\n", err: "no such socket"},
		{name: "a bad local address", in: procHeader + procRow("nonsense", s, "01", 1000, "222"), err: "no such socket"},
		{name: "a bad remote address", in: procHeader + procRow(c, "0100007F:ZZ", "01", 1000, "222"), err: "no such socket"},
		{name: "a bad row before the right one", in: procHeader + procRow("x", "y", "01", 1, "2") + procRow(c, s, "01", 1000, "222"), uid: 1000},
		{name: "an owner that is no number", in: procHeader + strings.Replace(procRow(c, s, "01", 1000, "222"), "  1000", "  none", 1), err: "strconv.Atoi"},
		{name: "a header alone", in: procHeader, err: "no such socket"},
		{name: "nothing", in: "", err: "no such socket"},
		{name: "a line too long", in: procHeader + strings.Repeat("x", 1<<20) + "\n", err: "token too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uid, err := socketOwner(strings.NewReader(tt.in), tcpEstablished, client, server)

			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Errorf("socketOwner = %d, %v, want an error with %q", uid, err, tt.err)
				}
				return
			}
			if err != nil || uid != tt.uid {
				t.Errorf("socketOwner = %d, %v, want %d", uid, err, tt.uid)
			}
		})
	}

	t.Run("a table that cannot be read", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		r := io.MultiReader(strings.NewReader(procHeader), iotest.ErrReader(boom))

		if _, err := socketOwner(r, tcpEstablished, client, server); !errors.Is(err, boom) {
			t.Errorf("socketOwner = %v, want boom", err)
		}
	})
}

func TestSocketOwnerOfAListener(t *testing.T) {
	t.Parallel()

	addr := netip.MustParseAddrPort("127.0.0.1:7374")
	a := procAddr(addr, false)
	anyRemote := procAddr(netip.MustParseAddrPort("0.0.0.0:0"), false)
	tests := []struct {
		name string
		in   string
		uid  int
		err  string
	}{
		{name: "the listener", in: procHeader + procRow(a, anyRemote, "0A", 1000, "111"), uid: 1000},
		{name: "after a connection to it", in: procHeader + procRow(a, procAddr(netip.MustParseAddrPort("127.0.0.1:50000"), false), "01", 1001, "222") +
			procRow(a, anyRemote, "0A", 1000, "111"), uid: 1000},
		{name: "on another port", in: procHeader + procRow(procAddr(netip.MustParseAddrPort("127.0.0.1:7375"), false), anyRemote, "0A", 1000, "111"),
			err: "no such socket"},
		{name: "on the any address", in: procHeader + procRow(procAddr(netip.MustParseAddrPort("0.0.0.0:7374"), false), anyRemote, "0A", 1000, "111"),
			err: "no such socket"},
		{name: "no inode", in: procHeader + procRow(a, anyRemote, "0A", 1000, "0"), err: "no such socket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uid, err := socketOwner(strings.NewReader(tt.in), tcpListen, addr, netip.AddrPort{})

			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Errorf("socketOwner = %d, %v, want an error with %q", uid, err, tt.err)
				}
				return
			}
			if err != nil || uid != tt.uid {
				t.Errorf("socketOwner = %d, %v, want %d", uid, err, tt.uid)
			}
		})
	}
}

func TestListenerOwner(t *testing.T) {
	t.Parallel()

	for _, network := range []string{"tcp4", "tcp6"} {
		t.Run(network, func(t *testing.T) {
			t.Parallel()

			host := "127.0.0.1"
			if network == "tcp6" {
				host = "[::1]"
			}
			l, err := net.Listen(network, host+":0")
			if err != nil {
				t.Skip(err)
			}
			defer func() { _ = l.Close() }()
			addr := l.Addr().(*net.TCPAddr).AddrPort()

			uid, err := ListenerOwner(addr)
			if err != nil || uid != os.Getuid() {
				t.Errorf("ListenerOwner(%s) = %d, %v, want %d", addr, uid, err, os.Getuid())
			}

			_ = l.Close()
			if uid, err := ListenerOwner(addr); err == nil {
				t.Errorf("ListenerOwner(%s) after Close = %d, want an error", addr, uid)
			}
		})
	}
}

// writeTable writes the table net/<name> below root.
func writeTable(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, "net")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTableOwner(t *testing.T) {
	t.Parallel()

	server := netip.MustParseAddrPort("127.0.0.1:7374")
	client := netip.MustParseAddrPort("127.0.0.1:50000")
	root := t.TempDir()
	writeTable(t, root, "tcp", procHeader+procRow(procAddr(client, false), procAddr(server, false), "01", 1234, "5"))
	path := filepath.Join(root, "net", "tcp")

	uid, err := tableOwner(path, tcpEstablished, client, server)
	if err != nil || uid != 1234 {
		t.Errorf("tableOwner = %d, %v, want 1234", uid, err)
	}

	_, err = tableOwner(path, tcpEstablished, server, client)
	if err == nil || !strings.HasPrefix(err.Error(), path+": no such socket") {
		t.Errorf("tableOwner of another socket = %v", err)
	}

	_, err = tableOwner(filepath.Join(root, "net", "missing"), tcpEstablished, client, server)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("tableOwner of a missing table = %v", err)
	}
}

func TestProcOwner(t *testing.T) {
	t.Parallel()

	server4 := netip.MustParseAddrPort("127.0.0.1:7374")
	client4 := netip.MustParseAddrPort("127.0.0.1:50000")
	server6 := netip.MustParseAddrPort("[::1]:7374")
	client6 := netip.MustParseAddrPort("[::1]:50000")
	row4 := procRow(procAddr(client4, false), procAddr(server4, false), "01", 1004, "1")
	rowMapped := procRow(procAddr(client4, true), procAddr(server4, true), "01", 1046, "2")
	row6 := procRow(procAddr(client6, false), procAddr(server6, false), "01", 1006, "3")

	tests := []struct {
		name          string
		tcp, tcp6     *string
		local, remote netip.AddrPort
		uid           int
		err           string
	}{
		{name: "IPv4 in tcp", tcp: str(procHeader + row4), tcp6: str(procHeader), local: client4, remote: server4, uid: 1004},
		{name: "IPv4 in tcp, tcp6 missing", tcp: str(procHeader + row4), local: client4, remote: server4, uid: 1004},
		{name: "IPv4 in tcp6", tcp: str(procHeader), tcp6: str(procHeader + rowMapped), local: client4, remote: server4, uid: 1046},
		{name: "IPv4 in tcp6, tcp missing", tcp6: str(procHeader + rowMapped), local: client4, remote: server4, uid: 1046},
		{name: "IPv6 in tcp6", tcp: str(procHeader + row4), tcp6: str(procHeader + row6), local: client6, remote: server6, uid: 1006},
		{name: "IPv6 is not looked for in tcp", tcp: str(procHeader + row6), tcp6: str(procHeader), local: client6, remote: server6, err: "no such socket"},
		{name: "IPv6 with tcp6 missing", tcp: str(procHeader + row6), local: client6, remote: server6, err: "tcp6: "},
		{name: "IPv4 nowhere", tcp: str(procHeader), tcp6: str(procHeader), local: client4, remote: server4, err: "tcp: no such socket\n"},
		{name: "no tables", local: client4, remote: server4, err: "no such file or directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if tt.tcp != nil {
				writeTable(t, root, "tcp", *tt.tcp)
			}
			if tt.tcp6 != nil {
				writeTable(t, root, "tcp6", *tt.tcp6)
			}

			uid, err := procOwner(root, tcpEstablished)(tt.local, tt.remote)

			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Errorf("procOwner = %d, %v, want an error with %q", uid, err, tt.err)
				}
				return
			}
			if err != nil || uid != tt.uid {
				t.Errorf("procOwner = %d, %v, want %d", uid, err, tt.uid)
			}
		})
	}
}

func str(s string) *string { return &s }

func TestPeerListenerOnALoopbackConnection(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"127.0.0.1:0", "[::1]:0"} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()

			l, err := net.Listen("tcp", addr)
			if err != nil {
				t.Skipf("no such loopback address: %v", err)
			}
			defer func() { _ = l.Close() }()
			refused := make(chan error, 4)
			pl, err := PeerListener(l, os.Getuid(), func(err error) { refused <- err })
			if err != nil {
				t.Fatalf("PeerListener: %v", err)
			}
			accepted := make(chan net.Conn, 1)
			go func() {
				c, err := pl.Accept()
				if err != nil {
					close(accepted)
					return
				}
				accepted <- c
			}()

			client, err := net.Dial("tcp", l.Addr().String())
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			defer func() { _ = client.Close() }()

			select {
			case c, ok := <-accepted:
				if !ok {
					t.Fatal("Accept failed")
				}
				defer func() { _ = c.Close() }()
				if _, err := c.Write([]byte("x")); err != nil {
					t.Errorf("Write: %v", err)
				}
				buf := make([]byte, 1)
				if _, err := io.ReadFull(client, buf); err != nil || buf[0] != 'x' {
					t.Errorf("client read = %q, %v", buf, err)
				}
			case err := <-refused:
				t.Fatalf("the user's own connection was refused: %v", err)
			case <-time.After(10 * time.Second):
				t.Fatal("no connection accepted")
			}
		})
	}
}

func TestPeerListenerRefusesAConnectionOfAnotherUser(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := make(chan error, 1)
	pl, err := PeerListener(l, os.Getuid()+1, func(err error) { refused <- err })
	if err != nil {
		t.Fatalf("PeerListener: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := pl.Accept()
		done <- err
	}()

	client, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = client.Close() }()

	select {
	case err := <-refused:
		if want := fmt.Sprintf("it is user %d's", os.Getuid()); !strings.Contains(err.Error(), want) {
			t.Errorf("refusal = %q, want it to hold %q", err, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the connection was not refused")
	}
	_ = client.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Error("the refused connection delivered data")
	}
	_ = pl.Close()
	if err := <-done; err == nil {
		t.Error("Accept returned a connection")
	}
}
