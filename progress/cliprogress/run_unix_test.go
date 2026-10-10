// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cliprogress_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// A log that cannot be written once the command runs fails nothing, and a
// line says once that it stops short, after the display has gone.
func TestALogThatStopsShortSaysSoOnce(t *testing.T) {
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer wr.Close()
	_ = rd.Close()
	screen := &progresstest.Screen{Width: 80}
	c := newClock()
	o := terminal(screen, c)
	o.Log.Given, o.Log.Value = true, fmt.Sprintf("/dev/fd/%d", wr.Fd())
	r, err := cliprogress.Start(context.Background(), o)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	command(r, c, nil)
	r.Finish(false)
	if got := screen.String(); strings.Count(got, "prog: the progress log /dev/fd/") != 1 || !strings.Contains(got, "stops short") {
		t.Errorf("the terminal shows\n%s", got)
	}
}
