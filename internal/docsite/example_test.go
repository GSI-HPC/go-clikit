// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package docsite_test

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
	"github.com/GSI-HPC/go-clikit/progress/display"
)

// The examples of this file are the code of the site's guides that no
// package's examples show: the main of a program, which reads its
// environment and its signals. They are compiled, not run.

// reinstall is the library code of the guide: it reports its work as
// spans in ctx, and knows no display.
func reinstall(ctx context.Context, nodes []string) error {
	_, err := fanout.Map(ctx, nodes, fanout.MapOptions[string]{Step: "reinstalling the nodes", Limit: 4},
		func(_ context.Context, _ string) (struct{}, error) {
			return struct{}{}, nil // the work on one node
		})
	return err
}

// A program reads its flag, its variable and its signals, asks the system
// about standard error, and hands cliprogress what it found: the kit reads
// none of it itself.
func Example_mainWithProgress() {
	flags := flag.NewFlagSet("clusterctl", flag.ContinueOnError)
	mode := flags.String("progress", "", "how to show progress: auto, tty, counter, plain or none")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return
	}
	given := false
	flags.Visit(func(f *flag.Flag) { given = given || f.Name == "progress" })

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	theme, _ := display.ParseTheme("tide")
	onTerminal := cliprogress.IsTerminal(os.Stderr)
	session, err := cliprogress.Start(ctx, cliprogress.Options{
		Program: "clusterctl",
		Mode: cliprogress.Setting{
			Flag: "--progress", Given: given, Value: *mode,
			Variable: "CLUSTERCTL_PROGRESS", Env: os.Getenv("CLUSTERCTL_PROGRESS"),
		},
		Stderr:     os.Stderr,
		OnTerminal: onTerminal,
		Dumb:       os.Getenv("TERM") == "dumb",
		Size:       func() (int, int, error) { return cliprogress.TerminalSize(os.Stderr) },
		Foreground: func() bool { return cliprogress.InForeground(os.Stderr) },
		ASCII:      !cliprogress.UTF8Locale(os.Getenv),
		Theme:      cliprogress.ThemeFor(theme, onTerminal, os.Getenv),
		Noun: func(n int) string {
			if n == 1 {
				return "host"
			}
			return "hosts"
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "clusterctl:", err) // a usage error
		return
	}
	// The logger's lines go through the display while it draws.
	log.SetOutput(session.Lines(os.Stderr))

	ctx, cmd := progress.Start(session.Context(), progress.KindCommand, "reinstall")
	err = reinstall(ctx, flags.Args())
	cmd.End(err)
	session.Finish(err != nil)
	log.SetOutput(os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "clusterctl:", err)
	}
}
