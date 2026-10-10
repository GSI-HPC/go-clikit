// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
	"github.com/GSI-HPC/go-clikit/termtext"
)

// treeThemeScene runs the work the themed frames show, under the command
// of ctx, on clock c, and has draw draw a frame at each moment worth one,
// given how far to move the clock and how many rows the terminal has. Steps
// end before the first frame, one of them inside another, one left out and
// one failed before it ran. The first frame has a call and a wait of the
// command's, and a counted step whose targets have ended in every way, one
// of three running with a request, two queued: the running are cut to one.
// The command is interrupted before the second, on a terminal of fewer
// rows, which cuts the failures to one. The step has ended by the third,
// and a step that waited to run has been canceled.
func treeThemeScene(ctx context.Context, c *clock, interrupt chan<- struct{}, draw func(d time.Duration, height int)) {
	bootCtx, boot := progress.Start(ctx, progress.KindStep, "boot")
	_, order := progress.Start(bootCtx, progress.KindStep, "set the boot order")
	c.Add(500 * time.Millisecond)
	order.End(nil)
	c.Add(300 * time.Millisecond)
	boot.End(nil)
	_, disarm := progress.Start(ctx, progress.KindStep, "disarm", progress.Queued())
	disarm.Skip("nothing was armed")
	_, inventory := progress.Start(ctx, progress.KindStep, "read the inventory", progress.Queued())
	inventory.End(unreachable("inventory: connection refused"))
	c.Add(200 * time.Millisecond)
	_, call := progress.Start(ctx, progress.KindCall, "ssh", progress.Node("install"), progress.Message("reading the inventory"))
	_, wait := progress.Start(ctx, progress.KindWait, "settle", progress.Timeout(15*time.Second), progress.Message("for the BMCs"))
	stepCtx, step := progress.Start(ctx, progress.KindStep, "power on", progress.WithFlags(progress.Fold), progress.Total(12))
	ctxs, spans := targets(stepCtx, nodes(12)...)
	for _, span := range spans[:7] {
		span.Run()
	}
	spans[0].End(unreachable("exe1.mgmt: dial tcp: i/o timeout"))
	spans[1].End(nil)
	spans[2].End(nil)
	spans[3].End(unreachable("exe4.mgmt: dial tcp: i/o timeout"))
	spans[4].End(context.Canceled)
	spans[5].Skip("in maintenance")
	spans[6].End(nil)
	c.Add(300 * time.Millisecond)
	for _, span := range spans[7:10] {
		span.Run()
	}
	progress.Start(ctxs[7], progress.KindCall, "redfish", progress.HTTP("PATCH", "/redfish/v1/Systems/1"), progress.Host("exe8.mgmt"))
	progress.Start(ctxs[8], progress.KindCall, "ssh", progress.Node("exe9"), progress.Timeout(10*time.Minute))
	draw(time.Second, 30)
	call.End(nil)
	wait.End(nil)
	spans[7].End(errors.New("exe8.mgmt: 400 Bad Request"))
	spans[8].End(errors.New("exe9: command exited 1"))
	spans[10].Run()
	close(interrupt)
	draw(1200*time.Millisecond, 24)
	spans[9].End(nil)
	spans[10].End(nil)
	spans[11].End(context.Canceled)
	step.End(errors.New("4 of 12 failed: exe[1,4,8-9]"))
	_, off := progress.Start(ctx, progress.KindStep, "power off", progress.Queued())
	off.End(context.Canceled)
	draw(time.Second, 24)
}

// treeThemeFrames runs the scene on a tree drawn in theme, ASCII if ascii
// says so, on a terminal width columns wide, and returns what the screen
// shows after each frame, as Styled shows it with styled set and as String
// shows it otherwise.
func treeThemeFrames(t *testing.T, theme display.Theme, ascii bool, width int, styled bool) []string {
	t.Helper()
	interrupt := make(chan struct{})
	f := newTreeFixture(t, "provision reinstall", treeSetup{width: width, ascii: ascii, theme: theme, styles: true, interrupted: interrupt})
	var frames []string
	treeThemeScene(f.ctx, f.clock, interrupt, func(d time.Duration, height int) {
		f.resize(width, height)
		f.draw(d)
		if styled {
			frames = append(frames, f.screen.Styled())
		} else {
			frames = append(frames, f.screen.String())
		}
	})
	return frames
}

// treeThemeName names theme in the colours it is drawn in, and in ASCII
// if ascii says so: "aurora in 16 colours, ASCII".
func treeThemeName(theme display.Theme, ascii bool) string {
	name := fmt.Sprintf("%v in %v colours", theme, theme.Colours())
	if ascii {
		name += ", ASCII"
	}
	return name
}

// A theme draws the tree in its art and colours: the names of the command
// and its steps in bold, the separators, guides, times and what is queued
// muted, the clock, the marks and the counts in their colours, a spinner
// for the running targets and a bar on the row of the counted step, and
// the names of targets, errors, requests and the interrupt's counts in the
// terminal's own colour. The lines of the steps that ended are coloured
// alike, the outcome of a step that never ran in the colour of how it
// ended and its reason in none. In 16 colours a theme draws the terminal's
// own, and in none its art alone.
func TestTheTreeInATheme(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		theme  display.Theme
		frames []string
	}{
		{display.Aurora, []string{`
«1;38;5;30»✓«» «1»boot«38;5;244» › «1»set the boot order«»  «38;5;244»0.5s«»
«1;38;5;30»✓«» «1»boot«»  «38;5;244»0.8s«»
«1;38;5;244»◌«» «1»disarm«»  «38;5;244»skipped«»: nothing was armed
«1;38;5;196»✗«» «1»read the inventory«»  «38;5;196»failed (transport)«»
«1»provision reinstall«38;5;244» ∙ «38;5;68»0:02.3«»
«38;5;244»⋮ «»ssh install reading the inventory  «38;5;244»1.3s«»
«38;5;244»⋮ «1»settle«»  «38;5;244»14s left«»  for the BMCs
«38;5;244»⋮ «1»power on«»  «38;5;30»⣿⣿«38;5;31»⣿⣿«38;5;196»⣿«38;5;244»⣀⣀⣀⣀⣀«» 7/12«38;5;244» ∙ «38;5;196»2 failed«38;5;244» ∙ «38;5;103»1 canceled«38;5;244» ∙ 1 skipped ∙ «38;5;98»3 running«38;5;244» ∙ 2 queued«»
  «38;5;244»⋮ «1;38;5;196»✗«» exe[1,4]  transport: {}: dial tcp: i/o timeout
  «38;5;244»⋮ «1;38;5;103»⊘«» exe5  «38;5;103»canceled«»
  «38;5;244»⋮ «1;38;5;244»◌«» exe6  «38;5;244»skipped«»: in maintenance
  «38;5;244»⋮ «38;5;98»⠸«» exe8  «38;5;244»1.0s«»  PATCH /redfish/v1/Systems/1
  «38;5;244»⋮ ⋯ 2 more running«»
  «38;5;244»⋮ «1;38;5;30»✓«» exe[2-3,7]
`, `
«1;38;5;30»✓«» «1»boot«38;5;244» › «1»set the boot order«»  «38;5;244»0.5s«»
«1;38;5;30»✓«» «1»boot«»  «38;5;244»0.8s«»
«1;38;5;244»◌«» «1»disarm«»  «38;5;244»skipped«»: nothing was armed
«1;38;5;196»✗«» «1»read the inventory«»  «38;5;196»failed (transport)«»
«1»provision reinstall«38;5;244» ∙ «38;5;103»interrupting«38;5;244» ∙ «»2 running will stop«38;5;244» ∙ «»1 queued will not start«38;5;244» ∙ «38;5;68»0:03.5«»
«38;5;244»⋮ «1»power on«»  «38;5;30»⣿⣿«38;5;31»⣿⣿«38;5;196»⣿⣿⣿«38;5;244»⣀⣀⣀«» 9/12«38;5;244» ∙ «38;5;196»4 failed«38;5;244» ∙ «38;5;103»1 canceled«38;5;244» ∙ 1 skipped ∙ «38;5;98»2 running«38;5;244» ∙ 1 queued«»
  «38;5;244»⋮ «1;38;5;196»✗«» exe[1,4]  transport: {}: dial tcp: i/o timeout
  «38;5;244»⋮ «1;38;5;196»✗«» «38;5;244»⋯ 2 more failed«»
  «38;5;244»⋮ «1;38;5;103»⊘«» exe5  «38;5;103»canceled«»
  «38;5;244»⋮ «1;38;5;244»◌«» exe6  «38;5;244»skipped«»: in maintenance
  «38;5;244»⋮ ⋯ 2 running«»
  «38;5;244»⋮ «1;38;5;30»✓«» exe[2-3,7]
`, `
«1;38;5;30»✓«» «1»boot«38;5;244» › «1»set the boot order«»  «38;5;244»0.5s«»
«1;38;5;30»✓«» «1»boot«»  «38;5;244»0.8s«»
«1;38;5;244»◌«» «1»disarm«»  «38;5;244»skipped«»: nothing was armed
«1;38;5;196»✗«» «1»read the inventory«»  «38;5;196»failed (transport)«»
«1;38;5;196»✗«» «1»power on«»  «38;5;244»2.5s«»  «38;5;30»5 ok«», «38;5;196»4 failed«», «38;5;103»2 canceled«», «38;5;244»1 skipped«»
«38;5;244»⋮ «1;38;5;196»✗«» exe[1,4]  transport: {}: dial tcp: i/o timeout
«38;5;244»⋮ «1;38;5;196»✗«» exe8  target: {}: 400 Bad Request
«38;5;244»⋮ «1;38;5;196»✗«» exe9  target: {}: command exited 1
«1;38;5;103»⊘«» «1»power off«»  «38;5;103»canceled«»
«1»provision reinstall«38;5;244» ∙ «38;5;103»interrupting«38;5;244» ∙ «38;5;68»0:04.5«»
`}},
		{display.Ember.In(display.Colours16), []string{`
«1»✓«» «1»boot«2» » «1»set the boot order«»  «2»0.5s«»
«1»✓«» «1»boot«»  «2»0.8s«»
«2»▢«» «1»disarm«»  «2»skipped«»: nothing was armed
«31»✘«» «1»read the inventory«»  «31»failed (transport)«»
«1»provision reinstall«2» ╏ «33»0:02.3«»
«2»╎ «»ssh install reading the inventory  «2»1.3s«»
«2»╎ «1»settle«»  «2»14s left«»  for the BMCs
«2»╎ «1»power on«»  «33»▮▮▮▮«31»▮«2»▯▯▯▯▯«» 7/12«2» ╏ «31»2 failed«2» ╏ «35»1 canceled«2» ╏ 1 skipped ╏ «33»3 running«2» ╏ 2 queued«»
  «2»╎ «31»✘«» exe[1,4]  transport: {}: dial tcp: i/o timeout
  «2»╎ «35»⊟«» exe5  «35»canceled«»
  «2»╎ ▢«» exe6  «2»skipped«»: in maintenance
  «2»╎ «33»▗«» exe8  «2»1.0s«»  PATCH /redfish/v1/Systems/1
  «2»╎ ⋯ 2 more running«»
  «2»╎ «1»✓«» exe[2-3,7]
`, `
«1»✓«» «1»boot«2» » «1»set the boot order«»  «2»0.5s«»
«1»✓«» «1»boot«»  «2»0.8s«»
«2»▢«» «1»disarm«»  «2»skipped«»: nothing was armed
«31»✘«» «1»read the inventory«»  «31»failed (transport)«»
«1»provision reinstall«2» ╏ «35»interrupting«2» ╏ «»2 running will stop«2» ╏ «»1 queued will not start«2» ╏ «33»0:03.5«»
«2»╎ «1»power on«»  «33»▮▮▮▮«31»▮▮▮«2»▯▯▯«» 9/12«2» ╏ «31»4 failed«2» ╏ «35»1 canceled«2» ╏ 1 skipped ╏ «33»2 running«2» ╏ 1 queued«»
  «2»╎ «31»✘«» exe[1,4]  transport: {}: dial tcp: i/o timeout
  «2»╎ «31»✘«» «2»⋯ 2 more failed«»
  «2»╎ «35»⊟«» exe5  «35»canceled«»
  «2»╎ ▢«» exe6  «2»skipped«»: in maintenance
  «2»╎ ⋯ 2 running«»
  «2»╎ «1»✓«» exe[2-3,7]
`, `
«1»✓«» «1»boot«2» » «1»set the boot order«»  «2»0.5s«»
«1»✓«» «1»boot«»  «2»0.8s«»
«2»▢«» «1»disarm«»  «2»skipped«»: nothing was armed
«31»✘«» «1»read the inventory«»  «31»failed (transport)«»
«31»✘«» «1»power on«»  «2»2.5s«»  «1»5 ok«», «31»4 failed«», «35»2 canceled«», «2»1 skipped«»
«2»╎ «31»✘«» exe[1,4]  transport: {}: dial tcp: i/o timeout
«2»╎ «31»✘«» exe8  target: {}: 400 Bad Request
«2»╎ «31»✘«» exe9  target: {}: command exited 1
«35»⊟«» «1»power off«»  «35»canceled«»
«1»provision reinstall«2» ╏ «35»interrupting«2» ╏ «33»0:04.5«»
`}},
		{display.Tide.In(display.NoColours), []string{`
✓ boot ⟩ set the boot order  0.5s
✓ boot  0.8s
↷ disarm  skipped: nothing was armed
✕ read the inventory  failed (transport)
provision reinstall ◦ 0:02.3
╎ ssh install reading the inventory  1.3s
╎ settle  14s left  for the BMCs
╎ power on  ◉◉◉◉◉◌◌◌◌◌ 7/12 ◦ 2 failed ◦ 1 canceled ◦ 1 skipped ◦ 3 running ◦ 2 queued
  ╎ ✕ exe[1,4]  transport: {}: dial tcp: i/o timeout
  ╎ ⊖ exe5  canceled
  ╎ ↷ exe6  skipped: in maintenance
  ╎ ◟ exe8  1.0s  PATCH /redfish/v1/Systems/1
  ╎ ⋯ 2 more running
  ╎ ✓ exe[2-3,7]
`, `
✓ boot ⟩ set the boot order  0.5s
✓ boot  0.8s
↷ disarm  skipped: nothing was armed
✕ read the inventory  failed (transport)
provision reinstall ◦ interrupting ◦ 2 running will stop ◦ 1 queued will not start ◦ 0:03.5
╎ power on  ◉◉◉◉◉◉◉◌◌◌ 9/12 ◦ 4 failed ◦ 1 canceled ◦ 1 skipped ◦ 2 running ◦ 1 queued
  ╎ ✕ exe[1,4]  transport: {}: dial tcp: i/o timeout
  ╎ ✕ ⋯ 2 more failed
  ╎ ⊖ exe5  canceled
  ╎ ↷ exe6  skipped: in maintenance
  ╎ ⋯ 2 running
  ╎ ✓ exe[2-3,7]
`, `
✓ boot ⟩ set the boot order  0.5s
✓ boot  0.8s
↷ disarm  skipped: nothing was armed
✕ read the inventory  failed (transport)
✕ power on  2.5s  5 ok, 4 failed, 2 canceled, 1 skipped
╎ ✕ exe[1,4]  transport: {}: dial tcp: i/o timeout
╎ ✕ exe8  target: {}: 400 Bad Request
╎ ✕ exe9  target: {}: command exited 1
⊖ power off  canceled
provision reinstall ◦ interrupting ◦ 0:04.5
`}},
	} {
		frames := treeThemeFrames(t, tc.theme, false, 100, true)
		for i, frame := range frames {
			if want := tc.frames[i][1:]; frame != want {
				t.Errorf("%s, frame %d:\n%s\nwant:\n%s", treeThemeName(tc.theme, false), i+1, frame, want)
			}
		}
	}
}

// With the ASCII option a theme draws its colours with the ASCII marks, a
// bar of # and no spinner or guide: nothing but ASCII.
func TestTheTreeInAThemeInASCII(t *testing.T) {
	t.Parallel()
	frames := treeThemeFrames(t, display.Aurora, true, 100, false)
	for i, want := range []string{`
+ boot > set the boot order  0.5s
+ boot  0.8s
- disarm  skipped: nothing was armed
x read the inventory  failed (transport)
provision reinstall - 0:02.3
  ssh install reading the inventory  1.3s
  settle  14s left  for the BMCs
  power on  [#####.....] 7/12 - 2 failed - 1 canceled - 1 skipped - 3 running - 2 queued
    x exe[1,4]  transport: {}: dial tcp: i/o timeout
    ~ exe5  canceled
    - exe6  skipped: in maintenance
    > exe8  1.0s  PATCH /redfish/v1/Systems/1
    ... 2 more running
    + exe[2-3,7]
`, `
+ boot > set the boot order  0.5s
+ boot  0.8s
- disarm  skipped: nothing was armed
x read the inventory  failed (transport)
provision reinstall - interrupting - 2 running will stop - 1 queued will not start - 0:03.5
  power on  [#######...] 9/12 - 4 failed - 1 canceled - 1 skipped - 2 running - 1 queued
    x exe[1,4]  transport: {}: dial tcp: i/o timeout
    x ... 2 more failed
    ~ exe5  canceled
    - exe6  skipped: in maintenance
    ... 2 running
    + exe[2-3,7]
`, `
+ boot > set the boot order  0.5s
+ boot  0.8s
- disarm  skipped: nothing was armed
x read the inventory  failed (transport)
x power on  2.5s  5 ok, 4 failed, 2 canceled, 1 skipped
  x exe[1,4]  transport: {}: dial tcp: i/o timeout
  x exe8  target: {}: 400 Bad Request
  x exe9  target: {}: command exited 1
~ power off  canceled
provision reinstall - interrupting - 0:04.5
`} {
		checkScreen(t, frames[i], want)
	}
	styled := treeThemeFrames(t, display.Aurora, true, 100, true)
	checkScreen(t, styled[0], `
«1;38;5;30»+«» «1»boot«38;5;244» > «1»set the boot order«»  «38;5;244»0.5s«»
«1;38;5;30»+«» «1»boot«»  «38;5;244»0.8s«»
«1;38;5;244»-«» «1»disarm«»  «38;5;244»skipped«»: nothing was armed
«1;38;5;196»x«» «1»read the inventory«»  «38;5;196»failed (transport)«»
«1»provision reinstall«38;5;244» - «38;5;68»0:02.3«»
  ssh install reading the inventory  «38;5;244»1.3s«»
  «1»settle«»  «38;5;244»14s left«»  for the BMCs
  «1»power on«»  «38;5;244»[«38;5;30»##«38;5;31»##«38;5;196»#«38;5;244».....]«» 7/12«38;5;244» - «38;5;196»2 failed«38;5;244» - «38;5;103»1 canceled«38;5;244» - 1 skipped - «38;5;98»3 running«38;5;244» - 2 queued«»
    «1;38;5;196»x«» exe[1,4]  transport: {}: dial tcp: i/o timeout
    «1;38;5;103»~«» exe5  «38;5;103»canceled«»
    «1;38;5;244»-«» exe6  «38;5;244»skipped«»: in maintenance
    «38;5;98»>«» exe8  «38;5;244»1.0s«»  PATCH /redfish/v1/Systems/1
    «38;5;244»... 2 more running«»
    «1;38;5;30»+«» exe[2-3,7]
`)
	for _, frame := range frames {
		for _, r := range frame {
			if r > 0x7e {
				t.Fatalf("the tree in Aurora in ASCII drew %q:\n%s", r, frame)
			}
		}
	}
}

// The bar is drawn on the row of a counted step only while the row fits
// the terminal with it, up to the end of the count, the call after it
// cut off as it may be; on a terminal one column narrower the row is drawn
// without the bar, as with no theme.
func TestTheBarOfATreeGivesWay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		width int
		want  string
	}{
		{100, `
«1»bmc power on«38;5;103» ⋄ «1;38;5;134»0:01.3«»
«38;5;103»╏ «1»power on«»  «38;5;33»▰▰«38;5;69»▰▰«38;5;197»▰«38;5;103»▱▱▱▱▱«» 4/8«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»2 running«38;5;103» ⋄ 2 queued«»  «38;5;103»1.3s/30s«»  ssh install
  «38;5;103»╏ «1;38;5;197»✘«» exe4  target: {}: refused
  «38;5;103»╏ «38;5;135»◶«» exe5  «38;5;103»1.3s«»
  «38;5;103»╏ «38;5;135»◶«» exe6  «38;5;103»1.3s«»
  «38;5;103»╏ «1;38;5;33»✓«» exe[1-3]
`},
		{61, `
«1»bmc power on«38;5;103» ⋄ «1;38;5;134»0:01.3«»
«38;5;103»╏ «1»power on«»  «38;5;33»▰▰«38;5;69»▰▰«38;5;197»▰«38;5;103»▱▱▱▱▱«» 4/8«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»2 running«38;5;103» ⋄ 2 queued«»
  «38;5;103»╏ «1;38;5;197»✘«» exe4  target: {}: refused
  «38;5;103»╏ «38;5;135»◶«» exe5  «38;5;103»1.3s«»
  «38;5;103»╏ «38;5;135»◶«» exe6  «38;5;103»1.3s«»
  «38;5;103»╏ «1;38;5;33»✓«» exe[1-3]
`},
		{60, `
«1»bmc power on«38;5;103» ⋄ «1;38;5;134»0:01.3«»
«38;5;103»╏ «1»power on«»  4/8«38;5;103» ⋄ «38;5;197»1 failed«38;5;103» ⋄ «38;5;135»2 running«38;5;103» ⋄ 2 queued«»  «38;5;103»1.3s/30s«»
  «38;5;103»╏ «1;38;5;197»✘«» exe4  target: {}: refused
  «38;5;103»╏ «38;5;135»◶«» exe5  «38;5;103»1.3s«»
  «38;5;103»╏ «38;5;135»◶«» exe6  «38;5;103»1.3s«»
  «38;5;103»╏ «1;38;5;33»✓«» exe[1-3]
`},
	} {
		f := newTreeFixture(t, "bmc power on", treeSetup{width: tc.width, theme: display.Neon, styles: true})
		ctx, _ := progress.Start(f.ctx, progress.KindStep, "power on", progress.WithFlags(progress.Fold), progress.Total(8))
		_, spans := targets(ctx, nodes(8)...)
		for i, span := range spans[:6] {
			span.Run()
			if i < 4 {
				var err error
				if i == 3 {
					err = errors.New("exe4: refused")
				}
				span.End(err)
			}
		}
		progress.Start(ctx, progress.KindCall, "ssh", progress.Node("install"), progress.Timeout(30*time.Second))
		f.draw(1300 * time.Millisecond)
		if got := f.screen.Styled(); got != tc.want[1:] {
			t.Errorf("Neon on %d columns:\n%s\nwant:\n%s", tc.width, got, tc.want[1:])
		}
	}
}

// Rows that never fit are cut off at the bottom of the region, the last
// saying in the muted colour that there is more; a batch is named in bold,
// "batch" with it.
func TestATreeInAThemeCutsOffWhatNeverFits(t *testing.T) {
	t.Parallel()
	f := newTreeFixture(t, "bmc power on", treeSetup{height: 18, theme: display.Classic, styles: true})
	ctx, _ := progress.Start(f.ctx, progress.KindStep, "power on", progress.WithFlags(progress.Fold), progress.Total(4))
	batchCtx, batch := progress.Start(ctx, progress.KindBatch, "1/2", progress.Queued(),
		progress.Batch(1, 2), progress.Node("exe[1-2]"), progress.Total(2))
	progress.Start(ctx, progress.KindBatch, "2/2", progress.Queued(), progress.Batch(2, 2), progress.Node("exe[3-4]"), progress.Total(2))
	batch.Run()
	_, spans := targets(batchCtx, "exe1", "exe2")
	for _, span := range spans {
		span.Run()
	}
	for i := range 6 {
		progress.Start(f.ctx, progress.KindStep, fmt.Sprintf("step %d", i+1))
	}
	f.draw(time.Second)
	checkScreen(t, f.screen.Styled(), `
«1»bmc power on«38;5;244» ⋅ «38;5;31»0:01.0«»
  «1»power on«»  0/4«38;5;244» ⋅ «38;5;31»2 running«38;5;244» ⋅ 2 queued«»
    «1»batch 1/2«»  0/2«38;5;244» ⋅ «38;5;31»2 running«»
      «38;5;244»⋯ 2 running«»
  «1»step 1«»  «38;5;244»1.0s«»
«38;5;244»⋯«»
`)
}

// The running mark of a theme turns with the tree's clock, a frame of the
// spinner every tenth of a second in Aurora: a frame drawn at the same
// instant draws the same.
func TestTheSpinnerOfATreeTurnsWithItsClock(t *testing.T) {
	t.Parallel()
	f := newTreeFixture(t, "exec", treeSetup{theme: display.Aurora.In(display.NoColours), styles: true})
	ctx, _ := progress.Start(f.ctx, progress.KindStep, "run", progress.WithFlags(progress.Fold), progress.Total(1))
	_, spans := targets(ctx, "exe1")
	spans[0].Run()
	var marks []string
	for i := range 12 {
		d := 100 * time.Millisecond
		if i == 0 {
			d = time.Second
		}
		rows := strings.Split(f.draw(d), "\n")
		// The last row is blank, after the newline that ends the target's:
		// "  ⋮ ⠋ exe1  1.0s".
		marks = append(marks, strings.Fields(rows[len(rows)-2])[1])
	}
	if got, want := strings.Join(marks, ""), "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋⠙"; got != want {
		t.Errorf("the spinner turned %s, want %s", got, want)
	}
}

// In a theme a running target's row shows the last line of its output in
// the terminal's own colour, as it shows the request it waits for: the
// line is the command's, not the theme's.
func TestATreeInAThemeLeavesTheOutputInTheTerminalsColour(t *testing.T) {
	t.Parallel()
	f := newTreeFixture(t, "cinc run", treeSetup{theme: display.Tide, styles: true})
	ctx, _ := progress.Start(f.ctx, progress.KindStep, "run",
		progress.WithFlags(progress.Fold|progress.ShowLines), progress.Total(2))
	ctxs, spans := targets(ctx, "exe1", "exe2")
	for i, span := range spans {
		span.Run()
		callCtx, _ := progress.Start(ctxs[i], progress.KindCall, "ssh", progress.Timeout(30*time.Minute))
		if i == 0 {
			out := progress.Tee(callCtx, io.Discard, progress.Stdout, nil)
			_, _ = io.WriteString(out, "Starting Cinc Client\nConverging 12 resources\n")
		}
	}
	f.draw(90 * time.Second)
	checkScreen(t, f.screen.Styled(), `
«1»cinc run«38;5;66» ◦ «38;5;31»1:30.0«»
«38;5;66»╎ «1»run«»  «38;5;66»◌◌◌◌◌◌◌◌◌◌«» 0/2«38;5;66» ◦ «38;5;29»2 running«»
  «38;5;66»╎ «38;5;29»◜«» exe1  «38;5;66»1m30.0s/30m«»  Converging 12 resources
  «38;5;66»╎ «38;5;29»◜«» exe2  «38;5;66»1m30.0s/30m«»  ssh
`)
}

// A frame that has no rows left, once the command has ended, but writes the
// lines of the steps that ended, sets the colours back before them, as a
// frame with rows does: a colour the command left on does not tint them.
func TestATreeInAThemeSetsTheColoursBackBeforeTheLinesItLeaves(t *testing.T) {
	t.Parallel()
	var raw bytes.Buffer
	term := display.NewTerminal(&raw, display.TerminalOptions{Size: func() (int, int, error) { return 100, 24, nil }})
	c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
	tree := display.NewTree(term, display.TreeOptions{Now: c.Now, Theme: display.Aurora})
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture, tree}, Now: c.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "exec")
	_, step := progress.Start(ctx, progress.KindStep, "boot")
	c.Add(time.Second)
	tree.Draw()
	step.End(nil)
	command.End(nil)
	raw.Reset()
	tree.Draw()
	bus.Close()
	tree.Close()
	progresstest.Check(t, capture.Events())
	// The frame takes the two rows drawn before off, and writes the line.
	const want = "\r\x1b[2K\x1b[1A\x1b[2K\x1b[0m\x1b[1;38;5;30m✓\x1b[0m \x1b[1mboot\x1b[0m  \x1b[38;5;244m1.0s\x1b[0m\n"
	if got := raw.String(); got != want {
		t.Errorf("the last frame wrote %q, want %q", got, want)
	}
}

// treeThemeMarked is a terminal that writes to screen what it is given
// with a | at the end of every row and of every write, in the colours on
// there, so that the screen's Styled shows a row that leaves a colour on as
// one that ends in that colour: «38;5;68»0:02.3|«».
type treeThemeMarked struct{ screen *progresstest.Screen }

func (m treeThemeMarked) Write(p []byte) (int, error) {
	_, _ = m.screen.Write(append(bytes.ReplaceAll(p, []byte("\n"), []byte("|\n")), '|'))
	return len(p), nil
}

// treeThemeSealed fails the test for each row of styled, the Styled of a
// screen under a treeThemeMarked, that ends in a colour.
func treeThemeSealed(t *testing.T, what, styled string) {
	t.Helper()
	for i, row := range strings.Split(strings.TrimSuffix(styled, "\n"), "\n") {
		if strings.HasSuffix(row, "«»") {
			t.Errorf("%s: row %d leaves a colour on: %s", what, i+1, row)
		}
	}
}

// No colour a theme draws runs on past a row: every row of every frame,
// and every line the tree leaves, sets the colours back, in every theme,
// in each number of colours, with the ASCII marks too, so that nothing
// the command writes after it, and no row below it, takes its colour.
func TestATreeInAThemeLeavesNoColourOn(t *testing.T) {
	t.Parallel()
	for _, theme := range display.Themes() {
		for _, colours := range []display.Colours{display.Colours256, display.Colours16, display.NoColours} {
			for _, ascii := range []bool{false, true} {
				theme := theme.In(colours)
				name := treeThemeName(theme, ascii)
				marked := &progresstest.Screen{Styles: true}
				height := 0
				term := display.NewTerminal(treeThemeMarked{marked}, display.TerminalOptions{
					Size: func() (int, int, error) { return 100, height, nil },
				})
				c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
				interrupt := make(chan struct{})
				tree := display.NewTree(term, display.TreeOptions{Now: c.Now, Theme: theme, ASCII: ascii, Interrupted: interrupt})
				capture := &progresstest.Capture{}
				bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture, tree}, Now: c.Now})
				ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "provision reinstall")
				frame := 0
				treeThemeScene(ctx, c, interrupt, func(d time.Duration, rows int) {
					height = rows
					c.Add(d)
					tree.Draw()
					frame++
					treeThemeSealed(t, fmt.Sprintf("%s, frame %d", name, frame), marked.Styled())
				})
				command.End(errors.New("interrupted"))
				bus.Close()
				tree.Close()
				treeThemeSealed(t, name+", closed", marked.Styled())
				progresstest.Check(t, capture.Events())
			}
		}
	}
}

// No row of the tree wraps in any theme, with the ASCII marks or without,
// on a terminal of any width from the smallest the tree is drawn on to 120
// columns: each frame takes as many rows on the screen as with no theme,
// whose rows are cut to the width, the lines the steps leave, which wrap
// alike, among them. The bar of the counted step gives way on a terminal
// too narrow for its row with it, and only there.
func TestTheRowsOfATreeInAThemeFitEveryWidth(t *testing.T) {
	t.Parallel()
	for width := 40; width <= 120; width++ {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			t.Parallel()
			for _, ascii := range []bool{false, true} {
				plain := treeThemeFrames(t, display.Theme{}, ascii, width, false)
				for _, theme := range display.Themes() {
					name := treeThemeName(theme, ascii)
					for i, frame := range treeThemeFrames(t, theme, ascii, width, false) {
						if got, want := strings.Count(frame, "\n"), strings.Count(plain[i], "\n"); got != want {
							t.Errorf("%s, frame %d on %d columns takes %d rows, with no theme %d:\n%s",
								name, i+1, width, got, want, frame)
						}
						// Classic draws no bar; the others draw ten cells, and
						// in ASCII the brackets around them.
						cols := 0
						switch {
						case theme == display.Classic:
						case ascii:
							cols = 12
						default:
							cols = 10
						}
						treeThemeBar(t, fmt.Sprintf("%s, frame %d on %d columns", name, i+1, width), frame, width, cols)
					}
				}
			}
		})
	}
}

// treeThemeBar checks the row of the counted step of the scene in frame,
// drawn on a terminal width columns wide in a theme whose bar is cols
// columns, 0 for none: a row with the bar holds its count whole, up to
// what is queued, and a row without it, whole, would not have fitted with
// the bar and a space.
func treeThemeBar(t *testing.T, what, frame string, width, cols int) {
	t.Helper()
	for _, row := range strings.Split(frame, "\n") {
		_, rest, ok := strings.Cut(row, "power on  ")
		if !ok || rest == "" {
			continue
		}
		whole := strings.HasSuffix(row, "queued")
		switch bar := rest[0] < '0' || rest[0] > '9'; {
		case bar && !whole:
			t.Errorf("%s: the bar is drawn on a row cut short: %s", what, row)
		case !bar && whole && cols > 0 && termtext.Width(row)+1+cols <= width-1:
			t.Errorf("%s: no bar on a row that fits one: %s", what, row)
		}
	}
}

// In a theme the bar of the counted step fills with its share of the work
// done, the cells of the target that failed at its end, and each running
// target with a bounded share draws a bar of six cells before its amount;
// the amount and the rate are muted, and the share takes the colour of the
// bar. In ASCII the bars are of #.
func TestTheWorkOfATreeInATheme(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		theme display.Theme
		ascii bool
		want  string
	}{
		{display.Tide, false, `
«1»deploy«38;5;66» ◦ «38;5;31»0:35.0«»
«38;5;66»╎ «1»copying the image«»  «38;5;32»◉◉◉◉◉◉◉«38;5;166»◉«38;5;66»◌◌«» 7/12«38;5;66» ◦ «38;5;166»1 failed«38;5;66» ◦ «38;5;29»4 running«38;5;66» ◦ 1 queued ◦ 18.8 GiB ◦ «38;5;32»82%«38;5;66» ◦ 640 MiB/s ◦ ~9s left«»
  «38;5;66»╎ «1;38;5;166»✕«» exe4  transport: transport: dial tcp: i/o timeout
  «38;5;66»╎ «38;5;29»◠«» exe8  «38;5;32»◉«38;5;66»◌◌◌◌◌«» «38;5;66»0.6/2.0 GiB ◦ «38;5;32»31%«38;5;66» ◦ 64.0 MiB/s ◦ ~22s left«»  «38;5;66»10.0s«»  copy
  «38;5;66»╎ «38;5;29»◠«» exe9  «38;5;32»◉◉◉«38;5;66»◌◌◌«» «38;5;66»1.2/2.0 GiB ◦ «38;5;32»62%«38;5;66» ◦ 128 MiB/s ◦ ~6s left«»  «38;5;66»10.0s«»  copy
  «38;5;66»╎ «38;5;29»◠«» exe10  «38;5;32»◉◉◉◉◉«38;5;66»◌«» «38;5;66»1.9/2.0 GiB ◦ «38;5;32»93%«38;5;66» ◦ 192 MiB/s ◦ ~1s left«»  «38;5;66»10.0s«»  copy
  «38;5;66»╎ «38;5;29»◠«» exe11  «38;5;32»◉◉◉◉◉◉«» «38;5;66»2.5/2.0 GiB ◦ «38;5;32»100%«38;5;66» ◦ 256 MiB/s«»  «38;5;66»10.0s«»  copy
  «38;5;66»╎ «1;38;5;32»✓«» exe[1-3,5-7]
`},
		{display.Tide, true, `
«1»deploy«38;5;66» - «38;5;31»0:35.0«»
  «1»copying the image«»  «38;5;66»[«38;5;32»#######«38;5;166»#«38;5;66»..]«» 7/12«38;5;66» - «38;5;166»1 failed«38;5;66» - «38;5;29»4 running«38;5;66» - 1 queued - 18.8 GiB - «38;5;32»82%«38;5;66» - 640 MiB/s - ~9s left«»
    «1;38;5;166»x«» exe4  transport: transport: dial tcp: i/o timeout
    «38;5;29»>«» exe8  «38;5;66»[«38;5;32»#«38;5;66».....]«» «38;5;66»0.6/2.0 GiB - «38;5;32»31%«38;5;66» - 64.0 MiB/s - ~22s left«»  «38;5;66»10.0s«»  copy
    «38;5;29»>«» exe9  «38;5;66»[«38;5;32»###«38;5;66»...]«» «38;5;66»1.2/2.0 GiB - «38;5;32»62%«38;5;66» - 128 MiB/s - ~6s left«»  «38;5;66»10.0s«»  copy
    «38;5;29»>«» exe10  «38;5;66»[«38;5;32»#####«38;5;66».]«» «38;5;66»1.9/2.0 GiB - «38;5;32»93%«38;5;66» - 192 MiB/s - ~1s left«»  «38;5;66»10.0s«»  copy
    «38;5;29»>«» exe11  «38;5;66»[«38;5;32»######«38;5;66»]«» «38;5;66»2.5/2.0 GiB - «38;5;32»100%«38;5;66» - 256 MiB/s«»  «38;5;66»10.0s«»  copy
    «1;38;5;32»+«» exe[1-3,5-7]
`},
	} {
		f := newTreeFixture(t, "deploy", treeSetup{width: 140, theme: tc.theme, ascii: tc.ascii, styles: true})
		copyImage(f)
		if got := f.screen.Styled(); got != tc.want[1:] {
			t.Errorf("%s:\n%s\nwant:\n%s", treeThemeName(tc.theme, tc.ascii), got, tc.want[1:])
		}
	}
}
