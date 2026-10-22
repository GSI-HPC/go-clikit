// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package localweb

import (
	"strings"
	"testing"
)

// procfs belongs to the process's user and is closed to others, so the
// directory passes the checks, and nothing can be made in it.
func TestListenFailsToMakeAStagingDirectory(t *testing.T) {
	t.Parallel()

	b, err := Listen(t.Context(), UnixPrefix+"/proc/self/x.sock", ListenOptions{})

	if err == nil {
		_ = b.Close()
		t.Fatal("Listen made a socket in procfs")
	}
	if !strings.HasPrefix(err.Error(), "make a directory to bind the socket in: ") {
		t.Errorf("error = %v", err)
	}
}
