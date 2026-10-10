// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
)

// The flag's word is refused where it cannot be had; the variable's shows
// nothing there, and says so only for a word it does not take.
func ExampleChoose() {
	for _, mode := range []cliprogress.Setting{
		{Flag: "--progress", Given: true, Value: "tty"},
		{Variable: "PROG_PROGRESS", Env: "tty"},
		{Variable: "PROG_PROGRESS", Env: "tree"},
		{Flag: "--progress", Given: true, Value: "plain"},
	} {
		mode, note, err := cliprogress.Choose(cliprogress.Options{
			Program:    "prog",
			Mode:       mode,
			OnTerminal: false, // standard error goes into a pipe
		})
		fmt.Printf("%v %q %v\n", mode, note, err)
	}
	// Output:
	// none "" --progress asks for a live tree, but standard error is not a terminal; use none, or auto to draw one only where it can be
	// none "" <nil>
	// none "prog: PROG_PROGRESS is \"tree\"; it takes one of auto, tty, counter, plain, none; no progress is shown" <nil>
	// plain "" <nil>
}

// A command whose progress --progress plain shows, as lines on standard
// error, ends with the summary the program asks for, since it failed.
func ExampleStart() {
	clock := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	r, err := cliprogress.Start(context.Background(), cliprogress.Options{
		Program: "prog",
		Mode:    cliprogress.Setting{Flag: "--progress", Given: true, Value: "plain"},
		Stderr:  os.Stdout,
		Now:     func() time.Time { return clock },
	})
	if err != nil {
		fmt.Println(err) // a usage error
		return
	}
	ctx, cmd := progress.Start(r.Context(), progress.KindCommand, "delete cluster")
	_, step := progress.Start(ctx, progress.KindStep, "mesh")
	clock = clock.Add(2 * time.Second)
	failed := errors.New("network sind-mesh has active endpoints")
	step.End(failed)
	cmd.End(failed)
	r.Finish(true)
	// Output:
	// [0:00] delete cluster › mesh: start
	// [0:02] delete cluster › mesh: failed in 2.0s
	// prog: delete cluster: failed in 2.0s
}
