// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree_test

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/GSI-HPC/go-clikit/cobratree"
)

func TestCoded(t *testing.T) {
	if err := cobratree.Coded(3, nil); err != nil {
		t.Errorf("Coded(3, nil) = %v, want nil", err)
	}
	for _, tc := range []struct {
		name   string
		err    error
		status int
		text   string
	}{
		{"nil", nil, 0, ""},
		{"plain", fs.ErrNotExist, 1, "file does not exist"},
		{"coded", cobratree.Coded(3, fs.ErrNotExist), 3, "file does not exist"},
		{"wrapped", fmt.Errorf("n1: %w", cobratree.Coded(3, fs.ErrNotExist)), 3, "n1: file does not exist"},
		{"coded again", cobratree.Coded(4, fmt.Errorf("n1: %w", cobratree.Coded(3, fs.ErrNotExist))), 3, "n1: file does not exist"},
		{"joined", errors.Join(fs.ErrClosed, cobratree.Coded(3, fs.ErrNotExist)), 3, "file already closed\nfile does not exist"},
		{"zero", cobratree.Coded(0, fs.ErrNotExist), 0, "file does not exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cobratree.Status(tc.err, 1); got != tc.status {
				t.Errorf("Status(%v, 1) = %d, want %d", tc.err, got, tc.status)
			}
			if tc.err != nil && tc.err.Error() != tc.text {
				t.Errorf("text %q, want %q", tc.err.Error(), tc.text)
			}
		})
	}
}

// A coded error is still what it wraps, a usage error among them.
func TestCodedLooksThrough(t *testing.T) {
	err := cobratree.Coded(64, cobratree.Usage(fs.ErrInvalid))
	if !errors.Is(err, fs.ErrInvalid) || !cobratree.IsUsage(err) {
		t.Errorf("%v: errors.Is %v, IsUsage %v; want both", err, errors.Is(err, fs.ErrInvalid), cobratree.IsUsage(err))
	}
}
