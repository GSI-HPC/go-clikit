// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !unix || aix

package launch

import (
	"context"
	"errors"
	"testing"
)

func TestAcquireIsUnsupported(t *testing.T) {
	t.Parallel()

	if i, err := Acquire(context.Background(), t.TempDir(), "prog"); i != nil || !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("Acquire = %v, %v, want errors.ErrUnsupported", i, err)
	}
	if err := checkPrivateDir(t.TempDir(), 0); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("checkPrivateDir = %v, want errors.ErrUnsupported", err)
	}
	if err := (&Instance{}).Retire(context.Background()); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("Retire = %v, want errors.ErrUnsupported", err)
	}
}
