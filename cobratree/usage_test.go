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

func TestUsage(t *testing.T) {
	if err := cobratree.Usage(nil); err != nil {
		t.Errorf("Usage(nil) = %v, want nil", err)
	}
	for _, tc := range []struct {
		name  string
		err   error
		usage bool
		text  string
	}{
		{"nil", nil, false, ""},
		{"plain", fs.ErrNotExist, false, "file does not exist"},
		{"marked", cobratree.Usage(fs.ErrNotExist), true, "file does not exist"},
		{"wrapped", fmt.Errorf("parsing: %w", cobratree.Usage(fs.ErrNotExist)), true, "parsing: file does not exist"},
		{"joined", errors.Join(fs.ErrClosed, cobratree.Usage(fs.ErrNotExist)), true, "file already closed\nfile does not exist"},
		{"formatted", cobratree.Usagef("--count: %w", fs.ErrInvalid), true, "--count: invalid argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cobratree.IsUsage(tc.err); got != tc.usage {
				t.Errorf("IsUsage(%v) = %v, want %v", tc.err, got, tc.usage)
			}
			if tc.err != nil && tc.err.Error() != tc.text {
				t.Errorf("text %q, want %q", tc.err.Error(), tc.text)
			}
		})
	}
	if err := cobratree.Usagef("--count: %w", fs.ErrInvalid); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("errors.Is(%v, fs.ErrInvalid) = false, want true", err)
	}
}
