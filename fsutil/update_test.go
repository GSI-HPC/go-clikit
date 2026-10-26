// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package fsutil_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/GSI-HPC/go-clikit/fsutil"
)

// Updates that run at once do not lose a write, which an unlocked append
// to a shared file does.
func TestUpdateSerialises(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "known_hosts")
	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for range writers {
		wg.Go(func() {
			errs <- fsutil.Update(context.Background(), path, 0o600, func(current []byte) ([]byte, error) {
				return append(current, 'x'), nil
			})
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if got := content(path); got != strings.Repeat("x", writers) {
		t.Errorf("the file holds %q, want %d bytes; a write was lost", got, writers)
	}
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	errChange := errors.New("change refused")
	for _, tc := range []struct {
		name    string
		path    func(t *testing.T, dir string) string
		change  func([]byte) ([]byte, error)
		want    string // the content of dir/file afterwards
		wantErr error
	}{
		{"a missing file is nil", func(_ *testing.T, dir string) string {
			return filepath.Join(dir, "file")
		}, func(b []byte) ([]byte, error) {
			if b != nil {
				return nil, errors.New("not nil")
			}
			return []byte("new"), nil
		}, "new", nil},
		{"a file is changed", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(dir, "file"), "old", 0o600)
		}, func(b []byte) ([]byte, error) { return append(b, "+new"...), nil }, "old+new", nil},
		{"a file is changed through a link", func(t *testing.T, dir string) string {
			write(t, filepath.Join(dir, "file"), "old", 0o600)
			return symlink(t, "file", filepath.Join(dir, "link"))
		}, func(b []byte) ([]byte, error) { return append(b, "+new"...), nil }, "old+new", nil},
		{"a change that fails writes nothing", func(t *testing.T, dir string) string {
			return write(t, filepath.Join(dir, "file"), "old", 0o600)
		}, func([]byte) ([]byte, error) { return []byte("new"), errChange }, "old", errChange},
		{"a change that fails makes no file", func(_ *testing.T, dir string) string {
			return filepath.Join(dir, "file")
		}, func([]byte) ([]byte, error) { return nil, errChange }, "no such file", errChange},
		{"a loop of links", func(t *testing.T, dir string) string {
			symlink(t, "b", filepath.Join(dir, "a"))
			return symlink(t, "a", filepath.Join(dir, "b"))
		}, nil, "no such file", nil},
		{"a directory cannot be read", func(t *testing.T, dir string) string {
			return mkdir(t, filepath.Join(dir, "dir"), 0o700)
		}, nil, "no such file", syscall.EISDIR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := tc.path(t, dir)
			err := fsutil.Update(context.Background(), path, 0o600, tc.change)
			switch {
			case tc.change == nil && err == nil:
				t.Error("Update succeeded")
			case tc.wantErr != nil && !errors.Is(err, tc.wantErr):
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			case tc.change != nil && tc.wantErr == nil && err != nil:
				t.Errorf("Update: %v", err)
			}
			if got := content(filepath.Join(dir, "file")); !strings.Contains(got, tc.want) {
				t.Errorf("the file holds %q, want %q", got, tc.want)
			}
			noTemporaryFiles(t, dir)
		})
	}
}

// Update waits for a lock someone else holds, until its context ends.
func TestUpdateWaitsForTheLock(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "file")
	unlock, err := fsutil.Lock(context.Background(), path, 0)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = fsutil.Update(ctx, path, 0o600, func([]byte) ([]byte, error) {
		t.Error("change was called without the lock")
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// Update hands its options to the write: with ExactMode, a file whose
// mode was widened is private again.
func TestUpdateExactMode(t *testing.T) {
	t.Parallel()

	path := write(t, filepath.Join(t.TempDir(), "file"), "old", 0o644)
	err := fsutil.Update(context.Background(), path, 0o600, func(b []byte) ([]byte, error) {
		return append(b, "+new"...), nil
	}, fsutil.ExactMode())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := content(path); got != "old+new" {
		t.Errorf("the file holds %q, want %q", got, "old+new")
	}
	if got := mode(t, path); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}
}
