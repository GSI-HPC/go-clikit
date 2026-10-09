// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// exampleClock is a clock the example moves on by hand.
type exampleClock struct{ now time.Time }

// newExampleClock returns a clock at noon.
func newExampleClock() *exampleClock {
	return &exampleClock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
}

func (c *exampleClock) Now() time.Time      { return c.now }
func (c *exampleClock) Add(d time.Duration) { c.now = c.now.Add(d) }

// startTargets announces a target for each node, queued, as a pool does
// before it runs the first.
func startTargets(ctx context.Context, nodes ...string) []*progress.Span {
	targets := make([]*progress.Span, len(nodes))
	for i, node := range nodes {
		_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	return targets
}

// Plain writes a line for each thing worth one, for a CI log; the Summary
// is the line a command prints once it has ended. The noun names the
// targets, which are nodes here.
func ExampleNewPlain() {
	clock := newExampleClock()
	term := display.NewTerminal(os.Stdout, display.TerminalOptions{})
	plain := display.NewPlain(term, display.PlainOptions{
		Now: clock.Now,
		Noun: func(n int) string {
			if n == 1 {
				return "1 node"
			}
			return fmt.Sprintf("%d nodes", n)
		},
	})
	summary := &display.Summary{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{plain, summary}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "create cluster")

	ctx, step := progress.Start(ctx, progress.KindStep, "starting the nodes",
		progress.WithFlags(progress.Fold), progress.Total(3), progress.Limit(3))
	targets := startTargets(ctx, "worker0", "worker1", "worker2")
	for _, target := range targets {
		target.Run()
	}
	clock.Add(3 * time.Second)
	failed := errors.New("container exited with 1")
	targets[1].End(failed)
	clock.Add(7 * time.Second)
	plain.Draw()
	clock.Add(2 * time.Second)
	targets[0].End(nil)
	targets[2].End(nil)
	step.End(failed)
	command.End(failed)
	bus.Close()
	plain.Close()
	fmt.Println(summary.Line())
	// Output:
	// [0:00] create cluster › starting the nodes: start, 3 nodes
	// [0:03] create cluster › starting the nodes › worker1 failed (target): container exited with 1
	// [0:10] create cluster › starting the nodes: 1/3 done, 1 failed, 2 running
	// [0:12] create cluster › starting the nodes: failed in 12s: 2 ok, 1 failed
	// create cluster: failed in 12s: 2 ok, 1 failed
}

// The Tree draws the work under way at the bottom of a terminal, redrawn
// as it goes on; a Screen shows what a terminal would.
func ExampleNewTree() {
	clock := newExampleClock()
	screen := &progresstest.Screen{Width: 60}
	term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return 60, 24, nil }})
	tree := display.NewTree(term, display.TreeOptions{Now: clock.Now})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{tree}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "power on")

	ctx, step := progress.Start(ctx, progress.KindStep, "powering on",
		progress.WithFlags(progress.Fold), progress.Total(4), progress.Limit(2))
	nodes := []string{"exe01", "exe02", "exe03", "exe04"}
	ctxs := make([]context.Context, len(nodes))
	targets := make([]*progress.Span, len(nodes))
	for i, node := range nodes {
		ctxs[i], targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	for _, target := range targets[:2] {
		target.Run()
		target.End(nil)
	}
	targets[2].Run()
	_, call := progress.Start(ctxs[2], progress.KindCall, "redfish",
		progress.HTTP("POST", "/redfish/v1/Systems/1"), progress.Timeout(30*time.Second))
	clock.Add(2 * time.Second)
	tree.Draw()
	fmt.Print(screen.String())
	fmt.Println("---")

	// Once the work is done, the region is gone and the line of the
	// finished step is left.
	call.End(nil)
	targets[2].End(nil)
	targets[3].Run()
	targets[3].End(nil)
	step.End(nil)
	command.End(nil)
	bus.Close()
	tree.Close()
	fmt.Print(screen.String())
	// Output:
	// power on · 0:02.0
	//   powering on  2/4 · 1 running · 1 queued
	//     ▸ exe03  2.0s/30s  POST /redfish/v1/Systems/1
	//     ✓ exe[01-02]
	// ---
	// ✓ powering on  2.0s  4 ok
}

// The Counter draws one line, for a terminal too small for the tree or a
// user who wants no more.
func ExampleNewCounter() {
	clock := newExampleClock()
	screen := &progresstest.Screen{}
	term := display.NewTerminal(screen, display.TerminalOptions{})
	counter := display.NewCounter(term, display.CounterOptions{Now: clock.Now})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{counter}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "uptime")

	ctx, step := progress.Start(ctx, progress.KindStep, "uptime",
		progress.WithFlags(progress.Fold), progress.Total(3), progress.Limit(8))
	targets := startTargets(ctx, "exe001", "exe002", "exe003")
	targets[0].Run()
	targets[0].End(nil)
	targets[1].Run()
	targets[1].End(errors.New("no answer"))
	targets[2].Run()
	clock.Add(41300 * time.Millisecond)
	counter.Draw()
	fmt.Println(screen.String())

	targets[2].End(nil)
	step.End(nil)
	command.End(nil)
	bus.Close()
	counter.Close()
	// Output:
	// uptime · 2/3 · 1 failed · 1 running · 0:41.3
}

// A program takes the theme from a flag, which the text methods of a Theme
// read, and draws it in as many colours as its terminal shows: here in
// none, as under NO_COLOR, which keeps the theme's art.
func ExampleTheme() {
	flags := flag.NewFlagSet("clusterctl", flag.ContinueOnError)
	var theme display.Theme
	flags.TextVar(&theme, "theme", display.Classic, "how progress looks")
	if err := flags.Parse([]string{"-theme", "aurora"}); err != nil {
		fmt.Println(err)
		return
	}
	noColor := true // the program read NO_COLOR from its environment
	if noColor {
		theme = theme.In(display.NoColours)
	}
	fmt.Println(theme, theme.Colours())
	fmt.Println(display.Themes())
	// Output:
	// aurora none
	// [classic aurora ember neon tide]
}

// ParseTheme reads a theme's name in any case, and says which names there
// are when it reads none.
func ExampleParseTheme() {
	theme, err := display.ParseTheme("Tide")
	fmt.Println(theme, theme.Colours(), err)
	_, err = display.ParseTheme("dawn")
	fmt.Println(err)
	// Output:
	// tide 256 <nil>
	// display: unknown theme "dawn": want none, classic, aurora, ember, neon or tide
}

// The Tree in a theme draws its art, here Tide's spinner, bar and guides,
// in the theme's colours, which a Screen with Styles shows as «p»text«».
func ExampleNewTree_theme() {
	clock := newExampleClock()
	screen := &progresstest.Screen{Width: 60, Styles: true}
	term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return 60, 24, nil }})
	tree := display.NewTree(term, display.TreeOptions{Now: clock.Now, Theme: display.Tide})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{tree}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "power on")

	ctx, step := progress.Start(ctx, progress.KindStep, "powering on",
		progress.WithFlags(progress.Fold), progress.Total(4), progress.Limit(2))
	targets := startTargets(ctx, "exe01", "exe02", "exe03", "exe04")
	for _, target := range targets[:2] {
		target.Run()
		target.End(nil)
	}
	targets[2].Run()
	clock.Add(2 * time.Second)
	tree.Draw()
	fmt.Print(screen.String())
	fmt.Print(screen.Styled())

	targets[2].End(errors.New("no answer"))
	targets[3].Run()
	targets[3].End(nil)
	step.End(errors.New("1 of 4 failed"))
	command.End(nil)
	bus.Close()
	tree.Close()
	// Output:
	// power on ◦ 0:02.0
	// ╎ powering on  ◉◉◉◉◉◌◌◌◌◌ 2/4 ◦ 1 running ◦ 1 queued
	//   ╎ ◡ exe03  2.0s
	//   ╎ ✓ exe[01-02]
	// «1»power on«38;5;66» ◦ «38;5;31»0:02.0«»
	// «38;5;66»╎ «1»powering on«»  «38;5;32»◉◉◉◉◉«38;5;66»◌◌◌◌◌«» 2/4«38;5;66» ◦ «38;5;29»1 running«38;5;66» ◦ 1 queued«»
	//   «38;5;66»╎ «38;5;29»◡«» exe03  «38;5;66»2.0s«»
	//   «38;5;66»╎ «1;38;5;32»✓«» exe[01-02]
}

// Plain lines in a theme take its colours and one mark after the time,
// here Ember's in the terminal's own 16 colours, and keep their words; so
// does the Summary in the same theme.
func ExampleNewPlain_theme() {
	clock := newExampleClock()
	screen := &progresstest.Screen{Styles: true}
	term := display.NewTerminal(screen, display.TerminalOptions{})
	theme := display.Ember.In(display.Colours16)
	plain := display.NewPlain(term, display.PlainOptions{Now: clock.Now, Theme: theme})
	summary := &display.Summary{Theme: theme}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{plain, summary}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "create cluster")

	ctx, step := progress.Start(ctx, progress.KindStep, "starting the nodes",
		progress.WithFlags(progress.Fold), progress.Total(2))
	targets := startTargets(ctx, "worker0", "worker1")
	for _, target := range targets {
		target.Run()
	}
	clock.Add(3 * time.Second)
	failed := errors.New("container exited with 1")
	targets[1].End(failed)
	targets[0].End(nil)
	step.End(failed)
	command.End(failed)
	bus.Close()
	plain.Close()
	_, _ = fmt.Fprintln(screen, summary.Line())
	fmt.Print(screen.Styled())
	// Output:
	// «33»[0:00]«» «33»▮«» create cluster«2» › «»starting the nodes: «33»start«», 2 targets
	// «33»[0:03]«» «31»✘«» create cluster«2» › «»starting the nodes«2» › «»worker1 «31»failed (target)«»: container exited with 1
	// «33»[0:03]«» «31»✘«» create cluster«2» › «»starting the nodes: «31»failed«» in «2»3.0s«»: «1»1 ok«», «31»1 failed«»
	// «31»✘«» «1»create cluster«»: «31»failed«» in «2»3.0s«»: «1»1 ok«», «31»1 failed«»
}

// The Counter in a theme starts its line with the theme's spinner, and
// draws a bar of each counted step's targets before their count, those
// that failed at its end in the colour of a failure: here Neon's, in 256
// colours.
func ExampleNewCounter_theme() {
	clock := newExampleClock()
	screen := &progresstest.Screen{Styles: true}
	term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return 80, 24, nil }})
	counter := display.NewCounter(term, display.CounterOptions{Now: clock.Now, Theme: display.Neon})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{counter}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "uptime")

	ctx, step := progress.Start(ctx, progress.KindStep, "uptime", progress.WithFlags(progress.Fold), progress.Total(4))
	targets := startTargets(ctx, "exe001", "exe002", "exe003", "exe004")
	for _, target := range targets[:3] {
		target.Run()
	}
	targets[0].End(nil)
	targets[1].End(errors.New("no answer"))
	clock.Add(41300 * time.Millisecond)
	counter.Draw()
	fmt.Print(screen.String())
	fmt.Print(screen.Styled())

	targets[2].End(nil)
	targets[3].Run()
	targets[3].End(nil)
	step.End(nil)
	command.End(nil)
	bus.Close()
	counter.Close()
	// Output:
	// ◶ uptime ⋄ ▰▰▰▰▱▱▱▱ 2/4 ⋄ 1 failed ⋄ 1 running ⋄ 1 queued ⋄ 0:41.3
	// «38;5;135»◶«» «1»uptime«38;5;103» ⋄ «38;5;33»▰▰«38;5;197»▰▰«38;5;103»▱▱▱▱«» 2/4«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»1 running«38;5;103» ⋄ 1 queued ⋄ «1;38;5;134»0:41.3«»
}

// The Summary in a theme starts with the mark of how the command ended, in
// its colour: here Tide's, in none, as under NO_COLOR, which keeps the mark.
func ExampleSummary_theme() {
	clock := newExampleClock()
	summary := &display.Summary{Theme: display.Tide.In(display.NoColours)}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{summary}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "drain")

	ctx, step := progress.Start(ctx, progress.KindStep, "draining", progress.WithFlags(progress.Fold), progress.Total(2))
	targets := startTargets(ctx, "exe001", "exe002")
	targets[0].Run()
	targets[0].End(nil)
	targets[1].Run()
	clock.Add(2 * time.Second)
	targets[1].End(context.Canceled)
	step.End(context.Canceled)
	command.End(context.Canceled)
	bus.Close()
	fmt.Println(summary.Line())
	// Output:
	// ⊖ drain: canceled in 2.0s: 1 ok, 1 canceled
}
