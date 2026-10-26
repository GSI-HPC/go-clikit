// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// nobody is the user and group a test gives a file when it runs as root.
const nobody = 65534

// stranger makes path another user's, as far as a check for the user it
// returns can tell: as root it gives the file to nobody and returns root;
// as anyone else it returns a user other than the one that owns the file.
func stranger(t *testing.T, path string) (me int) {
	t.Helper()
	if os.Geteuid() != 0 {
		return os.Geteuid() + 1
	}
	if err := os.Lchown(path, nobody, nobody); err != nil {
		t.Fatal(err)
	}
	return 0
}

// symlinkTo makes the symbolic link path to target.
func symlinkTo(t *testing.T, target, path string) string {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// ids returns the user and group that own the file path.
func ids(t *testing.T, path string) (uid, gid int) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	uid, gid, _ = owner(info)
	return uid, gid
}

// A link another user made is refused, so that someone who can write a
// shared directory cannot choose which file a write of root's changes.
func TestWriteAtomicRefusesAnotherUsersLink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "victim")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "known_hosts")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	me := stranger(t, link)
	if err := writeAtomic(link, []byte("owned"), 0o644, options{}, me, os.Rename); !errors.Is(err, ErrUntrusted) {
		t.Errorf("err = %v, want ErrUntrusted", err)
	}
	if data, _ := os.ReadFile(target); string(data) != "keep" {
		t.Errorf("the target of the link was written: %q", data)
	}
}

// A write that stops between the write of the temporary file and the
// rename leaves the file as it was, and removes the temporary file.
func TestWriteAtomicStopsBeforeTheRename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stopped")
	var temporary string
	err := writeAtomic(path, []byte("new"), 0o600, options{}, os.Geteuid(), func(from, _ string) error {
		temporary = from
		if data, _ := os.ReadFile(from); string(data) != "new" {
			t.Errorf("the temporary file holds %q before the rename", data)
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Errorf("err = %v, want %v", err, stop)
	}
	if data, _ := os.ReadFile(path); string(data) != "old" {
		t.Errorf("the file holds %q, want it as it was", data)
	}
	if _, err := os.Lstat(temporary); !errors.Is(err, fs.ErrNotExist) || filepath.Dir(temporary) != dir {
		t.Errorf("the temporary file %s was left behind, or was not beside the file: %v", temporary, err)
	}
}

// With ExactMode, the new content of a file whose mode is too wide is in
// a file of exactly perm before the rename, so that it is never readable
// with the mode the file had.
func TestWriteAtomicExactModeBeforeTheRename(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeAtomic(path, []byte("new"), 0o600, options{exact: true}, os.Geteuid(), func(from, to string) error {
		info, err := os.Stat(from)
		if err != nil {
			return err
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("the temporary file has mode %v before the rename, want 0600", got)
		}
		return os.Rename(from, to)
	})
	if err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
}

// A file that root writes keeps its owner and its group, so that a file
// another administrator owns stays theirs; anyone else keeps the group
// alone, which is all the system lets them give.
func TestWriteAtomicKeepsTheOwner(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(path, nobody, nobody); err != nil {
			t.Fatal(err)
		}
	}
	uid, gid := ids(t, path)
	// As root, me is the user that WriteAtomic writes as; as anyone else,
	// a write that believes it is root's keeps the owner the file has.
	if err := writeAtomic(path, []byte("two"), 0o600, options{}, 0, os.Rename); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	if u, g := ids(t, path); u != uid || g != gid {
		t.Errorf("owner = %d:%d, want %d:%d, the owner the file had", u, g, uid, gid)
	}
}

// keepOwner gives a file another group; anyone but root may only give
// one of their own groups.
func TestKeepOwner(t *testing.T) {
	t.Parallel()

	f, err := os.CreateTemp(t.TempDir(), "tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	_, gid := ids(t, f.Name())
	err = keepOwner(f, -1, gid+1)
	_, got := ids(t, f.Name())
	switch {
	case os.Geteuid() == 0 && err != nil:
		t.Errorf("keepOwner as root: %v", err)
	case err == nil && got != gid+1:
		t.Errorf("group = %d, want %d", got, gid+1)
	case err != nil && !errors.Is(err, syscall.EPERM):
		t.Errorf("err = %v, want EPERM", err)
	}
}

func TestEnsureDirRefusesAnotherUsersDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	me := stranger(t, dir)
	err := ensureDir(dir, me)
	if !errors.Is(err, ErrUntrusted) || !strings.Contains(err.Error(), "is owned by uid") {
		t.Errorf("err = %v, want ErrUntrusted for the owner", err)
	}
}

// A file, a directory above it, or a link on the way that another user
// owns is refused; root's are not.
func TestCheckTrustedRefusesAnotherUsersFile(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path func(t *testing.T, dir string) (path string, me int)
	}{
		{"a file", func(t *testing.T, dir string) (string, int) {
			path := filepath.Join(dir, "config")
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			return path, stranger(t, path)
		}},
		{"a directory above it", func(t *testing.T, dir string) (string, int) {
			sub := filepath.Join(dir, "sub")
			if err := os.Mkdir(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(sub, "config")
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			return path, stranger(t, sub)
		}},
		{"a link on the way", func(t *testing.T, dir string) (string, int) {
			if err := os.WriteFile(filepath.Join(dir, "config"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "link")
			if os.Geteuid() != 0 {
				// The directory of the test is this user's, and would be
				// refused before the link: the link goes where root owns
				// the directory, as it does /tmp.
				link = filepath.Join(os.TempDir(), "fsutil-link-"+strconv.Itoa(os.Getpid())+"-"+filepath.Base(filepath.Dir(dir)))
				t.Cleanup(func() { _ = os.Remove(link) })
				return link, stranger(t, symlinkTo(t, filepath.Join(dir, "config"), link))
			}
			if err := os.Symlink("config", link); err != nil {
				t.Fatal(err)
			}
			return link, stranger(t, link)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, me := tc.path(t, t.TempDir())
			err := checkTrusted(path, me)
			if !errors.Is(err, ErrUntrusted) || !strings.Contains(err.Error(), "is owned by uid") {
				t.Errorf("err = %v, want ErrUntrusted for the owner", err)
			}
		})
	}

	// Root's own files are trusted by everyone.
	if err := checkTrusted("/", os.Geteuid()+1); err != nil {
		t.Errorf("the root directory: %v", err)
	}
}

// A path relative to a working directory that is gone cannot be looked
// up.
func TestCheckTrustedWithoutAWorkingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := checkTrusted("config", os.Geteuid()); err == nil {
		t.Error("checkTrusted succeeded")
	}
}
