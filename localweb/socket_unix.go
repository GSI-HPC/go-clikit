// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package localweb

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// A Unix socket is a file: who may connect is decided by its mode, and who
// may put another socket under its name by the directory it lies in and
// by every directory above that. The rules below see to it that both are
// the user and nobody else.

const (
	// maxSocketPath is the longest path a socket address takes on this
	// system: the path member of sockaddr_un less the NUL that ends it,
	// 107 bytes on Linux and 103 on macOS.
	maxSocketPath = len(syscall.RawSockaddrUnix{}.Path) - 1

	// The socket is bound under stagedName in a directory whose name is
	// stagingPrefix and stagingRandom characters, and then moved to its
	// place (listenSocket). The names are short because that path has to
	// fit a socket address too.
	stagingPrefix = ".lw"
	stagingRandom = 5
	stagedName    = "s"
	// stagingTries bounds the search for a staging directory that does
	// not exist yet. A name is taken only by a second server that starts
	// in the same directory at the same moment, or by what a killed one
	// left.
	stagingTries = 16

	// answerTimeout is how long a socket that is already there gets to
	// accept a connection, which tells one in use from one left behind. A
	// local listener accepts at once or not at all.
	answerTimeout = time.Second
)

// checkSocketPath reports why path cannot be the path of a socket, as far
// as the path itself tells, or returns nil. The length is checked here, so
// that a path too long is refused with its reason before anything is made;
// the system would say "invalid argument" and nothing else.
func checkSocketPath(path string) error {
	if path == "" {
		return fmt.Errorf("%q names no socket; give its path, as in %s/path/to/prog.sock", UnixPrefix, UnixPrefix)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("the socket path %q is not absolute", path)
	}
	path = filepath.Clean(path)
	if len(path) > maxSocketPath {
		return fmt.Errorf("the socket path %q is %d bytes long, and a Unix socket on this system takes %d at most",
			path, len(path), maxSocketPath)
	}
	dir := filepath.Dir(path)
	if staged := len(filepath.Join(dir, stagingPrefix, stagedName)) + stagingRandom; staged > maxSocketPath {
		return fmt.Errorf("the directory of the socket, %q, is too long: the socket is bound under a temporary name "+
			"in it first, a path of %d bytes, and a Unix socket on this system takes %d at most", dir, staged, maxSocketPath)
	}
	return nil
}

// listenSocket listens on a Unix socket at path, which checkSocketPath has
// accepted, and returns a listener that removes the socket when it closes,
// and the path where the socket lies: path with the symbolic links on the
// way to its directory resolved (privateDir).
//
// The socket is bound in a directory of its own that nobody else may
// enter, given its mode there, and then renamed into its place, so it is
// open to its owner alone from the moment it has a name that others can
// see. Binding it in its place and changing the mode afterwards would
// leave it open, for a moment, to whomever the umask lets in; setting the
// umask would change it for the whole process.
func listenSocket(ctx context.Context, bind bindFunc, path string) (net.Listener, string, error) {
	path = filepath.Clean(path)
	dir, err := privateDir(filepath.Dir(path), processOwners())
	if err != nil {
		return nil, "", err
	}
	path = filepath.Join(dir, filepath.Base(path))
	// A link may lead to a longer path than the one CheckAddr measured.
	if err := checkSocketPath(path); err != nil {
		return nil, "", err
	}
	if err := freeForSocket(ctx, path); err != nil {
		return nil, "", err
	}
	staging, err := makeStagingDir(dir)
	if err != nil {
		return nil, "", err
	}
	// The directory has done its part once the socket has left it; after
	// a failure it may still hold the socket.
	defer func() { _ = os.RemoveAll(staging) }()

	staged := filepath.Join(staging, stagedName)
	l, err := bind(ctx, "unix", staged)
	if err != nil {
		return nil, "", fmt.Errorf("open the socket: %w", err)
	}
	if u, ok := l.(*net.UnixListener); ok {
		// Package net removes the file it bound when the listener closes,
		// by the name it was bound under, where it is no longer;
		// socketListener removes it where it is.
		u.SetUnlinkOnClose(false)
	}
	created, err := moveIntoPlace(staged, path)
	if err != nil {
		_ = l.Close()
		return nil, "", err
	}
	return &socketListener{Listener: l, path: path, created: created}, path, nil
}

// moveIntoPlace closes the socket at staged to everyone but its owner and
// renames it to path, in one step that replaces a socket an earlier server
// left there: at no moment is the name free for another process to take.
// It returns the description of the file, by which socketListener knows
// it again: the mode does not count for os.SameFile.
func moveIntoPlace(staged, path string) (fs.FileInfo, error) {
	created, err := os.Lstat(staged)
	if err == nil {
		err = os.Chmod(staged, 0o600)
	}
	if err == nil {
		err = os.Rename(staged, path)
	}
	if err != nil {
		return nil, fmt.Errorf("put the socket in its place: %w", err)
	}
	return created, nil
}

// owners says whose directories a socket may lie in and below.
type owners struct {
	// user is the ID the process runs as; the directory of the socket
	// belongs to this user.
	user int
	// system are the IDs that the directories above it may belong to as
	// well: root, and what root looks like where there is none.
	system []int
}

// processOwners returns the owners as they are for this process.
func processOwners() owners {
	// A system without user namespaces has neither file, and there root
	// is root.
	uidMap, _ := os.ReadFile("/proc/self/uid_map")
	overflow, _ := os.ReadFile("/proc/sys/kernel/overflowuid")
	return owners{user: os.Geteuid(), system: systemIDs(string(uidMap), string(overflow))}
}

// systemIDs returns the IDs under which a process sees the directories of
// the system: 0, and in a user namespace that maps no root also the ID the
// kernel shows for every owner the namespace has no ID for.
//
// uidMap is the content of /proc/self/uid_map, whose lines each map a
// range of IDs: its first ID inside, the ID outside, and the length.
// overflow is the content of /proc/sys/kernel/overflowuid.
//
// A sandbox or a rootless container often maps the user and nobody else.
// Nothing belongs to 0 there: /, /run and /tmp show the overflow ID, and a
// rule that asked for root would refuse every path. The price is that a
// directory of another user shows the same ID, so in such a namespace the
// mode of a directory above the socket's is checked and, in effect, its
// owner is not.
func systemIDs(uidMap, overflow string) []int {
	ids := []int{0}
	if uidMap == "" {
		return ids
	}
	for line := range strings.Lines(uidMap) {
		if f := strings.Fields(line); len(f) > 0 && f[0] == "0" {
			return ids
		}
	}
	if id, err := strconv.Atoi(strings.TrimSpace(overflow)); err == nil {
		ids = append(ids, id)
	}
	return ids
}

// privateDir makes sure that nobody but the user can put a file into dir,
// the directory of the socket, or another directory in its place. It
// makes what is missing of dir, closed to everyone else, and returns dir
// with the symbolic links on the way to it resolved.
//
// Whoever may write to the directory can rename the socket away and put a
// listener of their own under its name, and whoever may write to a
// directory above it can do the same with the directory. A forward opens
// the path again for every connection, so it would hand that listener the
// launch token. So:
//
//   - dir belongs to the user, and nobody else may write to it.
//   - Every directory above it, up to the root, belongs to the user or to
//     the system, and nobody but its owner may write to it, unless it has
//     the sticky bit: there nobody renames an entry but its owner and the
//     directory's, which is what lets a directory of the user's lie in
//     /tmp.
//   - A symbolic link can be pointed elsewhere by whoever may replace it,
//     so the links are resolved first, the rules applied to where they
//     lead, and the socket named without them.
func privateDir(dir string, who owners) (string, error) {
	// What MkdirAll has made or found, EvalSymlinks and Lstat can read.
	err := os.MkdirAll(dir, 0o700)
	if err == nil {
		dir, err = filepath.EvalSymlinks(dir)
	}
	var info fs.FileInfo
	if err == nil {
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return "", fmt.Errorf("make the directory of the socket: %w", err)
	}
	if uid(info) != who.user {
		return "", fmt.Errorf("the directory of the socket, %s, belongs to another user, who could replace the socket; "+
			"name a socket in a directory of your own", dir)
	}
	if mode := info.Mode().Perm(); mode&0o022 != 0 {
		return "", fmt.Errorf("the directory of the socket, %s, has the mode %04o: its group or everyone may write to it, "+
			"and could replace the socket; remove that permission or name a socket in another directory", dir, mode)
	}
	for above := filepath.Dir(dir); ; above = filepath.Dir(above) {
		if err := who.checkAbove(above); err != nil {
			return "", err
		}
		if above == filepath.Dir(above) {
			return dir, nil
		}
	}
}

// checkAbove reports why dir, a directory above the socket's, lets
// somebody other than the user and the system replace what lies below it,
// or returns nil.
func (o owners) checkAbove(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("look at %s, above the directory of the socket: %w", dir, err)
	}
	if id := uid(info); id != o.user && !slices.Contains(o.system, id) {
		return fmt.Errorf("%s, above the directory of the socket, belongs to another user, who could replace "+
			"everything below it, the socket included; name a socket below directories of your own", dir)
	}
	if mode := info.Mode(); mode.Perm()&0o022 != 0 && mode&fs.ModeSticky == 0 {
		return fmt.Errorf("%s, above the directory of the socket, has the mode %04o: its group or everyone may write "+
			"to it, and could replace everything below it, the socket included; remove that permission or name a "+
			"socket elsewhere", dir, mode.Perm())
	}
	return nil
}

// uid returns the owner of a file that os.Lstat described.
func uid(info fs.FileInfo) int {
	// os.Lstat on Unix describes every file with a *syscall.Stat_t.
	return int(info.Sys().(*syscall.Stat_t).Uid)
}

// freeForSocket reports why the socket cannot be put at path, or returns
// nil: when nothing is there, or a socket that nobody listens on, which a
// killed server leaves behind and the new socket replaces. A socket that
// answers belongs to a running server, and taking its name would cut that
// one off from every forward. Anything that is not a socket was put there
// for a reason, and is not this server's to remove.
func freeForSocket(ctx context.Context, path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("look at the socket path: %w", err)
	}
	if info.Mode().Type() != fs.ModeSocket {
		return fmt.Errorf("%s exists and is not a socket; it is not removed", path)
	}
	d := net.Dialer{Timeout: answerTimeout}
	conn, err := d.DialContext(ctx, "unix", path)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("another server is listening on the socket %s; stop it or name another socket", path)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("find out whether the socket %s is in use: %w", path, err)
	}
	return nil
}

// makeStagingDir makes a directory in dir that the user alone can enter,
// under a name that is not taken, and returns its path.
func makeStagingDir(dir string) (string, error) {
	var err error
	for range stagingTries {
		staging := filepath.Join(dir, stagingPrefix+rand.Text()[:stagingRandom])
		// Mkdir fails on a name that exists, a symbolic link included, so
		// the directory is one this call made.
		if err = os.Mkdir(staging, 0o700); err == nil {
			return staging, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	return "", fmt.Errorf("make a directory to bind the socket in: %w", err)
}

// socketListener is the listener on a Unix socket. It removes the socket
// when it closes, so that a later server, or a user who looks, finds a
// socket only where one is served.
type socketListener struct {
	net.Listener
	// path is where the socket lies.
	path string
	// created describes the socket as it was bound.
	created fs.FileInfo
	// remove runs the removal once: an HTTP server closes a listener on
	// more than one of its ways out.
	remove sync.Once
}

// Close removes the socket and closes the listener. The socket is removed
// only while it is still the one this listener bound: when the user has
// removed it and started another server on the same path, the file there
// is that one's. The check comes before the listener closes, because until
// then the bound socket keeps its file alive, and no other file can be
// mistaken for it.
func (l *socketListener) Close() error {
	l.remove.Do(func() {
		if now, err := os.Lstat(l.path); err == nil && os.SameFile(l.created, now) {
			_ = os.Remove(l.path)
		}
	})
	return l.Listener.Close()
}

// Addr returns the path the socket lies at, not the one it was bound
// under, which the system would report.
func (l *socketListener) Addr() net.Addr {
	return &net.UnixAddr{Name: l.path, Net: "unix"}
}

// addrInUse reports whether a bind failed because the address is in use.
func addrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

// noSuchAddress reports whether a bind failed because the machine does not
// have the address at all: IPv6 is off for the loopback interface, or the
// kernel has none.
func noSuchAddress(err error) bool {
	return errors.Is(err, syscall.EADDRNOTAVAIL) || errors.Is(err, syscall.EAFNOSUPPORT)
}
