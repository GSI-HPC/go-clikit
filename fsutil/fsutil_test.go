// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package fsutil_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/GSI-HPC/go-clikit/fsutil"
)

// write makes the file path with data and then gives it mode, which the
// umask cannot take away then.
func write(t *testing.T, path, data string, mode os.FileMode) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// mkdir makes the directory path and then gives it mode.
func mkdir(t *testing.T, path string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// symlink makes the symbolic link path to target.
func symlink(t *testing.T, target, path string) string {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// content returns what the file path holds, or the error that reading it
// returned.
func content(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

// mode returns the permission bits of the file path.
func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// noTemporaryFiles fails the test when dir holds a file whose name begins
// with a dot other than a lock file, which is what WriteAtomic's temporary
// files are.
func noTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && !strings.HasSuffix(e.Name(), ".lock") {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
}

func TestWriteAtomic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		existing os.FileMode // 0 for no file
		perm     os.FileMode
		opts     []fsutil.Option
		want     os.FileMode
	}{
		{"a new file gets exactly perm, whatever the umask", 0, 0o666, nil, 0o666},
		{"a new private file", 0, 0o600, nil, 0o600},
		{"a file keeps its mode", 0o664, 0o600, nil, 0o664},
		{"a file loses write permission for others", 0o666, 0o600, nil, 0o664},
		{"a new file with ExactMode", 0, 0o640, []fsutil.Option{fsutil.ExactMode()}, 0o640},
		{"a file made private with ExactMode", 0o644, 0o600, []fsutil.Option{fsutil.ExactMode()}, 0o600},
		{"a file made wider with ExactMode", 0o600, 0o666, []fsutil.Option{fsutil.ExactMode()}, 0o666},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "made", "for", "it")
			path := filepath.Join(dir, "state")
			if tc.existing != 0 {
				mkdir(t, dir, 0o700)
				write(t, path, "old", tc.existing)
			}
			if err := fsutil.WriteAtomic(path, []byte("new"), tc.perm, tc.opts...); err != nil {
				t.Fatalf("WriteAtomic: %v", err)
			}
			if got := content(path); got != "new" {
				t.Errorf("content = %q, want %q", got, "new")
			}
			if got := mode(t, path); got != tc.want {
				t.Errorf("mode = %v, want %v", got, tc.want)
			}
			if tc.existing == 0 {
				if got := mode(t, dir); got != 0o700 {
					t.Errorf("the directory it made has mode %v, want 0700", got)
				}
			}
			noTemporaryFiles(t, dir)
		})
	}
}

// linkedTarget lays out dir/c/target, the link dir/c/d/link to ../target,
// the linked directory dir/alias to c/d and the link dir/entry to
// alias/link, and returns the entry and the target. The system reads
// ../target against dir/c/d, where the link is, while dir/alias/../target
// spelled out is dir/target.
func linkedTarget(t *testing.T) (entry, target string) {
	t.Helper()
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "c", "d"), 0o700)
	target = write(t, filepath.Join(dir, "c", "target"), "one\n", 0o644)
	symlink(t, filepath.Join("..", "target"), filepath.Join(dir, "c", "d", "link"))
	symlink(t, filepath.Join("c", "d"), filepath.Join(dir, "alias"))
	return symlink(t, filepath.Join("alias", "link"), filepath.Join(dir, "entry")), target
}

// A write through a symbolic link changes the file the link points at
// and leaves the link a link, rather than turning a link to a shared file
// into a private copy.
func TestWriteAtomicWritesThroughALink(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		layout func(t *testing.T) (link, target string)
	}{
		{"a relative link", func(t *testing.T) (string, string) {
			dir := t.TempDir()
			return symlink(t, "real", filepath.Join(dir, "link")), write(t, filepath.Join(dir, "real"), "one\n", 0o644)
		}},
		{"an absolute link", func(t *testing.T) (string, string) {
			dir := t.TempDir()
			target := write(t, filepath.Join(dir, "real"), "one\n", 0o644)
			return symlink(t, target, filepath.Join(dir, "link")), target
		}},
		{"a link to a file that is not there yet", func(t *testing.T) (string, string) {
			dir := t.TempDir()
			return symlink(t, filepath.Join("new", "real"), filepath.Join(dir, "link")), filepath.Join(dir, "new", "real")
		}},
		{"a relative link behind a linked directory", linkedTarget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			link, target := tc.layout(t)
			if err := fsutil.WriteAtomic(link, []byte("two\n"), 0o644); err != nil {
				t.Fatalf("WriteAtomic: %v", err)
			}
			if got := content(target); got != "two\n" {
				t.Errorf("the target holds %q, want %q", got, "two\n")
			}
			if info, err := os.Lstat(link); err != nil || info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("the link was replaced: %v", err)
			}
			stray := filepath.Join(filepath.Dir(link), "target")
			if _, err := os.Lstat(stray); target != stray && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("a stray %s was written: %v", stray, err)
			}
		})
	}
}

// A write that fails leaves the file as it was, and no temporary file.
func TestWriteAtomicFails(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path func(t *testing.T, dir string) string
		want error
	}{
		{"a loop of links", func(t *testing.T, dir string) string {
			symlink(t, "b", filepath.Join(dir, "a"))
			return symlink(t, "a", filepath.Join(dir, "b"))
		}, nil},
		{"a file where its directory should be", func(t *testing.T, dir string) string {
			return filepath.Join(write(t, filepath.Join(dir, "file"), "keep", 0o600), "state")
		}, syscall.ENOTDIR},
		{"a directory that cannot be made", func(t *testing.T, dir string) string {
			return filepath.Join(symlink(t, filepath.Join("nowhere", "x"), filepath.Join(dir, "dangling")), "state")
		}, fs.ErrExist},
		{"a name too long for a temporary file beside it", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(dir, strings.Repeat("x", 250)), "keep", 0o600)
		}, syscall.ENAMETOOLONG},
		{"a directory at the path", func(t *testing.T, dir string) string {
			path := mkdir(t, filepath.Join(dir, "state"), 0o700)
			write(t, filepath.Join(path, "file"), "keep", 0o600)
			return path
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := tc.path(t, dir)
			err := fsutil.WriteAtomic(path, []byte("new"), 0o600)
			if err == nil {
				t.Fatal("WriteAtomic succeeded")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), "write "+path+": ") {
				t.Errorf("err = %q, which does not name the path", err)
			}
			for _, name := range []string{"file", strings.Repeat("x", 250), filepath.Join("state", "file")} {
				if got := content(filepath.Join(dir, name)); got != "keep" && !strings.Contains(got, "no such file") {
					t.Errorf("%s holds %q, want it kept", name, got)
				}
			}
			noTemporaryFiles(t, dir)
		})
	}
}

func TestEnsureDir(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		dir  func(t *testing.T, base string) string
		want error // nil for a directory accepted
	}{
		{"a new directory and its parents", func(_ *testing.T, base string) string {
			return filepath.Join(base, "a", "b")
		}, nil},
		{"a private directory", func(t *testing.T, base string) string {
			return mkdir(t, filepath.Join(base, "state"), 0o700)
		}, nil},
		{"a directory others may read", func(t *testing.T, base string) string {
			return mkdir(t, filepath.Join(base, "state"), 0o755)
		}, nil},
		{"a directory others may write", func(t *testing.T, base string) string {
			return mkdir(t, filepath.Join(base, "state"), 0o777)
		}, fsutil.ErrUntrusted},
		{"a directory its group may write", func(t *testing.T, base string) string {
			return mkdir(t, filepath.Join(base, "state"), 0o770)
		}, fsutil.ErrUntrusted},
		{"a sticky directory others may write", func(t *testing.T, base string) string {
			return mkdir(t, filepath.Join(base, "state"), 0o777|fs.ModeSticky)
		}, fsutil.ErrUntrusted},
		{"a link to a private directory", func(t *testing.T, base string) string {
			mkdir(t, filepath.Join(base, "real"), 0o700)
			return symlink(t, "real", filepath.Join(base, "state"))
		}, fsutil.ErrUntrusted},
		{"a file", func(t *testing.T, base string) string {
			return write(t, filepath.Join(base, "state"), "", 0o600)
		}, syscall.ENOTDIR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tc.dir(t, t.TempDir())
			err := fsutil.EnsureDir(dir)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("EnsureDir: %v", err)
				}
				if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
					t.Errorf("no directory was made: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("a new directory is private", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "a", "b")
		if err := fsutil.EnsureDir(dir); err != nil {
			t.Fatalf("EnsureDir: %v", err)
		}
		for _, d := range []string{dir, filepath.Dir(dir)} {
			if got := mode(t, d); got != 0o700 {
				t.Errorf("%s has mode %v, want 0700", d, got)
			}
		}
	})
}

// errLoop stands for the error of a loop of links, which matches no
// error value.
var errLoop = errors.New("too many levels of symbolic links")

func TestCheckTrusted(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path func(t *testing.T, dir string) string
		want error // nil for a path accepted
	}{
		{"a private file", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(dir, "config"), "", 0o600)
		}, nil},
		{"a file others may read", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(dir, "config"), "", 0o644)
		}, nil},
		{"a file its group may write", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(dir, "config"), "", 0o664)
		}, fsutil.ErrUntrusted},
		{"a file others may write", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(dir, "config"), "", 0o646)
		}, fsutil.ErrUntrusted},
		{"a directory", func(t *testing.T, dir string) string {
			return mkdir(t, filepath.Join(dir, "conf.d"), 0o755)
		}, nil},
		{"a sticky directory others may write", func(t *testing.T, dir string) string {
			return mkdir(t, filepath.Join(dir, "tmp"), 0o777|fs.ModeSticky)
		}, fsutil.ErrUntrusted},
		{"a file in a sticky directory others may write", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(mkdir(t, filepath.Join(dir, "tmp"), 0o777|fs.ModeSticky), "config"), "", 0o600)
		}, nil},
		{"a file in a directory others may write", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(mkdir(t, filepath.Join(dir, "open"), 0o777), "config"), "", 0o600)
		}, fsutil.ErrUntrusted},
		{"a file below a directory its group may write", func(t *testing.T, dir string) string {
			mkdir(t, filepath.Join(dir, "open", "sub"), 0o700)
			mkdir(t, filepath.Join(dir, "open"), 0o770)
			return write(t, filepath.Join(dir, "open", "sub", "config"), "", 0o600)
		}, fsutil.ErrUntrusted},
		{"a missing file", func(_ *testing.T, dir string) string {
			return filepath.Join(dir, "config")
		}, fs.ErrNotExist},
		{"a link to a private file", func(t *testing.T, dir string) string {
			write(t, filepath.Join(dir, "config"), "", 0o600)
			return symlink(t, "./config", filepath.Join(dir, "link"))
		}, nil},
		{"an absolute link to a private file", func(t *testing.T, dir string) string {
			return symlink(t, write(t, filepath.Join(dir, "config"), "", 0o600), filepath.Join(dir, "link"))
		}, nil},
		{"a link to a file others may write", func(t *testing.T, dir string) string {
			write(t, filepath.Join(dir, "config"), "", 0o666)
			return symlink(t, "config", filepath.Join(dir, "link"))
		}, fsutil.ErrUntrusted},
		{"a link into a directory others may write", func(t *testing.T, dir string) string {
			open := mkdir(t, filepath.Join(dir, "a", "open"), 0o700)
			write(t, filepath.Join(open, "config"), "", 0o600)
			mkdir(t, open, 0o777)
			mkdir(t, filepath.Join(dir, "b"), 0o700)
			return symlink(t, filepath.Join("..", "a", "open", "config"), filepath.Join(dir, "b", "link"))
		}, fsutil.ErrUntrusted},
		{"a link in a directory others may write", func(t *testing.T, dir string) string {
			write(t, filepath.Join(dir, "config"), "", 0o600)
			open := mkdir(t, filepath.Join(dir, "open"), 0o700)
			symlink(t, filepath.Join("..", "config"), filepath.Join(open, "link"))
			mkdir(t, open, 0o777)
			return filepath.Join(open, "link")
		}, fsutil.ErrUntrusted},
		{"a relative link behind a linked directory", func(t *testing.T, _ string) string {
			entry, _ := linkedTarget(t)
			return entry
		}, nil},
		{"a link above the root", func(t *testing.T, dir string) string {
			return symlink(t, strings.Repeat("../", 64), filepath.Join(dir, "link"))
		}, nil},
		{"a loop of links", func(t *testing.T, dir string) string {
			symlink(t, "b", filepath.Join(dir, "a"))
			return symlink(t, "a", filepath.Join(dir, "b"))
		}, errLoop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := tc.path(t, t.TempDir())
			err := fsutil.CheckTrusted(path)
			switch {
			case tc.want == nil && err != nil:
				t.Errorf("CheckTrusted: %v", err)
			case tc.want != nil && err == nil:
				t.Errorf("CheckTrusted accepted it, want %v", tc.want)
			case errors.Is(tc.want, errLoop) && !strings.Contains(err.Error(), "too many levels of symbolic links"):
				t.Errorf("err = %v, want too many links", err)
			case tc.want != nil && !errors.Is(tc.want, errLoop) && !errors.Is(err, tc.want):
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// The root, and a path relative to the working directory, are looked up
// as the system looks them up.
func TestCheckTrustedRootAndRelative(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "config"), "", 0o600)
	t.Chdir(dir)
	for _, path := range []string{"/", "config", "./config", filepath.Join("..", filepath.Base(dir), "config")} {
		if err := fsutil.CheckTrusted(path); err != nil {
			t.Errorf("CheckTrusted(%q): %v", path, err)
		}
	}
}
