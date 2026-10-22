// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package launch

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// The files of an Instance in its directory. sind and brig used these
// names and the keys of Record before the kit had them, so that a program
// that adopts the kit finds a server an older version of it started.
const (
	// LockFile is the file whose lock makes two starts take turns.
	LockFile = "ui.lock"
	// RecordFile holds the Record of the server that runs, as JSON.
	RecordFile = "ui.json"
)

// maxRecord is the most bytes Running reads of a record; a record is
// about a hundred.
const maxRecord = 4096

// dialTimeout is how long Running waits for the server of a record to
// take a connection.
const dialTimeout = time.Second

// Record is what an Instance publishes of the server that runs.
type Record struct {
	// URL is the address the server prints, the launch token included.
	URL string `json:"url"`
	// PID is the ID of the server's process.
	PID int `json:"pid"`
}

// Instance is the guard that keeps one server of a program per user: a
// private directory that holds the lock and the record of the server that
// runs.
type Instance struct {
	dir   string
	lockf *os.File

	// pid and uid are those of this process.
	pid, uid int
	// flock takes or drops the lock on a file descriptor, as flock(2).
	flock func(fd, how int) error
	// alive reports whether the process pid runs and is the user's.
	alive func(pid int) bool
	// dial reports whether something takes a connection at addr.
	dial func(addr netip.AddrPort) bool
	// owner returns the user of the socket that listens at addr.
	owner func(addr netip.AddrPort) (int, error)
}

// Dir returns the instance's directory, a directory of the user's that no
// other user can enter: the place for the Opener's redirect file.
func (i *Instance) Dir() string { return i.dir }

func (i *Instance) lockPath() string   { return filepath.Join(i.dir, LockFile) }
func (i *Instance) recordPath() string { return filepath.Join(i.dir, RecordFile) }

// Release drops the lock, which Acquire took, so that another start can
// go on. It does nothing when the lock is not held, so a program can
// defer it right after Acquire.
func (i *Instance) Release() {
	if i.lockf != nil {
		// Closing the file drops its lock.
		_ = i.lockf.Close()
		i.lockf = nil
	}
}

// Running returns the record of a server that still runs, and true; or
// false when there is none. A server runs when its record is a file of
// the user's that only the user can read, its process lives, and a socket
// of the user listens at the loopback address of its URL. The last check
// keeps the token from another user's server that listens on a port the
// recorded one gave up; where the owner of a socket cannot be told
// (localweb.ListenerOwner), no server runs.
func (i *Instance) Running() (Record, bool) {
	r, ok := i.read()
	if !ok || r.PID == i.pid || !i.alive(r.PID) {
		return Record{}, false
	}
	addr, err := recordAddr(r.URL)
	if err != nil || !i.dial(addr) {
		return Record{}, false
	}
	uid, err := i.owner(addr)
	if err != nil || uid != i.uid {
		return Record{}, false
	}
	return r, true
}

// Publish records the server of this process at rawURL, and drops the
// lock: from now on a start finds it Running. rawURL is the address the
// server prints: http, on a loopback IP address and a port.
func (i *Instance) Publish(rawURL string) error {
	if _, err := recordAddr(rawURL); err != nil {
		return err
	}
	data, _ := json.Marshal(Record{URL: rawURL, PID: i.pid}) // a string and a number
	if err := writePrivate(i.recordPath(), append(data, '\n')); err != nil {
		return fmt.Errorf("record the server: %w", err)
	}
	i.Release()
	return nil
}

// Retire removes the record when it is this process's, under the lock, so
// that a start that runs at the same time sees the server either running
// or gone. A program defers it after Publish.
func (i *Instance) Retire() error {
	if err := i.lock(); err != nil {
		return err
	}
	defer i.Release()
	if r, ok := i.read(); ok && r.PID == i.pid {
		return os.Remove(i.recordPath())
	}
	return nil
}

// decodeRecord reads a record from r: at most maxRecord bytes of JSON
// with a URL and a PID above 0.
func decodeRecord(r io.Reader) (Record, bool) {
	var rec Record
	data, err := io.ReadAll(io.LimitReader(r, maxRecord))
	if err != nil || json.Unmarshal(data, &rec) != nil || rec.PID <= 0 || rec.URL == "" {
		return Record{}, false
	}
	return rec, true
}

// recordAddr returns the address of the server at rawURL, which must be an
// http URL of a loopback IP address and a port, without a user or a
// fragment. A record holds nothing else: a URL that names another host
// would take the token there.
func recordAddr(rawURL string) (netip.AddrPort, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("parse the server's address: %w", err)
	}
	addr, err := netip.ParseAddrPort(u.Host)
	if u.Scheme != "http" || u.User != nil || u.Fragment != "" || err != nil || !addr.Addr().IsLoopback() || addr.Port() == 0 {
		return netip.AddrPort{}, fmt.Errorf("the server's address is no http address of a loopback IP address and a port: %q", u.Redacted())
	}
	return netip.AddrPortFrom(addr.Addr().Unmap(), addr.Port()), nil
}

// dial reports whether something takes a TCP connection at addr.
func dial(addr netip.AddrPort) bool {
	c, err := net.DialTimeout("tcp", addr.String(), dialTimeout)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// checkProgram returns an error unless program can name a directory: a
// name that is not empty, no path, and neither "." nor "..".
func checkProgram(program string) error {
	if program == "" || program == "." || program == ".." || program != filepath.Base(program) {
		return fmt.Errorf("the program's name %q cannot name a directory", program)
	}
	return nil
}
