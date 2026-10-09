// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
	"github.com/GSI-HPC/go-nodeset"
)

// counterThemeFixture is a counter drawn in a theme on a Screen that shows
// its colours, as wide as the terminal says it is, fed by a Bus whose
// events are checked when the test ends, for a command that started when
// the counter was made.
type counterThemeFixture struct {
	ctx     context.Context
	screen  *progresstest.Screen
	term    *display.Terminal
	counter *display.Counter
	clock   *clock
}

// counterThemeNew returns a counterThemeFixture for command on a terminal
// width columns wide, its counter made with o and the fixture's clock.
func counterThemeNew(t *testing.T, command string, width int, o display.CounterOptions) *counterThemeFixture {
	t.Helper()
	f := &counterThemeFixture{
		screen: &progresstest.Screen{Width: width, Styles: true},
		clock:  &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)},
	}
	f.term = display.NewTerminal(f.screen, display.TerminalOptions{Size: func() (int, int, error) { return width, 24, nil }})
	o.Now = f.clock.Now
	f.counter = display.NewCounter(f.term, o)
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture, f.counter}, Now: f.clock.Now})
	t.Cleanup(func() {
		bus.Close()
		f.counter.Close()
		progresstest.Check(t, capture.Events())
	})
	if command != "" {
		f.ctx, _ = progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, command)
	}
	return f
}

// draw moves the clock on by d, draws a frame, and returns the line the
// screen shows, with its colours as Styled shows them.
func (f *counterThemeFixture) draw(d time.Duration) string {
	f.clock.Add(d)
	f.counter.Draw()
	return strings.TrimSuffix(f.screen.Styled(), "\n")
}

// text returns the line the screen shows, without its colours.
func (f *counterThemeFixture) text() string { return strings.TrimSuffix(f.screen.String(), "\n") }

// counterThemeStep starts a counted step of n targets under ctx, all
// queued, and returns its targets. The Bus ends what is left open when the
// test ends.
func counterThemeStep(ctx context.Context, name string, n int) []*progress.Span {
	ctx, _ = progress.Start(ctx, progress.KindStep, name, progress.WithFlags(progress.Fold), progress.Total(n))
	targets := make([]*progress.Span, n)
	for i := range targets {
		node := fmt.Sprintf("exe%d", i+1)
		_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	return targets
}

// counterThemeResetTheMachines starts the step "reset the machines" of 8
// targets under ctx, of which 3 have ended, one of them failed, 2 run and
// 3 wait for their turn, and returns the targets.
func counterThemeResetTheMachines(ctx context.Context) []*progress.Span {
	targets := counterThemeStep(ctx, "reset the machines", 8)
	for _, target := range targets[:5] {
		target.Run()
	}
	targets[0].End(nil)
	targets[1].End(errors.New("no answer"))
	targets[2].End(nil)
	return targets
}

// counterThemeEveryCount starts the step "check the nodes" of 6 targets
// under ctx, which counts a target of every kind: one ended well, one
// failed, one canceled and one skipped, one running and one queued, with
// a wait open below it.
func counterThemeEveryCount(ctx context.Context) {
	stepCtx, _ := progress.Start(ctx, progress.KindStep, "check the nodes", progress.WithFlags(progress.Fold), progress.Total(6))
	spans := make([]*progress.Span, 6)
	for i := range spans {
		node := fmt.Sprintf("exe%d", i+1)
		_, spans[i] = progress.Start(stepCtx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	for _, span := range spans[:5] {
		span.Run()
	}
	spans[0].End(nil)
	spans[1].End(errors.New("exe2: no answer"))
	spans[2].End(context.Canceled)
	spans[3].Skip("dry run")
	progress.Start(stepCtx, progress.KindWait, "settle", progress.Message("the BMCs settle"))
}

// counterThemeNoBleed checks that what the command writes after the line
// shows in the terminal's own colour, as it would not if the line had left
// a colour on: no painted part of a line runs on into the output.
func counterThemeNoBleed(t *testing.T, f *counterThemeFixture) {
	t.Helper()
	_, _ = io.WriteString(f.term.Writer(f.screen), "prog: done\n")
	styled := f.screen.Styled()
	if !strings.HasPrefix(styled, "prog: done\n") {
		t.Errorf("after the line, the command's output shows as %q", styled)
	}
}

// In a theme the line says what it says in none, in the theme's art and
// colours: its spinner in front, names bold, the bar of each counted step
// before its count, the part of it that failed in the colour of a failure,
// the counts of targets in the colour of how they stand, separators and
// what is queued muted, the time in the clock's colour. In 256 colours
// Neon's bar runs through its gradient; Classic, here in the terminal's own
// 16 colours, draws no bar and starts with a mark that stands still; Aurora
// in no colours draws its art alone.
func TestCounterThemeDrawsTheLineInATheme(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		theme display.Theme
		want  []string
	}{
		{display.Neon, []string{
			"«38;5;135»◶«» «1»reset the machines«38;5;103» ⋄ «38;5;33»▰▰«38;5;197»▰«38;5;103»▱▱▱▱▱«» 3/8" +
				"«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»2 running«38;5;103» ⋄ 3 queued ⋄ «1;38;5;134»0:02.0«»",
			"«38;5;135»◵«» «1»reset the machines«38;5;103» ⋄ «38;5;33»▰▰«38;5;69»▰«38;5;63»▰«38;5;99»▰▰«38;5;197»▰«38;5;103»▱«» 7/8" +
				"«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»1 running«38;5;103» ⋄ «1;38;5;134»0:02.3«»",
		}},
		{display.Classic.In(display.Colours16), []string{
			"«36»▸«» «1»reset the machines«2» ⋅ «»3/8«2» ⋅ «31»1 failed«2» ⋅ «36»2 running«2» ⋅ 3 queued ⋅ «36»0:02.0«»",
			"«36»▸«» «1»reset the machines«2» ⋅ «»7/8«2» ⋅ «31»1 failed«2» ⋅ «36»1 running«2» ⋅ «36»0:02.3«»",
		}},
		{display.Aurora.In(display.NoColours), []string{
			"⠋ reset the machines ∙ ⣿⣿⣿⣀⣀⣀⣀⣀ 3/8 ∙ 1 failed ∙ 2 running ∙ 3 queued ∙ 0:02.0",
			"⠸ reset the machines ∙ ⣿⣿⣿⣿⣿⣿⣿⣀ 7/8 ∙ 1 failed ∙ 1 running ∙ 0:02.3",
		}},
	} {
		t.Run(tc.theme.String()+"/"+tc.theme.Colours().String(), func(t *testing.T) {
			t.Parallel()
			f := counterThemeNew(t, "provision reinstall", 100, display.CounterOptions{Theme: tc.theme})
			targets := counterThemeResetTheMachines(f.ctx)
			got := []string{f.draw(2 * time.Second)}
			targets[3].End(nil)
			targets[4].End(nil)
			for _, target := range targets[5:7] {
				target.Run()
				target.End(nil)
			}
			targets[7].Run()
			got = append(got, f.draw(300*time.Millisecond))
			check(t, got, tc.want...)
		})
	}
}

// Two counted steps under way side by side each have a segment, with a bar
// of its own, split by the theme's divider, muted.
func TestCounterThemeShowsTwoStepsSideBySide(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		theme display.Theme
		want  string
	}{
		{display.Tide, "«38;5;29»◡«» «1»read the power state«38;5;66» ◦ «38;5;32»◉◉«38;5;66»◌◌◌◌◌◌«» 1/3" +
			"«38;5;66» ◦ «38;5;29»1 running«38;5;66» ◦ 1 queued ╎ " +
			"«1»run«38;5;66» ◦ «38;5;166»◉◉«38;5;66»◌◌◌◌◌◌«» 1/3«38;5;66» ◦ «38;5;166»1 failed«38;5;66» ◦ " +
			"«38;5;29»1 running«38;5;66» ◦ 1 queued ◦ «38;5;31»0:02.0«»"},
		{display.Ember.In(display.Colours16), "«33»▝«» «1»read the power state«2» ╏ «33»▮▮«2»▯▯▯▯▯▯«» 1/3" +
			"«2» ╏ «33»1 running«2» ╏ 1 queued ❚ " +
			"«1»run«2» ╏ «31»▮▮«2»▯▯▯▯▯▯«» 1/3«2» ╏ «31»1 failed«2» ╏ «33»1 running«2» ╏ 1 queued ╏ «33»0:02.0«»"},
	} {
		t.Run(tc.theme.String(), func(t *testing.T) {
			t.Parallel()
			f := counterThemeNew(t, "provision status", 160, display.CounterOptions{Theme: tc.theme})
			bmcs := counterThemeStep(f.ctx, "read the power state", 3)
			nodes := counterThemeStep(f.ctx, "run", 3)
			bmcs[0].Run()
			bmcs[0].End(nil)
			bmcs[1].Run()
			nodes[0].Run()
			nodes[1].Run()
			nodes[0].End(errors.New("exe1: connection refused"))
			check(t, []string{f.draw(2 * time.Second)}, tc.want)
		})
	}
}

// A power-on in batches is drawn with the batch under way, plain, and the
// pause between two, muted, the bar counting the targets of the batches
// done and of the one under way. The test runs in a testing/synctest
// bubble, so that a frame is drawn during the pause.
func TestCounterThemeCountsAPowerOnInBatches(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		f := counterThemeNew(t, "bmc power on", 120, display.CounterOptions{Theme: display.Aurora})
		nodes, err := nodeset.Parse("exe[1-4]")
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		o := duringPauses(fanout.BatchOptions{Step: "power on", Size: 2, Pause: 30 * time.Second}, func(d time.Duration) {
			got = append(got, f.draw(time.Second))
			f.clock.Add(d)
		})
		fanout.Batches(f.ctx, nodes, o, func(ctx context.Context, batch *nodeset.NodeSet) error {
			names := batch.Expand()
			spans := make([]*progress.Span, len(names))
			for i, node := range names {
				_, spans[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
			}
			spans[0].Run()
			spans[0].End(nil)
			spans[1].Run()
			got = append(got, f.draw(time.Second))
			spans[1].End(nil)
			return nil
		})
		check(t, got,
			"«38;5;98»⠋«» «1»power on«38;5;244» ∙ «»batch 1/2«38;5;244» ∙ «38;5;30»⣿⣿«38;5;244»⣀⣀⣀⣀⣀⣀«» 1/4"+
				"«38;5;244» ∙ «38;5;98»1 running«38;5;244» ∙ 2 queued ∙ «38;5;68»0:01.0«»",
			"«38;5;98»⠋«» «1»power on«38;5;244» ∙ «»batch 1/2«38;5;244» ∙ «38;5;30»⣿⣿«38;5;31»⣿«38;5;32»⣿«38;5;244»⣀⣀⣀⣀«» 2/4"+
				"«38;5;244» ∙ 2 queued ∙ waiting ∙ «38;5;68»0:02.0«»",
			"«38;5;98»⠋«» «1»power on«38;5;244» ∙ «»batch 2/2«38;5;244» ∙ «38;5;30»⣿⣿«38;5;31»⣿«38;5;32»⣿«38;5;68»⣿⣿«38;5;244»⣀⣀«» 3/4"+
				"«38;5;244» ∙ «38;5;98»1 running«38;5;244» ∙ «38;5;68»0:33.0«»",
		)
	})
}

// Every count of targets is in the colour of how they stand, the targets
// queued and the wait open below muted, and the bar's failed end in the
// colour of a failure.
func TestCounterThemePaintsEveryCount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		theme display.Theme
		want  string
	}{
		{display.Tide, "«38;5;29»◡«» «1»check the nodes«38;5;66» ◦ «38;5;32»◉◉◉◉«38;5;166»◉«38;5;66»◌◌◌«» 4/6" +
			"«38;5;66» ◦ «38;5;166»1 failed«38;5;66» ◦ «38;5;103»1 canceled«38;5;66» ◦ 1 skipped ◦ " +
			"«38;5;29»1 running«38;5;66» ◦ 1 queued ◦ waiting ◦ «38;5;31»0:02.0«»"},
		{display.Ember.In(display.Colours16), "«33»▝«» «1»check the nodes«2» ╏ «33»▮▮▮▮«31»▮«2»▯▯▯«» 4/6" +
			"«2» ╏ «31»1 failed«2» ╏ «35»1 canceled«2» ╏ 1 skipped ╏ «33»1 running«2» ╏ 1 queued ╏ waiting ╏ «33»0:02.0«»"},
	} {
		t.Run(tc.theme.String(), func(t *testing.T) {
			t.Parallel()
			f := counterThemeNew(t, "bmc status", 160, display.CounterOptions{Theme: tc.theme})
			counterThemeEveryCount(f.ctx)
			check(t, []string{f.draw(2 * time.Second)}, tc.want)
		})
	}
}

// A line that would pass the last column but one with its bars, where the
// terminal cuts it, is drawn without any. A line too long even without them
// is cut, and the cut sets the colours back.
func TestCounterThemeDropsTheBarsThatWouldNotFit(t *testing.T) {
	t.Parallel()
	const (
		// Of 78 columns with its bar, 69 without.
		withBar = "«38;5;135»◶«» «1»reset the machines«38;5;103» ⋄ «38;5;33»▰▰«38;5;197»▰«38;5;103»▱▱▱▱▱«» 3/8" +
			"«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»2 running«38;5;103» ⋄ 3 queued ⋄ «1;38;5;134»0:02.0«»"
		without = "«38;5;135»◶«» «1»reset the machines«38;5;103» ⋄ «»3/8" +
			"«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»2 running«38;5;103» ⋄ 3 queued ⋄ «1;38;5;134»0:02.0«»"
	)
	for _, tc := range []struct {
		name  string
		width int
		want  string
	}{
		{"just wide enough", 79, withBar},
		{"a column short", 78, without},
		{"wide enough without", 70, without},
		{"too narrow without", 60, "«38;5;135»◶«» «1»reset the machines«38;5;103» ⋄ «»3/8" +
			"«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»2 running«38;5;103» ⋄ 3 queue«»"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := counterThemeNew(t, "provision reinstall", tc.width, display.CounterOptions{Theme: display.Neon})
			counterThemeResetTheMachines(f.ctx)
			check(t, []string{f.draw(2 * time.Second)}, tc.want)
			counterThemeNoBleed(t, f)
		})
	}
	// Of two steps, neither's bar is drawn when both would not fit, though
	// one would: the line is of 89 columns with both, 80 with one and 71
	// with none.
	for _, tc := range []struct {
		width int
		want  string
	}{
		{90, "◶ read the power state ⋄ ▱▱▱▱▱▱▱▱ 0/3 ⋄ 3 queued ╏ run ⋄ ▱▱▱▱▱▱▱▱ 0/3 ⋄ 3 queued ⋄ 0:02.0"},
		{85, "◶ read the power state ⋄ 0/3 ⋄ 3 queued ╏ run ⋄ 0/3 ⋄ 3 queued ⋄ 0:02.0"},
	} {
		f := counterThemeNew(t, "provision status", tc.width, display.CounterOptions{Theme: display.Neon.In(display.NoColours)})
		counterThemeStep(f.ctx, "read the power state", 3)
		counterThemeStep(f.ctx, "run", 3)
		f.draw(2 * time.Second)
		check(t, []string{f.text()}, tc.want)
	}
}

// With ASCII a theme keeps its colours and draws the ASCII marks, a running
// mark that stands still, and a bar of # and . in brackets, or none in a
// theme that has none, such as Classic.
func TestCounterThemeInASCII(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		theme        display.Theme
		text, styled string
	}{
		{
			display.Neon,
			"> reset the machines - [###.....] 3/8 - 1 failed - 2 running - 3 queued | run - [........] 0/3 - 3 queued - 0:02.0",
			"«38;5;135»>«» «1»reset the machines«38;5;103» - [«38;5;33»##«38;5;197»#«38;5;103».....]«» 3/8" +
				"«38;5;103» - «38;5;197»1 failed«38;5;103» - «38;5;135»2 running«38;5;103» - 3 queued | " +
				"«1»run«38;5;103» - [........]«» 0/3«38;5;103» - 3 queued - «1;38;5;134»0:02.0«»",
		},
		{
			display.Classic.In(display.Colours16),
			"> reset the machines - 3/8 - 1 failed - 2 running - 3 queued | run - 0/3 - 3 queued - 0:02.0",
			"«36»>«» «1»reset the machines«2» - «»3/8«2» - «31»1 failed«2» - «36»2 running«2» - 3 queued | " +
				"«1»run«2» - «»0/3«2» - 3 queued - «36»0:02.0«»",
		},
	} {
		t.Run(tc.theme.String(), func(t *testing.T) {
			t.Parallel()
			f := counterThemeNew(t, "provision status", 160, display.CounterOptions{Theme: tc.theme, ASCII: true})
			counterThemeResetTheMachines(f.ctx)
			counterThemeStep(f.ctx, "run", 3)
			styled := f.draw(2 * time.Second)
			check(t, []string{f.text()}, tc.text)
			check(t, []string{styled}, tc.styled)
		})
	}
}

// The spinner turns with the time on the clock, one frame for every so
// long of it, from the first frame on again after the last: a line drawn at
// the same instant shows the same frame. Classic's mark stands still.
func TestCounterThemeTurnsTheSpinnerWithTheClock(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		theme display.Theme
		every time.Duration
		want  string
	}{
		{display.Neon, 200 * time.Millisecond, "◷◶◵◴◷◶◵"},
		{display.Ember, 200 * time.Millisecond, "▘▝▗▖▘▝▗"},
		{display.Tide, 200 * time.Millisecond, "◟◜◠◝◞◡◟"},
		{display.Aurora, 100 * time.Millisecond, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋"},
		{display.Classic, 200 * time.Millisecond, "▸▸▸▸▸"},
	} {
		t.Run(tc.theme.String(), func(t *testing.T) {
			t.Parallel()
			f := counterThemeNew(t, "", 80, display.CounterOptions{Theme: tc.theme.In(display.NoColours)})
			var got strings.Builder
			f.draw(time.Second)
			for range []rune(tc.want) {
				line := f.text()
				spin, _, _ := strings.Cut(line, " ")
				got.WriteString(spin)
				if again := f.draw(0); again != line {
					t.Errorf("drawn again at the same instant, %q became %q", line, again)
				}
				f.draw(tc.every)
			}
			if got.String() != tc.want {
				t.Errorf("the spinner turned %q, want %q", got.String(), tc.want)
			}
		})
	}
}

// With no counted step under way the line names the step under way, or
// the command, bold, and with nothing to name it is the spinner and the
// time alone.
func TestCounterThemeNamesTheStepUnderWay(t *testing.T) {
	t.Parallel()
	f := counterThemeNew(t, "provision reinstall", 80, display.CounterOptions{Theme: display.Neon})
	got := []string{f.draw(time.Second)}
	_, step := progress.Start(f.ctx, progress.KindStep, "configuring the network boot")
	got = append(got, f.draw(time.Second))
	step.End(nil)
	check(t, got,
		"«38;5;135»◷«» «1»provision reinstall«38;5;103» ⋄ «1;38;5;134»0:01.0«»",
		"«38;5;135»◶«» «1»configuring the network boot«38;5;103» ⋄ «1;38;5;134»0:02.0«»",
	)
	bare := counterThemeNew(t, "", 80, display.CounterOptions{Theme: display.Neon})
	check(t, []string{bare.draw(time.Second)}, "«38;5;135»◷«» «1;38;5;134»0:01.0«»")
}

// No colour a line is drawn in runs on into what the command writes after
// it, in any theme, in any number of colours, in ASCII or not, whether the
// line fits or is cut.
func TestCounterThemeLeavesNoColourOn(t *testing.T) {
	t.Parallel()
	for _, theme := range display.Themes() {
		for _, colours := range []display.Colours{display.Colours256, display.Colours16, display.NoColours} {
			for _, ascii := range []bool{false, true} {
				for _, width := range []int{200, 40, 12} {
					t.Run(fmt.Sprintf("%s/%s/ascii=%t/%d", theme, colours, ascii, width), func(t *testing.T) {
						t.Parallel()
						f := counterThemeNew(t, "bmc status", width, display.CounterOptions{Theme: theme.In(colours), ASCII: ascii})
						counterThemeEveryCount(f.ctx)
						counterThemeResetTheMachines(f.ctx)
						f.draw(2 * time.Second)
						counterThemeNoBleed(t, f)
					})
				}
			}
		}
	}
}
