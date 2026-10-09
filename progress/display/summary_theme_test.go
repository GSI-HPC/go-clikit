// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// plainThemeSummaries are commands that end in each way, with targets
// counted and without, and one done within a second: each runs its work
// under the command and ends it.
var plainThemeSummaries = []struct {
	name string
	work func(ctx context.Context, c *clock, command *progress.Span)
}{
	{"done with counts", func(ctx context.Context, c *clock, command *progress.Span) {
		step(ctx, "run", three, nil)
		c.Add(time.Second)
		command.End(nil)
	}},
	{"done counting nothing", func(_ context.Context, c *clock, command *progress.Span) {
		c.Add(12 * time.Second)
		command.End(nil)
	}},
	{"failed with counts", func(ctx context.Context, c *clock, command *progress.Span) {
		step(ctx, "run", three, map[string]error{"exe2": errors.New("connection refused")})
		c.Add(1300 * time.Millisecond)
		command.End(errors.New("1 of 3 hosts failed: exe2"))
	}},
	{"failed counting nothing", func(_ context.Context, c *clock, command *progress.Span) {
		c.Add(2 * time.Minute)
		command.End(errors.New("the boot link of exe2 could not be changed"))
	}},
	{"canceled with counts", func(ctx context.Context, c *clock, command *progress.Span) {
		step(ctx, "run", three, map[string]error{"exe1": progress.Skip("in maintenance"), "exe2": errors.New("connection refused"),
			"exe3": context.Canceled})
		c.Add(18*time.Minute + 3*time.Second)
		command.End(context.Canceled)
	}},
	{"canceled counting nothing", func(_ context.Context, c *clock, command *progress.Span) {
		c.Add(3 * time.Second)
		command.End(context.Canceled)
	}},
	{"skipped with counts", func(ctx context.Context, c *clock, command *progress.Span) {
		step(ctx, "run", three, nil)
		c.Add(time.Hour + 2*time.Minute + 3*time.Second)
		command.Skip("nothing left to do")
	}},
	{"skipped counting nothing", func(_ context.Context, c *clock, command *progress.Span) {
		c.Add(time.Second)
		command.Skip("nothing to do")
	}},
	{"within a second", func(ctx context.Context, c *clock, command *progress.Span) {
		step(ctx, "run", three, nil)
		c.Add(999 * time.Millisecond)
		command.End(nil)
	}},
}

// plainThemeSummary returns the line a Summary in theme, in ASCII when
// ascii is set, leaves for the command work runs.
func plainThemeSummary(t *testing.T, theme display.Theme, ascii bool,
	work func(ctx context.Context, c *clock, command *progress.Span),
) string {
	t.Helper()
	c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
	summary := &display.Summary{Theme: theme, ASCII: ascii}
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture, summary}, Now: c.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "exec")
	work(ctx, c, command)
	bus.Close()
	progresstest.Check(t, capture.Events())
	return summary.Line()
}

// In a theme, the summary starts with the mark of how the command ended,
// in its colour, and takes the theme's colours: the command's name as a
// title, the word that says how it ended in the colour of that, the time
// muted, and each count in the colour of how its targets ended. In ASCII
// it draws the ASCII marks; with no theme, ASCII changes nothing.
func TestTheSummaryInATheme(t *testing.T) {
	t.Parallel()
	ember, tide, neon := display.Ember, display.Tide.In(display.Colours16), display.Neon.In(display.NoColours)
	for _, tc := range []struct {
		theme display.Theme
		ascii bool
		// want is what Line returns in each of plainThemeSummaries, as a
		// terminal shows it with the attributes of its text.
		want []string
	}{
		{ember, false, []string{
			"«1»✓«» «1»exec«»: «1»done«» in «38;5;101»1.0s«»: «1»3 ok«»",
			"«1»✓«» «1»exec«»: «1»done«» in «38;5;101»12s«»",
			"«1;38;5;162»✘«» «1»exec«»: «38;5;162»failed«» in «38;5;101»1.3s«»: «1»2 ok«», «38;5;162»1 failed«»",
			"«1;38;5;162»✘«» «1»exec«»: «38;5;162»failed«» in «38;5;101»2m00s«»",
			"«1;38;5;130»⊟«» «1»exec«»: «38;5;130»canceled«» in «38;5;101»18m03s«»: 0 ok, «38;5;162»1 failed«», «38;5;130»1 canceled«», «38;5;101»1 skipped«»",
			"«1;38;5;130»⊟«» «1»exec«»: «38;5;130»canceled«» in «38;5;101»3.0s«»",
			"«1;38;5;101»▢«» «1»exec«»: «38;5;101»skipped«» in «38;5;101»1h02m03s«»: «1»3 ok«»",
			"«1;38;5;101»▢«» «1»exec«»: «38;5;101»skipped«» in «38;5;101»1.0s«»",
			"",
		}},
		{tide, false, []string{
			"«36»✓«» «1»exec«»: «36»done«» in «2»1.0s«»: «36»3 ok«»",
			"«36»✓«» «1»exec«»: «36»done«» in «2»12s«»",
			"«31»✕«» «1»exec«»: «31»failed«» in «2»1.3s«»: «36»2 ok«», «31»1 failed«»",
			"«31»✕«» «1»exec«»: «31»failed«» in «2»2m00s«»",
			"«35»⊖«» «1»exec«»: «35»canceled«» in «2»18m03s«»: 0 ok, «31»1 failed«», «35»1 canceled«», «2»1 skipped«»",
			"«35»⊖«» «1»exec«»: «35»canceled«» in «2»3.0s«»",
			"«2»↷«» «1»exec«»: «2»skipped«» in «2»1h02m03s«»: «36»3 ok«»",
			"«2»↷«» «1»exec«»: «2»skipped«» in «2»1.0s«»",
			"",
		}},
		{neon, false, []string{
			"✓ exec: done in 1.0s: 3 ok",
			"✓ exec: done in 12s",
			"✘ exec: failed in 1.3s: 2 ok, 1 failed",
			"✘ exec: failed in 2m00s",
			"⊘ exec: canceled in 18m03s: 0 ok, 1 failed, 1 canceled, 1 skipped",
			"⊘ exec: canceled in 3.0s",
			"⇥ exec: skipped in 1h02m03s: 3 ok",
			"⇥ exec: skipped in 1.0s",
			"",
		}},
		{tide, true, []string{
			"«36»+«» «1»exec«»: «36»done«» in «2»1.0s«»: «36»3 ok«»",
			"«36»+«» «1»exec«»: «36»done«» in «2»12s«»",
			"«31»x«» «1»exec«»: «31»failed«» in «2»1.3s«»: «36»2 ok«», «31»1 failed«»",
			"«31»x«» «1»exec«»: «31»failed«» in «2»2m00s«»",
			"«35»~«» «1»exec«»: «35»canceled«» in «2»18m03s«»: 0 ok, «31»1 failed«», «35»1 canceled«», «2»1 skipped«»",
			"«35»~«» «1»exec«»: «35»canceled«» in «2»3.0s«»",
			"«2»-«» «1»exec«»: «2»skipped«» in «2»1h02m03s«»: «36»3 ok«»",
			"«2»-«» «1»exec«»: «2»skipped«» in «2»1.0s«»",
			"",
		}},
		{display.Theme{}, true, []string{
			"exec: done in 1.0s: 3 ok",
			"exec: done in 12s",
			"exec: failed in 1.3s: 2 ok, 1 failed",
			"exec: failed in 2m00s",
			"exec: canceled in 18m03s: 0 ok, 1 failed, 1 canceled, 1 skipped",
			"exec: canceled in 3.0s",
			"exec: skipped in 1h02m03s: 3 ok",
			"exec: skipped in 1.0s",
			"",
		}},
	} {
		name := plainThemeName(display.PlainOptions{Theme: tc.theme, ASCII: tc.ascii})
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if len(tc.want) != len(plainThemeSummaries) {
				t.Fatalf("%d summaries, want %d", len(tc.want), len(plainThemeSummaries))
			}
			for i, s := range plainThemeSummaries {
				line := plainThemeSummary(t, tc.theme, tc.ascii, s.work)
				if line == "" {
					if tc.want[i] != "" {
						t.Errorf("%s: no summary, want %q", s.name, tc.want[i])
					}
					continue
				}
				_, styled := plainThemeShown(line + "\n")
				if got := strings.TrimSuffix(styled, "\n"); got != tc.want[i] {
					t.Errorf("%s: the summary in its colours is %q, want %q", s.name, got, tc.want[i])
				}
			}
		})
	}
}

// A theme only adds to the summary: in every theme, in any number of
// colours and in ASCII, it is the summary of no theme once its colours are
// taken off and the mark in front of it with the space after it; it leaves
// no colour on; and where no theme leaves no summary, neither does a
// theme.
func TestTheSummaryInAThemeSaysTheWordsOfNone(t *testing.T) {
	t.Parallel()
	for _, o := range plainThemeOptions() {
		t.Run(plainThemeName(o), func(t *testing.T) {
			t.Parallel()
			for _, s := range plainThemeSummaries {
				none := plainThemeSummary(t, display.Theme{}, o.ASCII, s.work)
				line := plainThemeSummary(t, o.Theme, o.ASCII, s.work)
				if none == "" {
					if line != "" {
						t.Errorf("%s: the summary %q, where no theme leaves none", s.name, line)
					}
					continue
				}
				text, _ := plainThemeShown(line + "\n")
				if unmarked, marked := plainThemeUnmarked(strings.TrimSuffix(text, "\n"), ""); !marked || unmarked != none {
					t.Errorf("%s: the summary is %q, marked %v; want %q with a mark", s.name, text, marked, none)
				}
				plainThemeLeavesNoColourOn(t, line+"\n")
			}
		})
	}
}
