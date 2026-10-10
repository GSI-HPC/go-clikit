// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix && !solaris

package cliprogress

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// stranger returns the user that path, or the link it is with lchown, is
// another's to: a user other than this one, the owner, as this process is
// when it is not root; as root, which owns every file it makes and which
// appendPrivate trusts with links and pipes, path is given to nobody.
func stranger(t *testing.T, path string, lchown bool) int {
	t.Helper()
	me := os.Geteuid()
	if me != 0 {
		return me + 1
	}
	chown := os.Chown
	if lchown {
		chown = os.Lchown
	}
	if err := chown(path, 65534, 65534); err != nil {
		t.Fatalf("give %s to nobody: %v", path, err)
	}
	return me
}

// open opens path as appendPrivate does for me, and closes what it opened.
func open(path string, me int) error {
	f, err := appendPrivate(path, me)
	if err == nil {
		_ = f.Close()
	}
	return err
}

// within returns what done gives, and fails the test if it gives nothing
// in ten seconds, so that a call that waits for ever fails instead of
// holding the tests up.
func within[T any](t *testing.T, done <-chan T) T {
	t.Helper()
	select {
	case v := <-done:
		return v
	case <-time.After(10 * time.Second):
		t.Fatal("nothing in 10s")
	}
	var zero T
	return zero
}

// A private file is created for this user alone and appended to; one that
// others can read or write is refused as it is found, unwritten, and what
// is not a regular file is written as it is.
func TestTheLogIsAppendedToAFileForThisUserAlone(t *testing.T) {
	me := os.Geteuid()
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.jsonl")
	for _, line := range []string{"one\n", "two\n"} {
		f, err := appendPrivate(path, me)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
	if data, _ := os.ReadFile(path); string(data) != "one\ntwo\n" {
		t.Errorf("the log holds %q", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("the log was created with mode %v", info.Mode().Perm())
	}
	for _, mode := range []os.FileMode{0o640, 0o604, 0o620, 0o644} {
		wide := filepath.Join(dir, "wide.jsonl")
		_ = os.WriteFile(wide, []byte("kept\n"), 0o600)
		_ = os.Chmod(wide, mode)
		err := open(wide, me)
		if err == nil || !strings.HasSuffix(err.Error(), "run chmod 600 "+wide) {
			t.Errorf("a file of mode %v: %v", mode, err)
		}
		if data, _ := os.ReadFile(wide); string(data) != "kept\n" {
			t.Errorf("a file of mode %v was written: %q", mode, data)
		}
	}
	if err := open(os.DevNull, me); err != nil {
		t.Errorf("the null device, which anyone can write, was refused: %v", err)
	}
	if err := open(filepath.Join(dir, "missing", "progress.jsonl"), me); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a log in no directory: %v", err)
	}
	file := filepath.Join(dir, "file")
	_ = os.WriteFile(file, nil, 0o600)
	if err := open(filepath.Join(file, "progress.jsonl"), me); !errors.Is(err, syscall.ENOTDIR) {
		t.Errorf("a log under a file: %v", err)
	}
}

// Another user's file is not appended to: the lines would be theirs to
// read.
func TestAnotherUsersFileIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.jsonl")
	_ = os.WriteFile(path, nil, 0o600)
	me := stranger(t, path, false)
	err := open(path, me)
	if err == nil || !strings.Contains(err.Error(), "is owned by uid ") || !strings.Contains(err.Error(), ", not by this user (uid "+strconv.Itoa(me)+")") {
		t.Errorf("another user's file: %v", err)
	}
}

// A file of this user's under another name is not appended to: another
// user may have linked it into a directory anyone can write to, such as
// /tmp, where macOS and the BSDs do not stop them.
func TestAFileWithOtherNamesIsRefused(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "config")
	_ = os.WriteFile(mine, []byte("kept\n"), 0o600)
	path := filepath.Join(dir, "progress.jsonl")
	if err := os.Link(mine, path); err != nil {
		t.Fatal(err)
	}
	want := path + " has other names than this one (2 hard links)"
	if err := open(path, os.Geteuid()); err == nil || err.Error() != want {
		t.Errorf("a hard link to a file of this user's: %v, want %s", err, want)
	}
	if data, _ := os.ReadFile(mine); string(data) != "kept\n" {
		t.Errorf("the file behind the hard link was written: %q", data)
	}
}

// A link another user made on the way to the log is not followed, the
// file's own or a directory's: not to a file of this user's it points at,
// which would be appended to, and not to where a file is not, which would
// be created. Links of this user's are followed, relative ones from their
// directory, and a chain that does not end is refused.
func TestAnotherUsersLinkIsNotFollowed(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	_ = os.Mkdir(victim, 0o700)
	mine := filepath.Join(victim, "config")
	_ = os.WriteFile(mine, []byte("kept\n"), 0o600)
	planted := filepath.Join(dir, "planted")
	_ = os.Symlink("victim", planted)
	file := filepath.Join(dir, "file")
	_ = os.Symlink(mine, file)
	dangling := filepath.Join(dir, "dangling")
	_ = os.Symlink(filepath.Join(victim, "profile.sh"), dangling)
	me := stranger(t, planted, true)
	stranger(t, file, true)
	stranger(t, dangling, true)
	for _, path := range []string{filepath.Join(planted, "config"), filepath.Join(planted, "progress.jsonl"), file, dangling} {
		if err := open(path, me); err == nil || !strings.Contains(err.Error(), "neither this user (uid "+strconv.Itoa(me)+") nor root") {
			t.Errorf("through another user's link, %s: %v", path, err)
		}
	}
	if data, _ := os.ReadFile(mine); string(data) != "kept\n" {
		t.Errorf("the file behind the link was written: %q", data)
	}
	for _, name := range []string{"progress.jsonl", "profile.sh"} {
		if _, err := os.Lstat(filepath.Join(victim, name)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s was created behind a link", name)
		}
	}

	me = os.Geteuid()
	_ = os.Symlink(victim, filepath.Join(dir, "own"))
	_ = os.Symlink("own/../own", filepath.Join(dir, "chained"))
	if err := open(filepath.Join(dir, "chained", ".", "progress.jsonl"), me); err != nil {
		t.Errorf("through links of this user's: %v", err)
	}
	if _, err := os.Stat(filepath.Join(victim, "progress.jsonl")); err != nil {
		t.Errorf("the log was not created where the links lead: %v", err)
	}
	_ = os.Symlink("loop", filepath.Join(dir, "loop"))
	if err := open(filepath.Join(dir, "loop"), me); err == nil || !strings.HasSuffix(err.Error(), "too many levels of symbolic links") {
		t.Errorf("a loop of links: %v", err)
	}
	t.Chdir(dir)
	if err := open("own/relative.jsonl", me); err != nil {
		t.Errorf("a relative path through a link of this user's: %v", err)
	}
}

// A link put at the log's name after the walk is not followed, though a
// link of this user's or root's leads to its directory, as root's /tmp
// does to /private/tmp on macOS, and though the path such a link names
// goes through another, as a link to /tmp/logs does: the open follows the
// links on the way, and the log's own name only when the walk found a
// link there.
func TestALinkPutAtTheLogsNameSinceTheWalkIsNotFollowed(t *testing.T) {
	me := os.Geteuid()
	dir := t.TempDir()
	shared := filepath.Join(dir, "private")
	_ = os.MkdirAll(filepath.Join(shared, "logs"), 0o700)
	_ = os.Symlink("private", filepath.Join(dir, "tmp"))
	_ = os.Mkdir(filepath.Join(dir, "home"), 0o700)
	_ = os.Symlink(filepath.Join(dir, "tmp", "logs"), filepath.Join(dir, "home", "logs"))
	mine := filepath.Join(dir, "config")
	_ = os.WriteFile(mine, []byte("kept\n"), 0o600)
	for _, c := range []struct{ path, planted string }{
		{filepath.Join(dir, "tmp", "progress.jsonl"), filepath.Join(shared, "progress.jsonl")},
		{filepath.Join(dir, "home", "logs", "progress.jsonl"), filepath.Join(shared, "logs", "progress.jsonl")},
	} {
		follow, err := trustedLinks(c.path, me)
		if err != nil || follow {
			t.Errorf("the walk to %s through directories' links says follow %v: %v", c.path, follow, err)
		}
		_ = os.Symlink(mine, c.planted)
		if _, err := openPrivate(c.path, follow, me); !errors.Is(err, syscall.ELOOP) {
			t.Errorf("a link put at the log's name since the walk, %s: %v", c.path, err)
		}
	}
	if data, _ := os.ReadFile(mine); string(data) != "kept\n" {
		t.Errorf("the file behind the link was written: %q", data)
	}

	_ = os.Symlink("../progress.jsonl", filepath.Join(shared, "own.jsonl"))
	if err := open(filepath.Join(dir, "tmp", "own.jsonl"), me); err != nil {
		t.Errorf("a link of this user's as the log's name: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "progress.jsonl")); err != nil {
		t.Errorf("the log was not created where the link points: %v", err)
	}
}

// A named pipe another user made is refused, since whoever reads it would
// read every line: one with a reader once it is open, one without before
// anything waits for a reader, and one found only once it is open, through
// a link that names no path, then; one of this user's is written, as the
// pipe a shell's process substitution hands on is.
func TestAnotherUsersPipeIsRefused(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "progress.jsonl")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	rd, err := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	f, err := appendPrivate(fifo, os.Geteuid())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("one\n")
	_ = f.Close()
	buf := make([]byte, 16)
	if n, _ := rd.Read(buf); string(buf[:n]) != "one\n" {
		t.Errorf("the pipe carried %q", buf[:n])
	}
	me := stranger(t, fifo, false)
	if err := open(fifo, me); err == nil || !strings.Contains(err.Error(), "nor root") {
		t.Errorf("another user's pipe: %v", err)
	}
	_ = rd.Close()
	if err := open(fifo, me); err == nil || !strings.Contains(err.Error(), "nor root") {
		t.Errorf("another user's pipe with no reader: %v", err)
	}

	// /proc/<pid>/fd/N is a link to "pipe:[…]", which names no path.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	path := fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), w.Fd())
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no %s: %v", path, err)
	}
	if err := open(path, os.Geteuid()); err != nil {
		t.Errorf("a pipe of this user's, as %s: %v", path, err)
	}
	// Root's pipes are trusted, and root makes every pipe it opens.
	if err := open(path, os.Geteuid()+1); os.Geteuid() != 0 && (err == nil || !strings.Contains(err.Error(), "nor root")) {
		t.Errorf("a pipe of another user's, as %s: %v", path, err)
	}
}

// A named pipe with no reader holds nothing up: one another user put at
// the log's name after the walk is refused at once, and what is no pipe,
// such as a socket, fails as the system fails it. One of this user's is
// opened once a reader opens it, as an open waits, and the lines reach
// the reader.
func TestAPipeWithNoReaderHoldsNothingUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.jsonl")
	follow, err := trustedLinks(path, os.Geteuid())
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	me := stranger(t, path, false)
	if _, err := openPrivate(path, follow, me); err == nil || !strings.Contains(err.Error(), "nor root") {
		t.Errorf("another user's pipe put at the log's name since the walk: %v", err)
	}

	// A socket's name is kept short, as macOS wants it.
	t.Chdir(dir)
	l, err := net.Listen("unix", "socket")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := open("socket", os.Geteuid()); err == nil {
		t.Error("a socket was opened")
	}

	// The reader comes only after the open without blocking has found
	// none.
	mine := filepath.Join(dir, "mine.jsonl")
	if err := syscall.Mkfifo(mine, 0o600); err != nil {
		t.Fatal(err)
	}
	var f *os.File
	opened := make(chan error, 1)
	go func() {
		var err error
		f, err = appendPrivate(mine, os.Geteuid())
		opened <- err
	}()
	arrived := make(chan struct{})
	read := make(chan string, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(arrived)
		data, _ := os.ReadFile(mine)
		read <- string(data)
	}()
	if err := within(t, opened); err != nil {
		t.Fatalf("a pipe of this user's that a reader opens later: %v", err)
	}
	select {
	case <-arrived:
	default:
		t.Error("a pipe of this user's was opened before a reader opened it")
	}
	_, _ = f.WriteString("one\n")
	_ = f.Close()
	if got := within(t, read); got != "one\n" {
		t.Errorf("the pipe carried %q", got)
	}
}

// A path that names one of the process's own descriptors, as
// --progress-log=/dev/stderr does with standard error redirected to a
// file, is written through a duplicate of that descriptor: the lines
// follow what the process wrote there, whatever mode the shell gave the
// file, and none is written over. A descriptor not open for writing is
// refused, and one not open at all.
func TestTheProcesssOwnDescriptorIsWrittenThrough(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr")
	stderr, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	_, _ = stderr.WriteString("before\n")
	for _, dir := range []string{"/dev/fd/", "/proc/self/fd/"} {
		name := dir + strconv.Itoa(int(stderr.Fd()))
		f, err := appendPrivate(name, os.Geteuid()+1)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		_, _ = f.WriteString("log " + name + "\n")
		_ = f.Close()
		_, _ = stderr.WriteString("after " + name + "\n")
	}
	fd := strconv.Itoa(int(stderr.Fd()))
	want := "before\nlog /dev/fd/" + fd + "\nafter /dev/fd/" + fd + "\nlog /proc/self/fd/" + fd + "\nafter /proc/self/fd/" + fd + "\n"
	if data, _ := os.ReadFile(path); string(data) != want {
		t.Errorf("the file reads %q, want %q", data, want)
	}
	for _, name := range []string{"/dev/stdout", "/dev/stderr"} {
		f, ok, err := ownDescriptor(name)
		if !ok || err != nil {
			t.Errorf("%s: %v, %v", name, ok, err)
		}
		_ = f.Close()
	}
	if _, ok, _ := ownDescriptor("/dev/fd/x"); ok {
		t.Error("a name that is no number names a descriptor")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	for _, f := range []*os.File{r, stderr, null} {
		if f == stderr {
			f, _ = os.Open(path)
			defer f.Close()
		}
		name := "/dev/fd/" + strconv.Itoa(int(f.Fd()))
		if err := open(name, os.Geteuid()); err == nil || err.Error() != "open "+name+": not open for writing" {
			t.Errorf("a descriptor open for reading, %s: %v", f.Name(), err)
		}
	}
	if err := open("/proc/self/fd/987654", os.Geteuid()); !errors.Is(err, syscall.EBADF) {
		t.Errorf("a descriptor not open: %v", err)
	}
}
