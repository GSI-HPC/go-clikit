// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
	"github.com/GSI-HPC/go-clikit/termtext"
)

// plainThemeWork runs a command that draws every kind of plain line, on a
// Plain with the options o and a summary in its theme, and returns what
// the screen got, the summary last: the start of a counted step and of a
// batch, a target that failed, a pause, how far the step has got, a wait
// that failed, the ends of a batch, a batch left out and a step that
// failed, a batch left out for an interrupt, steps that were interrupted,
// skipped, left out as they failed and that ended well, and a wait that
// was interrupted.
func plainThemeWork(t *testing.T, o display.PlainOptions) string {
	t.Helper()
	f := newPlainFixtureWith(t, "bmc power on", o)
	stepCtx, step := progress.Start(f.ctx, progress.KindStep, "power on",
		progress.WithFlags(progress.Fold), progress.Total(6), progress.Limit(2))
	firstCtx, first := progress.Start(stepCtx, progress.KindBatch, "1/2", progress.Queued(), progress.Batch(1, 2), progress.Total(5))
	_, second := progress.Start(stepCtx, progress.KindBatch, "2/2", progress.Queued(), progress.Batch(2, 2), progress.Total(1),
		progress.Node("exe6"))
	first.Run()
	targets := make([]*progress.Span, 5)
	for i := range targets {
		node := fmt.Sprintf("exe%d", i+1)
		_, targets[i] = progress.Start(firstCtx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	for i, err := range []error{unreachable("exe1.mgmt: dial tcp: i/o timeout"), progress.Skip("powered on already"), context.Canceled} {
		targets[i].Run()
		f.clock.Add(time.Second)
		targets[i].End(err)
	}
	targets[3].Run()
	_, stagger := progress.Start(firstCtx, progress.KindWait, "stagger", progress.Timeout(30*time.Second))
	f.draw(7 * time.Second)
	stagger.End(unreachable("the clock of exe4.mgmt stopped"))
	targets[3].End(nil)
	targets[4].Run()
	targets[4].End(nil)
	first.End(errors.New("1 of 5 service processors failed"))
	second.Skip("not tried: an earlier batch failed")
	step.End(errors.New("1 of 6 hosts failed: exe1"))

	offCtx, off := progress.Start(f.ctx, progress.KindStep, "power off", progress.WithFlags(progress.Fold), progress.Total(2))
	_, left := progress.Start(offCtx, progress.KindBatch, "1/1", progress.Queued(), progress.Batch(1, 1), progress.Total(2),
		progress.Node("exe[7-8]"))
	left.End(context.Canceled)
	off.End(context.Canceled)

	_, slurm := progress.Start(f.ctx, progress.KindStep, "check Slurm jobs")
	f.clock.Add(time.Second)
	slurm.End(context.Canceled)
	_, drain := progress.Start(f.ctx, progress.KindStep, "drain")
	drain.Skip("no jobs to drain")
	_, unlock := progress.Start(f.ctx, progress.KindStep, "unlock", progress.Queued())
	unlock.End(errors.New("the lock is held"))
	_, settle := progress.Start(f.ctx, progress.KindWait, "settle", progress.Timeout(5*time.Second))
	f.clock.Add(2 * time.Second)
	settle.End(context.Canceled)
	_, report := progress.Start(f.ctx, progress.KindStep, "report")
	f.clock.Add(1500 * time.Millisecond)
	report.End(nil)
	f.draw(0)
	return f.end(context.Canceled)
}

// plainThemeShown returns what a terminal shows of raw: its text alone, and
// its text with the attributes of each run, as progresstest.Screen.Styled
// writes them.
func plainThemeShown(raw string) (text, styled string) {
	s := &progresstest.Screen{Styles: true}
	_, _ = io.WriteString(s, raw)
	return s.String(), s.Styled()
}

// plainThemeOptions are the options of a Plain in every theme, in every
// number of colours, in Unicode and in ASCII.
func plainThemeOptions() []display.PlainOptions {
	var all []display.PlainOptions
	for _, theme := range display.Themes() {
		for _, c := range []display.Colours{display.Colours256, display.Colours16, display.NoColours} {
			for _, ascii := range []bool{false, true} {
				all = append(all, display.PlainOptions{Theme: theme.In(c), ASCII: ascii})
			}
		}
	}
	return all
}

// plainThemeName names the options of a Plain for a subtest.
func plainThemeName(o display.PlainOptions) string {
	name := o.Theme.String() + " in " + o.Theme.Colours().String()
	if o.ASCII {
		name += " in ASCII"
	}
	return name
}

// plainThemeSGR finds the sequences that set attributes of text.
var plainThemeSGR = regexp.MustCompile("\x1b\\[[0-9;:]*m")

// plainThemeLeavesNoColourOn checks that every line of raw sets each
// attribute it sets back before it sets another and before it ends, that
// it holds no escape but these, and that text written after it is in the
// terminal's own colour: nothing a theme draws bleeds into the next line,
// or into the command's output.
func plainThemeLeavesNoColourOn(t *testing.T, raw string) {
	t.Helper()
	for line := range strings.Lines(raw) {
		seqs := plainThemeSGR.FindAllString(line, -1)
		if strings.Count(line, "\x1b") != len(seqs) {
			t.Errorf("an escape that sets no attribute in %q", line)
		}
		for i, seq := range seqs {
			if reset := seq == "\x1b[0m"; reset != (i%2 == 1) {
				t.Errorf("sequence %d of %q is %q: a colour not set back, or set back twice", i, line, seq)
			}
		}
		if len(seqs)%2 != 0 {
			t.Errorf("%q ends with a colour on", line)
		}
	}
	text, styled := plainThemeShown(raw + "after\n")
	if strings.Contains(text, "^[") {
		t.Errorf("a sequence the screen does not apply:\n%s", text)
	}
	if !strings.HasSuffix(styled, "\nafter\n") {
		t.Errorf("what follows the lines is not in the terminal's own colour:\n%s", styled)
	}
}

// plainThemeUnmarked returns row without the mark after its prefix and
// the space after the mark, and whether there was a mark there of one
// column.
func plainThemeUnmarked(row, prefix string) (string, bool) {
	rest, ok := strings.CutPrefix(row, prefix)
	if !ok {
		return row, false
	}
	mark, after, ok := strings.Cut(rest, " ")
	return prefix + after, ok && termtext.Width(mark) == 1 && strings.TrimSpace(mark) == mark
}

// In a theme, a plain line takes its colours: the time in the clock's, a
// mark after it, of what runs for a start, a pause and how far a step has
// got and of how it ended for the rest, the separators of the path muted,
// and the words that say how a span is doing in the colours of what they
// say; names, errors, reasons and the sizes of steps stay in the terminal's
// own colour. The summary takes the theme too.
func TestPlainLinesInATheme(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		o    display.PlainOptions
		// text, when set, is what the screen shows; styled is what it shows
		// with the attributes of the text.
		text, styled string
	}{
		{name: "ember in 256 colours", o: display.PlainOptions{Theme: display.Ember}, styled: `
«1;38;5;136»[0:00]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»power on: «38;5;166»start«», 6 targets, 2 at a time
«1;38;5;136»[0:00]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»power on«38;5;101» › «»batch 1/2: «38;5;166»start«», 5 targets
«1;38;5;136»[0:01]«» «1;38;5;162»✘«» bmc power on«38;5;101» › «»power on«38;5;101» › «»batch 1/2«38;5;101» › «»exe1 «38;5;162»failed (transport)«»: exe1.mgmt: dial tcp: i/o timeout
«1;38;5;136»[0:03]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»power on«38;5;101» › «»batch 1/2«38;5;101» › «»stagger: «38;5;101»waiting 30s«»
«1;38;5;136»[0:10]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»power on: batch 1/2, 3/6 done, «38;5;162»1 failed«», «38;5;130»1 canceled«», «38;5;101»1 skipped«», «38;5;166»1 running«», «38;5;101»2 queued«», «38;5;101»waiting«»
«1;38;5;136»[0:10]«» «1;38;5;162»✘«» bmc power on«38;5;101» › «»power on«38;5;101» › «»batch 1/2«38;5;101» › «»stagger: «38;5;162»failed (transport)«»
«1;38;5;136»[0:10]«» «1;38;5;162»✘«» bmc power on«38;5;101» › «»power on«38;5;101» › «»batch 1/2: «38;5;162»failed«» in «38;5;101»10s«»: «1»2 ok«», «38;5;162»1 failed«», «38;5;130»1 canceled«», «38;5;101»1 skipped«»
«1;38;5;136»[0:10]«» «1;38;5;101»▢«» bmc power on«38;5;101» › «»power on«38;5;101» › «»batch 2/2: «38;5;101»skipped«»: not tried: an earlier batch failed
«1;38;5;136»[0:10]«» «1;38;5;162»✘«» bmc power on«38;5;101» › «»power on: «38;5;162»failed«» in «38;5;101»10s«»: «1»2 ok«», «38;5;162»1 failed«», «38;5;130»1 canceled«», «38;5;101»2 skipped«»
«1;38;5;136»[0:10]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»power off: «38;5;166»start«», 2 targets
«1;38;5;136»[0:10]«» «1;38;5;130»⊟«» bmc power on«38;5;101» › «»power off«38;5;101» › «»batch 1/1: «38;5;130»canceled«»
«1;38;5;136»[0:10]«» «1;38;5;130»⊟«» bmc power on«38;5;101» › «»power off: «38;5;130»canceled«» in «38;5;101»0.0s«»: 0 ok, «38;5;130»2 canceled«»
«1;38;5;136»[0:10]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»check Slurm jobs: «38;5;166»start«»
«1;38;5;136»[0:11]«» «1;38;5;130»⊟«» bmc power on«38;5;101» › «»check Slurm jobs: «38;5;130»canceled«» in «38;5;101»1.0s«»
«1;38;5;136»[0:11]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»drain: «38;5;166»start«»
«1;38;5;136»[0:11]«» «1;38;5;101»▢«» bmc power on«38;5;101» › «»drain: «38;5;101»skipped«» in «38;5;101»0.0s«»
«1;38;5;136»[0:11]«» «1;38;5;162»✘«» bmc power on«38;5;101» › «»unlock: «38;5;162»failed (target)«»
«1;38;5;136»[0:11]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»settle: «38;5;101»waiting 5s«»
«1;38;5;136»[0:13]«» «1;38;5;130»⊟«» bmc power on«38;5;101» › «»settle: «38;5;130»canceled«»
«1;38;5;136»[0:13]«» «1;38;5;166»▮«» bmc power on«38;5;101» › «»report: «38;5;166»start«»
«1;38;5;136»[0:14]«» «1»✓«» bmc power on«38;5;101» › «»report: «1»done«» in «38;5;101»1.5s«»
«1;38;5;130»⊟«» «1»bmc power on«»: «38;5;130»canceled«» in «38;5;101»14s«»: «1»2 ok«», «38;5;162»1 failed«», «38;5;130»3 canceled«», «38;5;101»2 skipped«»
`},
		{name: "tide in 16 colours", o: display.PlainOptions{Theme: display.Tide.In(display.Colours16)}, styled: `
«32»[0:00]«» «32»↝«» bmc power on«2» › «»power on: «32»start«», 6 targets, 2 at a time
«32»[0:00]«» «32»↝«» bmc power on«2» › «»power on«2» › «»batch 1/2: «32»start«», 5 targets
«32»[0:01]«» «31»✕«» bmc power on«2» › «»power on«2» › «»batch 1/2«2» › «»exe1 «31»failed (transport)«»: exe1.mgmt: dial tcp: i/o timeout
«32»[0:03]«» «32»↝«» bmc power on«2» › «»power on«2» › «»batch 1/2«2» › «»stagger: «2»waiting 30s«»
«32»[0:10]«» «32»↝«» bmc power on«2» › «»power on: batch 1/2, 3/6 done, «31»1 failed«», «35»1 canceled«», «2»1 skipped«», «32»1 running«», «2»2 queued«», «2»waiting«»
«32»[0:10]«» «31»✕«» bmc power on«2» › «»power on«2» › «»batch 1/2«2» › «»stagger: «31»failed (transport)«»
«32»[0:10]«» «31»✕«» bmc power on«2» › «»power on«2» › «»batch 1/2: «31»failed«» in «2»10s«»: «36»2 ok«», «31»1 failed«», «35»1 canceled«», «2»1 skipped«»
«32»[0:10]«» «2»↷«» bmc power on«2» › «»power on«2» › «»batch 2/2: «2»skipped«»: not tried: an earlier batch failed
«32»[0:10]«» «31»✕«» bmc power on«2» › «»power on: «31»failed«» in «2»10s«»: «36»2 ok«», «31»1 failed«», «35»1 canceled«», «2»2 skipped«»
«32»[0:10]«» «32»↝«» bmc power on«2» › «»power off: «32»start«», 2 targets
«32»[0:10]«» «35»⊖«» bmc power on«2» › «»power off«2» › «»batch 1/1: «35»canceled«»
«32»[0:10]«» «35»⊖«» bmc power on«2» › «»power off: «35»canceled«» in «2»0.0s«»: 0 ok, «35»2 canceled«»
«32»[0:10]«» «32»↝«» bmc power on«2» › «»check Slurm jobs: «32»start«»
«32»[0:11]«» «35»⊖«» bmc power on«2» › «»check Slurm jobs: «35»canceled«» in «2»1.0s«»
«32»[0:11]«» «32»↝«» bmc power on«2» › «»drain: «32»start«»
«32»[0:11]«» «2»↷«» bmc power on«2» › «»drain: «2»skipped«» in «2»0.0s«»
«32»[0:11]«» «31»✕«» bmc power on«2» › «»unlock: «31»failed (target)«»
«32»[0:11]«» «32»↝«» bmc power on«2» › «»settle: «2»waiting 5s«»
«32»[0:13]«» «35»⊖«» bmc power on«2» › «»settle: «35»canceled«»
«32»[0:13]«» «32»↝«» bmc power on«2» › «»report: «32»start«»
«32»[0:14]«» «36»✓«» bmc power on«2» › «»report: «36»done«» in «2»1.5s«»
«35»⊖«» «1»bmc power on«»: «35»canceled«» in «2»14s«»: «36»2 ok«», «31»1 failed«», «35»3 canceled«», «2»2 skipped«»
`},
		{name: "neon in no colours", o: display.PlainOptions{Theme: display.Neon.In(display.NoColours)}, styled: `
[0:00] ➤ bmc power on › power on: start, 6 targets, 2 at a time
[0:00] ➤ bmc power on › power on › batch 1/2: start, 5 targets
[0:01] ✘ bmc power on › power on › batch 1/2 › exe1 failed (transport): exe1.mgmt: dial tcp: i/o timeout
[0:03] ➤ bmc power on › power on › batch 1/2 › stagger: waiting 30s
[0:10] ➤ bmc power on › power on: batch 1/2, 3/6 done, 1 failed, 1 canceled, 1 skipped, 1 running, 2 queued, waiting
[0:10] ✘ bmc power on › power on › batch 1/2 › stagger: failed (transport)
[0:10] ✘ bmc power on › power on › batch 1/2: failed in 10s: 2 ok, 1 failed, 1 canceled, 1 skipped
[0:10] ⇥ bmc power on › power on › batch 2/2: skipped: not tried: an earlier batch failed
[0:10] ✘ bmc power on › power on: failed in 10s: 2 ok, 1 failed, 1 canceled, 2 skipped
[0:10] ➤ bmc power on › power off: start, 2 targets
[0:10] ⊘ bmc power on › power off › batch 1/1: canceled
[0:10] ⊘ bmc power on › power off: canceled in 0.0s: 0 ok, 2 canceled
[0:10] ➤ bmc power on › check Slurm jobs: start
[0:11] ⊘ bmc power on › check Slurm jobs: canceled in 1.0s
[0:11] ➤ bmc power on › drain: start
[0:11] ⇥ bmc power on › drain: skipped in 0.0s
[0:11] ✘ bmc power on › unlock: failed (target)
[0:11] ➤ bmc power on › settle: waiting 5s
[0:13] ⊘ bmc power on › settle: canceled
[0:13] ➤ bmc power on › report: start
[0:14] ✓ bmc power on › report: done in 1.5s
⊘ bmc power on: canceled in 14s: 2 ok, 1 failed, 3 canceled, 2 skipped
`},
		{name: "classic in ASCII", o: display.PlainOptions{Theme: display.Classic, ASCII: true}, text: `
[0:00] * bmc power on > power on: start, 6 targets, 2 at a time
[0:00] * bmc power on > power on > batch 1/2: start, 5 targets
[0:01] x bmc power on > power on > batch 1/2 > exe1 failed (transport): exe1.mgmt: dial tcp: i/o timeout
[0:03] * bmc power on > power on > batch 1/2 > stagger: waiting 30s
[0:10] * bmc power on > power on: batch 1/2, 3/6 done, 1 failed, 1 canceled, 1 skipped, 1 running, 2 queued, waiting
[0:10] x bmc power on > power on > batch 1/2 > stagger: failed (transport)
[0:10] x bmc power on > power on > batch 1/2: failed in 10s: 2 ok, 1 failed, 1 canceled, 1 skipped
[0:10] - bmc power on > power on > batch 2/2: skipped: not tried: an earlier batch failed
[0:10] x bmc power on > power on: failed in 10s: 2 ok, 1 failed, 1 canceled, 2 skipped
[0:10] * bmc power on > power off: start, 2 targets
[0:10] ~ bmc power on > power off > batch 1/1: canceled
[0:10] ~ bmc power on > power off: canceled in 0.0s: 0 ok, 2 canceled
[0:10] * bmc power on > check Slurm jobs: start
[0:11] ~ bmc power on > check Slurm jobs: canceled in 1.0s
[0:11] * bmc power on > drain: start
[0:11] - bmc power on > drain: skipped in 0.0s
[0:11] x bmc power on > unlock: failed (target)
[0:11] * bmc power on > settle: waiting 5s
[0:13] ~ bmc power on > settle: canceled
[0:13] * bmc power on > report: start
[0:14] + bmc power on > report: done in 1.5s
~ bmc power on: canceled in 14s: 2 ok, 1 failed, 3 canceled, 2 skipped
`, styled: `
«38;5;31»[0:00]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»power on: «38;5;31»start«», 6 targets, 2 at a time
«38;5;31»[0:00]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»power on«38;5;244» > «»batch 1/2: «38;5;31»start«», 5 targets
«38;5;31»[0:01]«» «1;38;5;196»x«» bmc power on«38;5;244» > «»power on«38;5;244» > «»batch 1/2«38;5;244» > «»exe1 «38;5;196»failed (transport)«»: exe1.mgmt: dial tcp: i/o timeout
«38;5;31»[0:03]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»power on«38;5;244» > «»batch 1/2«38;5;244» > «»stagger: «38;5;244»waiting 30s«»
«38;5;31»[0:10]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»power on: batch 1/2, 3/6 done, «38;5;196»1 failed«», «38;5;136»1 canceled«», «38;5;244»1 skipped«», «38;5;31»1 running«», «38;5;244»2 queued«», «38;5;244»waiting«»
«38;5;31»[0:10]«» «1;38;5;196»x«» bmc power on«38;5;244» > «»power on«38;5;244» > «»batch 1/2«38;5;244» > «»stagger: «38;5;196»failed (transport)«»
«38;5;31»[0:10]«» «1;38;5;196»x«» bmc power on«38;5;244» > «»power on«38;5;244» > «»batch 1/2: «38;5;196»failed«» in «38;5;244»10s«»: «38;5;28»2 ok«», «38;5;196»1 failed«», «38;5;136»1 canceled«», «38;5;244»1 skipped«»
«38;5;31»[0:10]«» «1;38;5;244»-«» bmc power on«38;5;244» > «»power on«38;5;244» > «»batch 2/2: «38;5;244»skipped«»: not tried: an earlier batch failed
«38;5;31»[0:10]«» «1;38;5;196»x«» bmc power on«38;5;244» > «»power on: «38;5;196»failed«» in «38;5;244»10s«»: «38;5;28»2 ok«», «38;5;196»1 failed«», «38;5;136»1 canceled«», «38;5;244»2 skipped«»
«38;5;31»[0:10]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»power off: «38;5;31»start«», 2 targets
«38;5;31»[0:10]«» «1;38;5;136»~«» bmc power on«38;5;244» > «»power off«38;5;244» > «»batch 1/1: «38;5;136»canceled«»
«38;5;31»[0:10]«» «1;38;5;136»~«» bmc power on«38;5;244» > «»power off: «38;5;136»canceled«» in «38;5;244»0.0s«»: 0 ok, «38;5;136»2 canceled«»
«38;5;31»[0:10]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»check Slurm jobs: «38;5;31»start«»
«38;5;31»[0:11]«» «1;38;5;136»~«» bmc power on«38;5;244» > «»check Slurm jobs: «38;5;136»canceled«» in «38;5;244»1.0s«»
«38;5;31»[0:11]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»drain: «38;5;31»start«»
«38;5;31»[0:11]«» «1;38;5;244»-«» bmc power on«38;5;244» > «»drain: «38;5;244»skipped«» in «38;5;244»0.0s«»
«38;5;31»[0:11]«» «1;38;5;196»x«» bmc power on«38;5;244» > «»unlock: «38;5;196»failed (target)«»
«38;5;31»[0:11]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»settle: «38;5;244»waiting 5s«»
«38;5;31»[0:13]«» «1;38;5;136»~«» bmc power on«38;5;244» > «»settle: «38;5;136»canceled«»
«38;5;31»[0:13]«» «1;38;5;31»*«» bmc power on«38;5;244» > «»report: «38;5;31»start«»
«38;5;31»[0:14]«» «1;38;5;28»+«» bmc power on«38;5;244» > «»report: «38;5;28»done«» in «38;5;244»1.5s«»
«1;38;5;136»~«» «1»bmc power on«»: «38;5;136»canceled«» in «38;5;244»14s«»: «38;5;28»2 ok«», «38;5;196»1 failed«», «38;5;136»3 canceled«», «38;5;244»2 skipped«»
`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			text, styled := plainThemeShown(plainThemeWork(t, tc.o))
			if tc.text != "" && text != tc.text[1:] {
				t.Errorf("screen:\n%s\nwant:\n%s", text, tc.text[1:])
			}
			if styled != tc.styled[1:] {
				t.Errorf("screen in its colours:\n%s\nwant:\n%s", styled, tc.styled[1:])
			}
		})
	}
}

// A theme only adds: every line in every theme, in any number of colours
// and in ASCII, is the line of no theme once its colours are taken off and
// the mark after its time, or in front of the summary, with the space
// after it. No theme draws no mark and no escape code.
func TestPlainLinesInAThemeSayTheWordsOfNone(t *testing.T) {
	t.Parallel()
	for _, o := range plainThemeOptions() {
		t.Run(plainThemeName(o), func(t *testing.T) {
			t.Parallel()
			none := plainThemeWork(t, display.PlainOptions{ASCII: o.ASCII})
			if strings.Contains(none, "\x1b") {
				t.Errorf("no theme drew an escape code: %q", none)
			}
			themed, _ := plainThemeShown(plainThemeWork(t, o))
			want := strings.Split(strings.TrimSuffix(none, "\n"), "\n")
			got := strings.Split(strings.TrimSuffix(themed, "\n"), "\n")
			if len(got) != len(want) {
				t.Fatalf("%d lines in the theme, %d with none:\n%s", len(got), len(want), themed)
			}
			for i, row := range got {
				// Each line but the summary, the last, starts with its
				// time.
				prefix := ""
				if i < len(got)-1 {
					prefix, _, _ = strings.Cut(want[i], " ")
					prefix += " "
				}
				unmarked, marked := plainThemeUnmarked(row, prefix)
				if !marked || unmarked != want[i] {
					t.Errorf("line %d is %q, marked %v; want %q with a mark", i, row, marked, want[i])
				}
			}
		})
	}
}

// Nothing a theme draws is left on at the end of a line, so that no colour
// runs on into the next line or into what the command writes.
func TestPlainLinesInAThemeLeaveNoColourOn(t *testing.T) {
	t.Parallel()
	for _, o := range plainThemeOptions() {
		t.Run(plainThemeName(o), func(t *testing.T) {
			t.Parallel()
			plainThemeLeavesNoColourOn(t, plainThemeWork(t, o))
		})
	}
}
