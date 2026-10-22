// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package localweb

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// shortDir makes a directory with a short path, closed to everyone else:
// the temporary directory of the test may be too long for a socket, as it
// is on macOS. Its path has its symbolic links resolved.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "lw")
	if err != nil {
		t.Fatalf("make a directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolve %s: %v", dir, err)
	}
	return dir
}

// deadSocket leaves a socket file that nobody listens on, as a killed
// server does.
func deadSocket(t *testing.T, path string) {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen on %s: %v", path, err)
	}
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode().Type() != fs.ModeSocket {
		t.Fatalf("no socket left at %s: %v", path, err)
	}
}

// dials reports whether a connection to the socket at path is accepted.
func dials(path string) bool {
	c, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func mustMkdir(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Chmod, since the umask would take bits off Mkdir's.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod: %v", err)
	}
}

func TestCheckSocketPath(t *testing.T) {
	t.Parallel()

	long := "/" + strings.Repeat("d", maxSocketPath)
	// The path fits, and the temporary name in its directory does not.
	longDir := "/" + strings.Repeat("d", maxSocketPath-3)
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "a path", path: "/run/user/1000/prog/prog.sock"},
		{name: "a path with dots", path: "/run/user/../prog/./prog.sock"},
		{name: "the longest path", path: "/" + strings.Repeat("d", maxSocketPath-1)},
		{name: "empty", path: "", want: `"unix:" names no socket`},
		{name: "relative", path: "prog.sock", want: `the socket path "prog.sock" is not absolute`},
		{name: "too long", path: long, want: "bytes long, and a Unix socket on this system takes"},
		{name: "a directory too long for the temporary name", path: longDir + "/a", want: "is too long: the socket is bound under a temporary name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := checkSocketPath(tt.path)

			switch {
			case tt.want == "" && err != nil:
				t.Errorf("checkSocketPath(%q) = %v, want nil", tt.path, err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("checkSocketPath(%q) = %v, want it to hold %q", tt.path, err, tt.want)
			}
		})
	}
	// CheckAddr takes the path after the prefix.
	if err := CheckAddr(UnixPrefix + "relative"); err == nil {
		t.Error("CheckAddr accepted a relative socket path")
	}
}

func TestListenOnASocket(t *testing.T) {
	t.Parallel()

	dir := shortDir(t)
	path := filepath.Join(dir, "sub", "prog.sock")
	var staged string
	bind := func(ctx context.Context, network, address string) (net.Listener, error) {
		// The socket is bound out of sight, in a directory of its own.
		staged = address
		parent := filepath.Dir(address)
		info, err := os.Stat(parent)
		if err != nil {
			t.Errorf("the staging directory: %v", err)
		} else if info.Mode().Perm() != 0o700 {
			t.Errorf("staging directory mode = %v, want 0700", info.Mode().Perm())
		}
		if !strings.HasPrefix(filepath.Base(parent), stagingPrefix) || filepath.Base(address) != stagedName {
			t.Errorf("bound at %s, want a %s… directory and %s", address, stagingPrefix, stagedName)
		}
		if filepath.Dir(parent) != filepath.Join(dir, "sub") {
			t.Errorf("staging directory %s is not beside the socket", parent)
		}
		return systemBind(ctx, network, address)
	}

	b, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{Bind: bind})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = b.Close() }()

	if b.Socket != path || len(b.Listeners) != 1 || len(b.Addrs) != 0 || b.Host() != "" {
		t.Fatalf("Bound = %+v, want the socket %s alone", b, path)
	}
	if got := b.Listeners[0].Addr(); got.String() != path || got.Network() != "unix" {
		t.Errorf("Addr() = %v (%s), want %s", got, got.Network(), path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("the socket: %v", err)
	}
	if info.Mode().Type() != fs.ModeSocket || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want a socket with mode 0600", info.Mode())
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("directory = %v, %v, want mode 0700", info, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "prog.sock" {
		t.Errorf("directory holds %v, %v, want the socket alone", entries, err)
	}
	if _, err := os.Lstat(staged); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the staged socket %s is left: %v", staged, err)
	}
	if !b.Answers("localhost:5555") || b.Answers("example.org") {
		t.Error("a socket answers to the loopback names alone")
	}
	if !dials(path) {
		t.Error("the socket does not accept")
	}

	if err := b.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket is left after Close: %v", err)
	}
}

func TestListenOnTheDefaultBind(t *testing.T) {
	t.Parallel()

	path := filepath.Join(shortDir(t), "p.sock")

	b, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = b.Close() }()

	if !dials(path) {
		t.Error("the socket does not accept")
	}
}

func TestListenReplacesADeadSocket(t *testing.T) {
	t.Parallel()

	path := filepath.Join(shortDir(t), "p.sock")
	deadSocket(t, path)
	if dials(path) {
		t.Fatal("the leftover socket answers")
	}

	b, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = b.Close() }()

	if !dials(path) {
		t.Error("the new socket does not accept")
	}
	if info, err := os.Lstat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("socket = %v, %v, want mode 0600", info, err)
	}
}

func TestListenRefusesALiveSocket(t *testing.T) {
	t.Parallel()

	path := filepath.Join(shortDir(t), "p.sock")
	first, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = first.Close() }()

	second, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})

	if err == nil {
		_ = second.Close()
		t.Fatal("Listen took the socket of a running server")
	}
	if !strings.Contains(err.Error(), "another server is listening on the socket "+path) {
		t.Errorf("error = %q", err)
	}
	if !dials(path) {
		t.Error("the first server was cut off")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("directory holds %v, want the socket alone", entries)
	}
}

func TestListenRefusesAFileThatIsNoSocket(t *testing.T) {
	t.Parallel()

	path := filepath.Join(shortDir(t), "p.sock")
	if err := os.WriteFile(path, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	b, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})

	if err == nil {
		_ = b.Close()
		t.Fatal("Listen replaced a file")
	}
	if want := path + " exists and is not a socket; it is not removed"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if got, _ := os.ReadFile(path); string(got) != "mine" {
		t.Errorf("the file now holds %q", got)
	}
}

func TestSocketIsRemovedOnCloseOnlyWhileItIsTheOneBound(t *testing.T) {
	t.Parallel()

	t.Run("still the one bound", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(shortDir(t), "p.sock")
		b, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})
		if err != nil {
			t.Fatal(err)
		}

		_ = b.Close()
		// A second Close, as net/http makes on more than one of its ways out.
		_ = b.Close()

		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the socket is left: %v", err)
		}
	})

	t.Run("removed by the user", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(shortDir(t), "p.sock")
		b, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}

		if err := b.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("something appeared at the path: %v", err)
		}
	})

	t.Run("replaced by another server", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(shortDir(t), "p.sock")
		a, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		b, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})
		if err != nil {
			t.Fatal(err)
		}

		if err := a.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}

		if !dials(path) {
			t.Error("closing the first server cut off the second")
		}
		_ = b.Close()
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the second socket is left: %v", err)
		}
	})
}

func TestListenOnASocketFailures(t *testing.T) {
	t.Parallel()

	t.Run("the directory lies below a file", func(t *testing.T) {
		t.Parallel()

		dir := shortDir(t)
		file := filepath.Join(dir, "file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		_, err := Listen(t.Context(), UnixPrefix+filepath.Join(file, "d", "p.sock"), ListenOptions{})

		if err == nil || !strings.HasPrefix(err.Error(), "make the directory of the socket: ") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("the link leads to a directory too long for a socket", func(t *testing.T) {
		t.Parallel()

		base := shortDir(t)
		// The socket path fits, and the temporary name beside it does not.
		target := filepath.Join(base, strings.Repeat("t", maxSocketPath-len("/p.sock")-len(base)-1))
		mustMkdir(t, target, 0o700)
		link := filepath.Join(base, "l")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		addr := UnixPrefix + filepath.Join(link, "p.sock")
		if err := CheckAddr(addr); err != nil {
			t.Fatalf("CheckAddr: %v", err)
		}

		_, err := Listen(t.Context(), addr, ListenOptions{})

		if err == nil || !strings.Contains(err.Error(), "is too long") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("the link is resolved", func(t *testing.T) {
		t.Parallel()

		base := shortDir(t)
		mustMkdir(t, filepath.Join(base, "real"), 0o700)
		if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "l")); err != nil {
			t.Fatal(err)
		}

		b, err := Listen(t.Context(), UnixPrefix+filepath.Join(base, "l", "p.sock"), ListenOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = b.Close() }()

		if want := filepath.Join(base, "real", "p.sock"); b.Socket != want {
			t.Errorf("Socket = %q, want %q", b.Socket, want)
		}
	})

	t.Run("the socket path cannot be looked at", func(t *testing.T) {
		t.Parallel()

		dir := shortDir(t)
		// A NUL cannot be in a path.
		err := freeForSocket(t.Context(), filepath.Join(dir, "p.sock\x00"))
		if err == nil || !strings.HasPrefix(err.Error(), "look at the socket path: ") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("the bind fails", func(t *testing.T) {
		t.Parallel()

		dir := shortDir(t)
		boom := errors.New("boom")
		bind := func(context.Context, string, string) (net.Listener, error) { return nil, boom }

		_, err := Listen(t.Context(), UnixPrefix+filepath.Join(dir, "p.sock"), ListenOptions{Bind: bind})

		if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "open the socket: ") {
			t.Errorf("error = %v", err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("directory holds %v, want it empty", entries)
		}
	})

	t.Run("the bind makes no socket", func(t *testing.T) {
		t.Parallel()

		dir := shortDir(t)
		l := &fakeListener{addr: &net.UnixAddr{Name: "x", Net: "unix"}}
		bind := func(context.Context, string, string) (net.Listener, error) { return l, nil }

		_, err := Listen(t.Context(), UnixPrefix+filepath.Join(dir, "p.sock"), ListenOptions{Bind: bind})

		if err == nil || !strings.HasPrefix(err.Error(), "put the socket in its place: ") {
			t.Errorf("error = %v", err)
		}
		if !l.isClosed() {
			t.Error("the listener is left open")
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("directory holds %v, want it empty", entries)
		}
	})

	t.Run("the socket cannot be given its mode", func(t *testing.T) {
		t.Parallel()

		dir := shortDir(t)
		// A link where the socket was: it exists, and has nothing behind it.
		bind := func(ctx context.Context, network, address string) (net.Listener, error) {
			l, err := systemBind(ctx, network, address)
			if err != nil {
				return nil, err
			}
			_ = os.Remove(address)
			return l, os.Symlink(filepath.Join(dir, "nothing"), address)
		}

		_, err := Listen(t.Context(), UnixPrefix+filepath.Join(dir, "p.sock"), ListenOptions{Bind: bind})

		if err == nil || !strings.HasPrefix(err.Error(), "put the socket in its place: ") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("the socket cannot be put in its place", func(t *testing.T) {
		t.Parallel()

		dir := shortDir(t)
		path := filepath.Join(dir, "p.sock")
		// A directory takes the name after the free check was made.
		bind := func(ctx context.Context, network, address string) (net.Listener, error) {
			if err := os.Mkdir(path, 0o700); err != nil {
				return nil, err
			}
			return systemBind(ctx, network, address)
		}

		_, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{Bind: bind})

		if err == nil || !strings.HasPrefix(err.Error(), "put the socket in its place: ") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("the directory is refused", func(t *testing.T) {
		t.Parallel()

		base := shortDir(t)
		open := filepath.Join(base, "open")
		mustMkdir(t, open, 0o777)

		_, err := Listen(t.Context(), UnixPrefix+filepath.Join(open, "p.sock"), ListenOptions{})

		if err == nil || !strings.Contains(err.Error(), "may write to it") {
			t.Errorf("error = %v", err)
		}
	})
}

func TestFreeForSocket(t *testing.T) {
	t.Parallel()

	t.Run("nothing there", func(t *testing.T) {
		t.Parallel()

		if err := freeForSocket(t.Context(), filepath.Join(shortDir(t), "p.sock")); err != nil {
			t.Errorf("freeForSocket = %v, want nil", err)
		}
	})

	t.Run("the path lies below a file", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(shortDir(t), "file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		err := freeForSocket(t.Context(), filepath.Join(file, "p.sock"))

		if err == nil || !strings.HasPrefix(err.Error(), "look at the socket path: ") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("a dead socket", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(shortDir(t), "p.sock")
		deadSocket(t, path)

		if err := freeForSocket(t.Context(), path); err != nil {
			t.Errorf("freeForSocket = %v, want nil", err)
		}
	})

	t.Run("a live socket", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(shortDir(t), "p.sock")
		l, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = l.Close() }()

		err = freeForSocket(t.Context(), path)

		if err == nil || !strings.Contains(err.Error(), "another server is listening") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("a context that has ended", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(shortDir(t), "p.sock")
		deadSocket(t, path)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := freeForSocket(ctx, path)

		if err == nil || !strings.HasPrefix(err.Error(), "find out whether the socket "+path+" is in use: ") {
			t.Errorf("error = %v", err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want it to wrap context.Canceled", err)
		}
	})
}

func TestMakeStagingDir(t *testing.T) {
	t.Parallel()

	t.Run("a directory only the user enters", func(t *testing.T) {
		t.Parallel()

		dir := shortDir(t)

		a, err := makeStagingDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		b, err := makeStagingDir(dir)
		if err != nil {
			t.Fatal(err)
		}

		if a == b || filepath.Dir(a) != dir || !strings.HasPrefix(filepath.Base(a), stagingPrefix) {
			t.Errorf("directories %s and %s, want two of %s… in %s", a, b, stagingPrefix, dir)
		}
		if len(filepath.Base(a)) != len(stagingPrefix)+stagingRandom {
			t.Errorf("name %q has the wrong length", filepath.Base(a))
		}
		if info, err := os.Stat(a); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("directory = %v, %v, want mode 0700", info, err)
		}
	})

	t.Run("a missing directory", func(t *testing.T) {
		t.Parallel()

		_, err := makeStagingDir(filepath.Join(shortDir(t), "missing"))

		if err == nil || !strings.HasPrefix(err.Error(), "make a directory to bind the socket in: ") {
			t.Errorf("error = %v", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("error = %v, want it to wrap fs.ErrNotExist", err)
		}
	})
}

func TestPrivateDir(t *testing.T) {
	t.Parallel()

	me := processOwners()
	if me.user != os.Geteuid() || !slices.Contains(me.system, 0) {
		t.Errorf("processOwners() = %+v, want the process's user and root", me)
	}

	t.Run("makes what is missing", func(t *testing.T) {
		t.Parallel()

		base := shortDir(t)

		got, err := privateDir(filepath.Join(base, "a", "b"), me)
		if err != nil {
			t.Fatal(err)
		}

		if want := filepath.Join(base, "a", "b"); got != want {
			t.Errorf("privateDir = %q, want %q", got, want)
		}
		for _, d := range []string{"a", "a/b"} {
			if info, err := os.Stat(filepath.Join(base, d)); err != nil || info.Mode().Perm() != 0o700 {
				t.Errorf("%s = %v, %v, want mode 0700", d, info, err)
			}
		}
	})

	t.Run("resolves links", func(t *testing.T) {
		t.Parallel()

		base := shortDir(t)
		mustMkdir(t, filepath.Join(base, "real"), 0o700)
		if err := os.Symlink("real", filepath.Join(base, "l")); err != nil {
			t.Fatal(err)
		}

		got, err := privateDir(filepath.Join(base, "l"), me)

		if want := filepath.Join(base, "real"); err != nil || got != want {
			t.Errorf("privateDir = %q, %v, want %q", got, err, want)
		}
	})

	t.Run("a directory of another user", func(t *testing.T) {
		t.Parallel()

		other := owners{user: me.user + 1, system: me.system}

		_, err := privateDir(shortDir(t), other)

		if err == nil || !strings.Contains(err.Error(), "belongs to another user, who could replace the socket") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("a directory others may write to", func(t *testing.T) {
		t.Parallel()

		for _, mode := range []fs.FileMode{0o770, 0o707, 0o777, 0o720} {
			dir := filepath.Join(shortDir(t), "d")
			mustMkdir(t, dir, mode)

			_, err := privateDir(dir, me)

			if err == nil || !strings.Contains(err.Error(), "may write to it") {
				t.Errorf("mode %04o: error = %v", mode, err)
			}
		}
	})

	t.Run("a directory others may read", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(shortDir(t), "d")
		mustMkdir(t, dir, 0o755)

		if _, err := privateDir(dir, me); err != nil {
			t.Errorf("privateDir = %v, want nil", err)
		}
	})

	t.Run("a directory above that others may write to", func(t *testing.T) {
		t.Parallel()

		above := filepath.Join(shortDir(t), "above")
		mustMkdir(t, above, 0o777)

		_, err := privateDir(filepath.Join(above, "d"), me)

		if err == nil || !strings.Contains(err.Error(), "above the directory of the socket, has the mode 0777") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("a directory above that others may write to, with the sticky bit", func(t *testing.T) {
		t.Parallel()

		above := filepath.Join(shortDir(t), "above")
		mustMkdir(t, above, 0o777|fs.ModeSticky)

		if _, err := privateDir(filepath.Join(above, "d"), me); err != nil {
			t.Errorf("privateDir = %v, want nil", err)
		}
	})

	t.Run("the directory cannot be made", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(shortDir(t), "file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		_, err := privateDir(filepath.Join(file, "d"), me)

		if err == nil || !strings.HasPrefix(err.Error(), "make the directory of the socket: ") {
			t.Errorf("error = %v", err)
		}
	})
}

func TestCheckAbove(t *testing.T) {
	t.Parallel()

	user := os.Geteuid()
	dir := shortDir(t)
	open := filepath.Join(dir, "open")
	mustMkdir(t, open, 0o777)

	tests := []struct {
		name string
		who  owners
		dir  string
		want string
	}{
		{name: "the user's", who: owners{user: user}, dir: dir},
		{name: "the system's", who: owners{user: user + 1, system: []int{user}}, dir: dir},
		{name: "another user's", who: owners{user: user + 1, system: []int{user + 2}}, dir: dir, want: "belongs to another user"},
		{name: "no system", who: owners{user: user + 1}, dir: dir, want: "belongs to another user"},
		{name: "writable by others", who: owners{user: user}, dir: open, want: "has the mode 0777"},
		{name: "missing", who: owners{user: user}, dir: filepath.Join(dir, "missing"), want: "above the directory of the socket: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.who.checkAbove(tt.dir)

			switch {
			case tt.want == "" && err != nil:
				t.Errorf("checkAbove = %v, want nil", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("checkAbove = %v, want it to hold %q", err, tt.want)
			}
		})
	}
}

func TestSystemIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		uidMap   string
		overflow string
		want     []int
	}{
		{name: "no namespace files", want: []int{0}},
		{name: "the initial namespace", uidMap: "         0          0 4294967295\n", overflow: "65534\n", want: []int{0}},
		{name: "root among others", uidMap: "1000 1000 1\n         0     100000      1\n", overflow: "65534\n", want: []int{0}},
		{name: "a namespace that maps the user alone", uidMap: "1000 1000 1\n", overflow: "65534\n", want: []int{0, 65534}},
		{name: "a root that is not the first ID inside", uidMap: "10 0 1\n", overflow: "65534\n", want: []int{0, 65534}},
		{name: "blank lines", uidMap: "\n  \n1000 1000 1", overflow: "4294967294", want: []int{0, 4294967294}},
		{name: "no overflow ID", uidMap: "1000 1000 1\n", want: []int{0}},
		{name: "an overflow ID that is no number", uidMap: "1000 1000 1\n", overflow: "none\n", want: []int{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := systemIDs(tt.uidMap, tt.overflow); !slices.Equal(got, tt.want) {
				t.Errorf("systemIDs = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServeStopsASocketAndRemovesIt(t *testing.T) {
	t.Parallel()

	path := filepath.Join(shortDir(t), "p.sock")
	b, err := Listen(t.Context(), UnixPrefix+path, ListenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) })
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, b.Listeners, h, ServeOptions{}) }()
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}}}

	res, err := client.Get("http://localhost/")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = res.Body.Close()
	cancel()

	if err := <-done; err != nil {
		t.Errorf("Serve = %v, want nil", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket is left after the stop: %v", err)
	}
}
