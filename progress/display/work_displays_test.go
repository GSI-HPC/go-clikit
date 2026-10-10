// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
)

// fetchBaseImage starts the step "fetching the base image" under ctx, with
// a call that downloads 2 GiB and advances by 8 MiB every 100 ms for n
// tenths of a second on c, and returns the step and the call.
func fetchBaseImage(ctx context.Context, c *clock, n int) (step, call *progress.Span) {
	ctx, step = progress.Start(ctx, progress.KindStep, "fetching the base image")
	_, call = progress.Start(ctx, progress.KindCall, "download",
		progress.HTTP("GET", "/images/rocky-9.4.qcow2"), progress.Work(progress.Bytes, 2<<30))
	for range n {
		c.Add(100 * time.Millisecond)
		call.Advance(8 << 20)
	}
	return step, call
}

// indexTheArchive starts the step "indexing the archive" under ctx, whose
// work is of no known size, and advances it by 500 items every 100 ms for
// n tenths of a second on c.
func indexTheArchive(ctx context.Context, c *clock, n int) *progress.Span {
	_, step := progress.Start(ctx, progress.KindStep, "indexing the archive", progress.Work(progress.Items, 0))
	for range n {
		c.Add(100 * time.Millisecond)
		step.Advance(500)
	}
	return step
}

// updateTheFirmware starts the step "updating the firmware" under ctx,
// whose work is in Percent and reaches 40% in n tenths of a second on c.
func updateTheFirmware(ctx context.Context, c *clock, n int) *progress.Span {
	_, step := progress.Start(ctx, progress.KindStep, "updating the firmware", progress.Work(progress.Percent, 100))
	for i := range n {
		c.Add(100 * time.Millisecond)
		step.SetAmount(int64(40 * (i + 1) / n))
	}
	return step
}

// The counter draws, with no counted step under way, the work of the newest
// named step in its place, after its name, and nothing of the work of the
// command, which rolls up everything below it. Once the step has ended it
// names the command alone.
func TestTheCounterDrawsTheWorkOfTheStepItNames(t *testing.T) {
	t.Parallel()
	f := counterThemeNew(t, "fetch", 140, display.CounterOptions{})
	step, call := fetchBaseImage(f.ctx, f.clock, 144)
	got := []string{f.draw(0)}
	for range 112 {
		f.clock.Add(100 * time.Millisecond)
		call.Advance(8 << 20)
	}
	call.End(nil)
	got = append(got, f.draw(0))
	step.End(nil)
	got = append(got, f.draw(0))
	check(t, got,
		"fetching the base image · 1.1/2.0 GiB · 56% · 80.0 MiB/s · ~12s left · 0:14.4",
		"fetching the base image · 2.0/2.0 GiB · 100% · 80.0 MiB/s · 0:25.6",
		"fetch · 0:25.6")
}

// Work of no known size is drawn as its amount and its rate, work in
// Percent as its share, and the work of a call made for the command itself,
// which is not a step, is not drawn.
func TestTheCounterDrawsWorkOfNoKnownSizeAndInPercent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		start func(ctx context.Context, c *clock)
		want  string
	}{
		{"items", func(ctx context.Context, c *clock) { indexTheArchive(ctx, c, 30) },
			"indexing the archive · 15.0k · 5.0k/s · 0:03.0"},
		{"percent", func(ctx context.Context, c *clock) { updateTheFirmware(ctx, c, 30) },
			"updating the firmware · 40% · ~5s left · 0:03.0"},
		{"a call of the command", func(ctx context.Context, c *clock) {
			_, call := progress.Start(ctx, progress.KindCall, "download", progress.Work(progress.Bytes, 1<<30))
			c.Add(3 * time.Second)
			call.Advance(1 << 29)
		}, "fetch · 0:03.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := counterThemeNew(t, "fetch", 140, display.CounterOptions{})
			tc.start(f.ctx, f.clock)
			check(t, []string{f.draw(0)}, tc.want)
		})
	}
}

// A counted step is followed by the work it rolls up from its targets: the
// amount, the share, the rate and the time left, which a line too wide for
// the terminal gives up first. Targets whose work is of no known size give
// the step an amount and a rate and no share, and the line is as it always
// was where they report none.
func TestTheCounterDrawsTheWorkOfAPool(t *testing.T) {
	t.Parallel()
	f := counterThemeNew(t, "deploy", 140, display.CounterOptions{})
	copyImageOn(f.ctx, f.clock)
	check(t, []string{f.draw(0)},
		"copying the image · 7/12 · 1 failed · 4 running · 1 queued · 18.8 GiB · 82% · 640 MiB/s · ~9s left · 0:35.0")

	f = counterThemeNew(t, "deploy", 100, display.CounterOptions{})
	copyImageOn(f.ctx, f.clock)
	check(t, []string{f.draw(0)},
		"copying the image · 7/12 · 1 failed · 4 running · 1 queued · 18.8 GiB · 82% · 640 MiB/s · 0:35.0")

	for _, tc := range []struct {
		width int
		want  string
	}{
		{140, "indexing the archives · 0/3 · 2 running · 1 queued · 30.0k · 6.0k/s · 0:05.0"},
		{61, "indexing the archives · 0/3 · 2 running · 1 queued · 0:05.0"},
	} {
		f = counterThemeNew(t, "index", tc.width, display.CounterOptions{})
		ctx, _ := progress.Start(f.ctx, progress.KindStep, "indexing the archives", progress.WithFlags(progress.Fold), progress.Total(3))
		ctxs, spans := targets(ctx, nodes(3)...)
		var walks []*progress.Span
		for i := range 2 {
			spans[i].Run()
			_, walk := progress.Start(ctxs[i], progress.KindCall, "walk", progress.Work(progress.Items, 0))
			walks = append(walks, walk)
		}
		f.clock.Add(5 * time.Second)
		walks[0].Advance(25000)
		walks[1].Advance(5000)
		check(t, []string{f.draw(0)}, tc.want)
	}
}

// In a theme the bar of a counted step fills with its share of the work
// done, the cells of the targets that failed at its end, and the amount and
// the rate are muted and the share is in the colour of the bar. A line too
// wide for the terminal gives up the time left, the rate, the amount and
// the bar in turn until it fits.
func TestTheCounterInAThemeDrawsTheWorkOfAPool(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		o     display.CounterOptions
		width int
		want  string
	}{
		{"Tide", display.CounterOptions{Theme: display.Tide}, 140,
			"«38;5;29»◠«» «1»copying the image«38;5;66» ◦ «38;5;32»◉◉◉◉◉«38;5;166»◉«38;5;66»◌◌«» 7/12«38;5;66» ◦ «38;5;166»1 failed«38;5;66» ◦ " +
				"«38;5;29»4 running«38;5;66» ◦ 1 queued ◦ 18.8 GiB ◦ «38;5;32»82%«38;5;66» ◦ 640 MiB/s ◦ ~9s left ◦ «38;5;31»0:35.0«»"},
		{"Tide in ASCII", display.CounterOptions{Theme: display.Tide, ASCII: true}, 140,
			"«38;5;29»>«» «1»copying the image«38;5;66» - [«38;5;32»#####«38;5;166»#«38;5;66»..]«» 7/12«38;5;66» - «38;5;166»1 failed«38;5;66» - " +
				"«38;5;29»4 running«38;5;66» - 1 queued - 18.8 GiB - «38;5;32»82%«38;5;66» - 640 MiB/s - ~9s left - «38;5;31»0:35.0«»"},
		{"Neon", display.CounterOptions{Theme: display.Neon}, 140,
			"«38;5;135»◵«» «1»copying the image«38;5;103» ⋄ «38;5;33»▰▰«38;5;69»▰«38;5;63»▰«38;5;99»▰«38;5;197»▰«38;5;103»▱▱«» 7/12«38;5;103» ⋄ " +
				"«38;5;197»1 failed«38;5;103» ⋄ «38;5;135»4 running«38;5;103» ⋄ 1 queued ⋄ 18.8 GiB ⋄ «38;5;33»82%«38;5;103» ⋄ 640 MiB/s ⋄ ~9s left ⋄ «1;38;5;134»0:35.0«»"},
		{"Tide gives up the time left first", display.CounterOptions{Theme: display.Tide}, 110,
			"«38;5;29»◠«» «1»copying the image«38;5;66» ◦ «38;5;32»◉◉◉◉◉«38;5;166»◉«38;5;66»◌◌«» 7/12«38;5;66» ◦ «38;5;166»1 failed«38;5;66» ◦ " +
				"«38;5;29»4 running«38;5;66» ◦ 1 queued ◦ 18.8 GiB ◦ «38;5;32»82%«38;5;66» ◦ 640 MiB/s ◦ «38;5;31»0:35.0«»"},
		{"Tide gives up the rate next", display.CounterOptions{Theme: display.Tide}, 100,
			"«38;5;29»◠«» «1»copying the image«38;5;66» ◦ «38;5;32»◉◉◉◉◉«38;5;166»◉«38;5;66»◌◌«» 7/12«38;5;66» ◦ «38;5;166»1 failed«38;5;66» ◦ " +
				"«38;5;29»4 running«38;5;66» ◦ 1 queued ◦ 18.8 GiB ◦ «38;5;32»82%«38;5;66» ◦ «38;5;31»0:35.0«»"},
		{"Tide gives up the amount next", display.CounterOptions{Theme: display.Tide}, 90,
			"«38;5;29»◠«» «1»copying the image«38;5;66» ◦ «38;5;32»◉◉◉◉◉«38;5;166»◉«38;5;66»◌◌«» 7/12«38;5;66» ◦ «38;5;166»1 failed«38;5;66» ◦ " +
				"«38;5;29»4 running«38;5;66» ◦ 1 queued ◦ «38;5;32»82%«38;5;66» ◦ «38;5;31»0:35.0«»"},
		{"Tide gives up the bar last", display.CounterOptions{Theme: display.Tide}, 80,
			"«38;5;29»◠«» «1»copying the image«38;5;66» ◦ «»7/12«38;5;66» ◦ «38;5;166»1 failed«38;5;66» ◦ «38;5;29»4 running«38;5;66» ◦ " +
				"1 queued ◦ «38;5;32»82%«38;5;66» ◦ «38;5;31»0:35.0«»"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := counterThemeNew(t, "deploy", tc.width, tc.o)
			copyImageOn(f.ctx, f.clock)
			check(t, []string{f.draw(0)}, tc.want)
			counterThemeNoBleed(t, f)
		})
	}
}

// A tree on a terminal too small for it draws the line of the counter in
// its place, with the work.
func TestTheTreeOnASmallTerminalDrawsTheWorkInTheCountersLine(t *testing.T) {
	t.Parallel()
	f := newTreeFixture(t, "deploy", treeSetup{width: 120, height: 5})
	copyImageOn(f.ctx, f.clock)
	got := f.draw(0)
	want := "copying the image · 7/12 · 1 failed · 4 running · 1 queued · 18.8 GiB · 82% · 640 MiB/s · ~9s left · 0:35.0\n"
	if got != want {
		t.Errorf("the line is\n%s\nwant\n%s", got, want)
	}
}

// A step with work says how far it has got every ten seconds, as a counted
// step does, with its work after its counts, the parts split by commas; the
// line it leaves says what the work came to and how fast, after how the
// targets ended when it counts them. Work in Percent adds no amount to it.
func TestPlainLinesSayHowFarTheWorkHasGot(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		run  func(f *plainFixture)
		want string
	}{
		{"bytes of a known size", func(f *plainFixture) {
			step, call := fetchBaseImage(f.ctx, f.clock, 100)
			f.draw(0)
			for range 150 {
				f.clock.Add(100 * time.Millisecond)
				call.Advance(8 << 20)
			}
			call.End(nil)
			step.End(nil)
		}, `
[0:00] fetch › fetching the base image: start
[0:10] fetch › fetching the base image: 0.8/2.0 GiB, 39%, 80.0 MiB/s, ~16s left
[0:25] fetch › fetching the base image: done in 25s: 2.0 GiB at 80.0 MiB/s
fetch: done in 25s: 2.0 GiB at 80.0 MiB/s
`},
		{"a pool", func(f *plainFixture) {
			p := copyImageOn(f.ctx, f.clock)
			f.draw(0)
			p.finish(f.clock)
		}, `
[0:00] fetch › copying the image: start, 12 targets, 4 at a time
[0:13] fetch › copying the image › exe4 failed (transport): transport: dial tcp: i/o timeout
[0:35] fetch › copying the image: 7/12 done, 1 failed, 4 running, 1 queued, 18.8 GiB, 82%, 640 MiB/s, ~9s left
[0:36] fetch › copying the image: done in 36s: 11 ok, 1 failed, 22.8 GiB at 647 MiB/s
fetch: done in 36s: 11 ok, 1 failed, 22.8 GiB at 647 MiB/s
`},
		{"items of no known size", func(f *plainFixture) {
			step := indexTheArchive(f.ctx, f.clock, 100)
			f.draw(0)
			step.End(nil)
		}, `
[0:00] fetch › indexing the archive: start
[0:10] fetch › indexing the archive: 50.0k, 5.0k/s
[0:10] fetch › indexing the archive: done in 10s: 50.0k at 5.0k/s
fetch: done in 10s: 50.0k at 5.0k/s
`},
		{"percent", func(f *plainFixture) {
			step := updateTheFirmware(f.ctx, f.clock, 100)
			f.draw(0)
			step.End(nil)
		}, `
[0:00] fetch › updating the firmware: start
[0:10] fetch › updating the firmware: 40%, ~15s left
[0:10] fetch › updating the firmware: done in 10s
fetch: done in 10s
`},
		{"a step with no work says nothing", func(f *plainFixture) {
			_, step := progress.Start(f.ctx, progress.KindStep, "check")
			f.draw(10 * time.Second)
			step.End(nil)
		}, `
[0:00] fetch › check: start
[0:10] fetch › check: done in 10s
fetch: done in 10s
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newPlainFixture(t, "fetch")
			tc.run(f)
			checkScreen(t, f.end(nil), tc.want)
		})
	}
}

// plainThemeWorkLines runs commands that report work and returns what the
// screen got, the summary last, on a Plain with the options o.
func plainThemeWorkLines(t *testing.T, o display.PlainOptions) string {
	t.Helper()
	f := newPlainFixtureWith(t, "deploy", o)
	step, call := fetchBaseImage(f.ctx, f.clock, 100)
	f.draw(0)
	for range 150 {
		f.clock.Add(100 * time.Millisecond)
		call.Advance(8 << 20)
	}
	call.End(nil)
	step.End(nil)
	p := copyImageOn(f.ctx, f.clock)
	f.draw(0)
	p.finish(f.clock)
	return f.end(nil)
}

// The work of a step takes the colours of a theme, and only adds to the
// lines of none: they are the lines of no theme once the colours and the
// marks are taken off, and no colour is left on.
func TestPlainLinesInAThemeSayTheWork(t *testing.T) {
	t.Parallel()
	for _, o := range plainThemeOptions() {
		t.Run(plainThemeName(o), func(t *testing.T) {
			t.Parallel()
			none := plainThemeWorkLines(t, display.PlainOptions{ASCII: o.ASCII})
			if !strings.Contains(none, "0.8/2.0 GiB, 39%, 80.0 MiB/s, ~16s left") || !strings.Contains(none, "22.8 GiB at 647 MiB/s") {
				t.Fatalf("no theme says no work:\n%s", none)
			}
			themed := plainThemeWorkLines(t, o)
			plainThemeLeavesNoColourOn(t, themed)
			text, _ := plainThemeShown(themed)
			want := strings.Split(strings.TrimSuffix(none, "\n"), "\n")
			got := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
			if len(got) != len(want) {
				t.Fatalf("%d lines in the theme, %d with none:\n%s", len(got), len(want), text)
			}
			for i, row := range got {
				prefix := ""
				if i < len(got)-1 {
					prefix, _, _ = strings.Cut(want[i], " ")
					prefix += " "
				}
				if unmarked, marked := plainThemeUnmarked(row, prefix); !marked || unmarked != want[i] {
					t.Errorf("line %d is %q, marked %v; want %q with a mark", i, row, marked, want[i])
				}
			}
		})
	}
}

// In a theme the amount and the rate of a step are muted, the share takes
// the colour of the bar and what the work came to is muted.
func TestPlainLinesInAThemeColourTheWork(t *testing.T) {
	t.Parallel()
	_, styled := plainThemeShown(plainThemeWorkLines(t, display.PlainOptions{Theme: display.Tide.In(display.Colours16)}))
	for _, want := range []string{
		"«2»0.8/2.0 GiB«», «36»39%«», «2»80.0 MiB/s«», «2»~16s left«»",
		"«2»2.0 GiB«» at «2»80.0 MiB/s«»",
	} {
		if !strings.Contains(styled, want) {
			t.Errorf("no %q in\n%s", want, styled)
		}
	}
}

// plainThemeSummariesWithWork are commands that move work, and end in each
// of the ways the summary counts: each runs its work under the command and
// ends it.
var plainThemeSummariesWithWork = []struct {
	name string
	work func(ctx context.Context, c *clock, command *progress.Span)
	want string
}{
	{"bytes and counts", func(ctx context.Context, c *clock, command *progress.Span) {
		p := copyImageOn(ctx, c)
		p.finish(c)
		command.End(nil)
	}, "exec: done in 36s: 11 ok, 1 failed, 22.8 GiB at 647 MiB/s"},
	{"bytes and no counts", func(ctx context.Context, c *clock, command *progress.Span) {
		step, call := fetchBaseImage(ctx, c, 256)
		call.End(nil)
		step.End(nil)
		command.End(nil)
	}, "exec: done in 25s: 2.0 GiB at 80.0 MiB/s"},
	{"bytes in a command that failed", func(ctx context.Context, c *clock, command *progress.Span) {
		step, call := fetchBaseImage(ctx, c, 100)
		call.End(context.Canceled)
		step.End(context.Canceled)
		command.End(context.Canceled)
	}, "exec: canceled in 10s: 800 MiB at 80.0 MiB/s"},
	{"items", func(ctx context.Context, c *clock, command *progress.Span) {
		indexTheArchive(ctx, c, 100).End(nil)
		command.End(nil)
	}, "exec: done in 10s: 50.0k at 5.0k/s"},
	{"work in two units", func(ctx context.Context, c *clock, command *progress.Span) {
		step, call := fetchBaseImage(ctx, c, 100)
		call.End(nil)
		step.End(nil)
		indexTheArchive(ctx, c, 100).End(nil)
		command.End(nil)
	}, "exec: done in 20s"},
	{"work in percent", func(ctx context.Context, c *clock, command *progress.Span) {
		updateTheFirmware(ctx, c, 100).End(nil)
		command.End(nil)
	}, "exec: done in 10s"},
	{"work in percent and bytes, in two units", func(ctx context.Context, c *clock, command *progress.Span) {
		updateTheFirmware(ctx, c, 100).End(nil)
		step, call := fetchBaseImage(ctx, c, 100)
		call.End(nil)
		step.End(nil)
		command.End(nil)
	}, "exec: done in 20s"},
	{"work in bytes and percent, in two units", func(ctx context.Context, c *clock, command *progress.Span) {
		step, call := fetchBaseImage(ctx, c, 100)
		call.End(nil)
		step.End(nil)
		updateTheFirmware(ctx, c, 100).End(nil)
		command.End(nil)
	}, "exec: done in 20s"},
}

// The summary adds the amount the command moved and its mean rate when all
// its work is in one unit: amounts in different units are never added, and
// work in Percent has no amount to add.
func TestTheSummaryAddsTheWorkOfTheCommand(t *testing.T) {
	t.Parallel()
	for _, tc := range plainThemeSummariesWithWork {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := plainThemeSummary(t, display.Theme{}, false, tc.work); got != tc.want {
				t.Errorf("the summary is %q, want %q", got, tc.want)
			}
		})
	}
}

// In a theme the work of the summary is muted, and the line is the line of
// none once its colours and its mark are taken off, in every theme, in any
// number of colours and in ASCII.
func TestTheSummaryInAThemeAddsTheWorkOfTheCommand(t *testing.T) {
	t.Parallel()
	tc := plainThemeSummariesWithWork[0]
	line := plainThemeSummary(t, display.Tide.In(display.Colours16), false, tc.work)
	_, styled := plainThemeShown(line + "\n")
	if want := "«36»11 ok«», «31»1 failed«», «2»22.8 GiB«» at «2»647 MiB/s«»"; !strings.Contains(styled, want) {
		t.Errorf("no %q in %s", want, styled)
	}
	for _, o := range plainThemeOptions() {
		t.Run(plainThemeName(o), func(t *testing.T) {
			t.Parallel()
			for _, tc := range plainThemeSummariesWithWork {
				line := plainThemeSummary(t, o.Theme, o.ASCII, tc.work)
				text, _ := plainThemeShown(line + "\n")
				if unmarked, marked := plainThemeUnmarked(strings.TrimSuffix(text, "\n"), ""); !marked || unmarked != tc.want {
					t.Errorf("%s: the summary is %q, marked %v; want %q with a mark", tc.name, text, marked, tc.want)
				}
				plainThemeLeavesNoColourOn(t, line+"\n")
			}
		})
	}
}
