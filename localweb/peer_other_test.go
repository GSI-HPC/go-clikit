// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package localweb

import (
	"errors"
	"net"
	"testing"
)

func TestPeerListenerIsUnsupported(t *testing.T) {
	t.Parallel()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	pl, err := PeerListener(l, 0, nil)

	if pl != nil || !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("PeerListener = %v, %v, want errors.ErrUnsupported", pl, err)
	}
}
