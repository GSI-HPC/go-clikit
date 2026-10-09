// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"golang.org/x/text/width"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/termtext"
)

// pictographs are the runes of the first plane that Unicode gives the
// property Extended_Pictographic (UTS #51, Unicode 15.0), emoji and those
// kept for emoji to come, and every rune of the planes above it, where the
// rest of them are. A terminal may draw any of them as a picture two
// columns wide, an emoji's, whatever its East Asian Width says: ✔ is one
// of them, and ✓ is not.
var pictographs = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x00A9, 0x00A9, 1}, {0x00AE, 0x00AE, 1}, {0x203C, 0x203C, 1}, {0x2049, 0x2049, 1}, {0x2122, 0x2122, 1},
		{0x2139, 0x2139, 1}, {0x2194, 0x2199, 1}, {0x21A9, 0x21AA, 1}, {0x231A, 0x231B, 1}, {0x2328, 0x2328, 1},
		{0x2388, 0x2388, 1}, {0x23CF, 0x23CF, 1}, {0x23E9, 0x23F3, 1}, {0x23F8, 0x23FA, 1}, {0x24C2, 0x24C2, 1},
		{0x25AA, 0x25AB, 1}, {0x25B6, 0x25B6, 1}, {0x25C0, 0x25C0, 1}, {0x25FB, 0x25FE, 1}, {0x2600, 0x2605, 1},
		{0x2607, 0x2612, 1}, {0x2614, 0x2685, 1}, {0x2690, 0x2705, 1}, {0x2708, 0x2712, 1}, {0x2714, 0x2714, 1},
		{0x2716, 0x2716, 1}, {0x271D, 0x271D, 1}, {0x2721, 0x2721, 1}, {0x2728, 0x2728, 1}, {0x2733, 0x2734, 1},
		{0x2744, 0x2744, 1}, {0x2747, 0x2747, 1}, {0x274C, 0x274C, 1}, {0x274E, 0x274E, 1}, {0x2753, 0x2755, 1},
		{0x2757, 0x2757, 1}, {0x2763, 0x2767, 1}, {0x2795, 0x2797, 1}, {0x27A1, 0x27A1, 1}, {0x27B0, 0x27B0, 1},
		{0x27BF, 0x27BF, 1}, {0x2934, 0x2935, 1}, {0x2B05, 0x2B07, 1}, {0x2B1B, 0x2B1C, 1}, {0x2B50, 0x2B50, 1},
		{0x2B55, 0x2B55, 1}, {0x3030, 0x3030, 1}, {0x303D, 0x303D, 1}, {0x3297, 0x3297, 1}, {0x3299, 0x3299, 1},
	},
	R32:         []unicode.Range32{{0x10000, 0x10FFFF, 1}},
	LatinOffset: 2,
}

// Every glyph a theme draws takes one column, by termtext.Width, is of an
// East Asian Width no locale draws wide, and is no emoji, which a terminal
// may draw two columns wide, so that a theme never makes a row wrap: its
// marks, the frames of its spinner, its bar's cells, and the glyphs of its
// separators and guides, whose spaces are spaces.
func TestEveryGlyphOfAThemeTakesOneColumn(t *testing.T) {
	t.Parallel()
	for _, r := range "✔✖➡☑✳❤⏩😀" {
		if !unicode.Is(pictographs, r) {
			t.Errorf("%q (U+%04X), an emoji, is not among the pictographs", r, r)
		}
	}
	for _, theme := range Themes() {
		a := theme.art
		glyphs := append([]string{a.ok, a.failed, a.canceled, a.skipped, a.still, a.more, a.fill, a.track}, a.spin...)
		for _, spaced := range []string{a.sep, a.path, a.divider, a.guide} {
			glyphs = append(glyphs, strings.TrimSpace(spaced))
		}
		for _, g := range glyphs {
			if g == "" {
				continue
			}
			for _, r := range g {
				if w := termtext.Width(string(r)); w != 1 {
					t.Errorf("%s: %q (U+%04X) takes %d columns", a.name, r, r, w)
				}
				if width.LookupRune(r).Kind() == width.EastAsianAmbiguous {
					t.Errorf("%s: %q (U+%04X) is of ambiguous width", a.name, r, r)
				}
				if unicode.Is(pictographs, r) {
					t.Errorf("%s: %q (U+%04X) is, or may become, an emoji", a.name, r, r)
				}
			}
		}
		for _, spaced := range []string{a.sep, a.path, a.divider} {
			if !strings.HasPrefix(spaced, " ") || !strings.HasSuffix(spaced, " ") {
				t.Errorf("%s: separator %q is not set off by spaces", a.name, spaced)
			}
		}
		if a.guide != "" && termtext.Width(a.guide) != 2 {
			t.Errorf("%s: guide %q takes %d columns, not the two of an indent", a.name, a.guide, termtext.Width(a.guide))
		}
		// A guide stands beside the spinner on a running target's row, so it
		// is no braille cell: it would read as a frame of a braille spinner.
		if strings.ContainsFunc(a.guide, func(r rune) bool { return unicode.Is(unicode.Braille, r) }) {
			t.Errorf("%s: guide %q is braille", a.name, a.guide)
		}
		if len(a.spin) == 0 || a.every <= 0 {
			t.Errorf("%s: a spinner of %d frames, each for %v", a.name, len(a.spin), a.every)
		}
	}
}

// lookOf gives no theme the marks the displays have always drawn, in no
// colour; a theme its own art in its colours; and a theme in ASCII the ASCII
// marks in its colours, with a bar of #, no spinner and no guide.
func TestLookOf(t *testing.T) {
	t.Parallel()
	if l := lookOf(Theme{}, false); l.ok != "✓" || l.sep != " · " || l.pal != nil || l.lead || l.fill != "" || l.guide != "" {
		t.Errorf("no theme looks like %+v", l)
	}
	if l := lookOf(Theme{}, true); l.ok != "+" || l.sep != " - " || l.pal != nil || l.lead || !slices.Equal(l.spin, []string{">"}) || l.running != "*" {
		t.Errorf("no theme in ASCII looks like %+v", l)
	}
	l := lookOf(Aurora, false)
	if l.failed != "✗" || l.skipped != "◌" || l.sep != " ∙ " || l.guide != "⋮ " || l.fill != "⣿" || !l.lead ||
		l.pal != &auroraArt.p256 || len(l.spin) != 10 || l.running != "✦" {
		t.Errorf("Aurora looks like %+v", l)
	}
	if l := lookOf(Aurora.In(Colours16), false); l.pal != &auroraArt.p16 {
		t.Errorf("Aurora in 16 colours has palette %p, want %p", l.pal, &auroraArt.p16)
	}
	for _, c := range []Colours{NoColours, 9} {
		if l := lookOf(Aurora.In(c), false); l.pal != nil || l.fill != "⣿" {
			t.Errorf("Aurora in %v colours looks like %+v, want its art in none", c, l)
		}
	}
	// In ASCII a plain line's running mark is a star, which no path holds,
	// as > does between its parts.
	l = lookOf(Ember, true)
	if l.ok != "+" || l.skipped != "-" || l.guide != "" || !slices.Equal(l.spin, []string{">"}) || l.running != "*" ||
		l.open+l.fill+l.track+l.close != "[#.]" || l.pal != &emberArt.p256 || !l.lead {
		t.Errorf("Ember in ASCII looks like %+v", l)
	}
	if l := lookOf(Classic, true); l.fill != "" || l.open != "" {
		t.Errorf("Classic, which has no bar, has one in ASCII: %+v", l)
	}
}

// paint sets text in its role's colour and back after it, and leaves it
// as it is where there is no colour: no theme, a role with none, or no text.
// A mark is bold as well where the palette draws marks bold, and bold once.
func TestPaintAndMark(t *testing.T) {
	t.Parallel()
	aurora, ember, sixteen, none := lookOf(Aurora, false), lookOf(Ember, false), lookOf(Classic.In(Colours16), false), lookOf(Theme{}, false)
	for _, tc := range []struct {
		name, got, want string
	}{
		{"a role in colour", aurora.paint(roleFailed, "1 failed"), "\x1b[38;5;196m1 failed\x1b[0m"},
		{"no theme", none.paint(roleFailed, "1 failed"), "1 failed"},
		{"no text", aurora.paint(roleFailed, ""), ""},
		{"a role with no colour", lookOf(Classic, false).paint(roleBar, "x"), "x"},
		{"a mark, bold", aurora.mark(roleFailed, "✗"), "\x1b[1;38;5;196m✗\x1b[0m"},
		{"a mark already bold", ember.mark(roleOK, "✓"), "\x1b[1m✓\x1b[0m"},
		{"a mark bold and coloured already", ember.mark(roleClock, "x"), "\x1b[1;38;5;136mx\x1b[0m"},
		{"a mark in 16 colours, not bold", sixteen.mark(roleFailed, "✗"), "\x1b[31m✗\x1b[0m"},
		{"a mark of no theme", none.mark(roleFailed, "✗"), "✗"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	for status, want := range map[progress.Status]role{
		progress.StatusOK: roleOK, progress.StatusFailed: roleFailed, progress.StatusCanceled: roleCanceled, progress.StatusSkipped: roleSkipped,
	} {
		if got := statusRole(status); got != want {
			t.Errorf("statusRole(%v) = %v, want %v", status, got, want)
		}
	}
}

// The spinner shows the frame of the time since the display started, each
// frame for its theme's length, round and round; a mark that stands still
// is the same at any time, and a time before the start is the first frame.
func TestSpinner(t *testing.T) {
	t.Parallel()
	tide := lookOf(Tide.In(NoColours), false)
	for _, tc := range []struct {
		age  time.Duration
		want string
	}{
		{0, "◜"}, {199 * time.Millisecond, "◜"}, {200 * time.Millisecond, "◠"}, {time.Second, "◟"},
		{1200 * time.Millisecond, "◜"}, {-time.Second, "◜"},
	} {
		if got := tide.spinner(tc.age); got != tc.want {
			t.Errorf("Tide's spinner at %v is %q, want %q", tc.age, got, tc.want)
		}
	}
	if got := lookOf(Tide, false).spinner(0); got != "\x1b[38;5;29m◜\x1b[0m" {
		t.Errorf("Tide's spinner is drawn %q, not in the colour of what runs", got)
	}
	for _, age := range []time.Duration{0, 3 * time.Second} {
		if got := lookOf(Theme{}, false).spinner(age); got != "▸" {
			t.Errorf("no theme's running mark at %v is %q, want ▸", age, got)
		}
	}
}

// A row's indent is two columns a level; a theme with a guide draws it at
// the innermost level, muted.
func TestIndent(t *testing.T) {
	t.Parallel()
	ember := lookOf(Ember.In(NoColours), false)
	for depth, want := range []string{"", "╎ ", "  ╎ ", "    ╎ "} {
		if got := ember.indent(depth); got != want {
			t.Errorf("Ember's indent at %d is %q, want %q", depth, got, want)
		}
		if got := lookOf(Theme{}, false).indent(depth); got != strings.Repeat("  ", depth) {
			t.Errorf("no theme's indent at %d is %q", depth, got)
		}
	}
	if got := lookOf(Ember, false).indent(1); got != "\x1b[38;5;101m╎ \x1b[0m" {
		t.Errorf("Ember's guide is drawn %q, not muted", got)
	}
}

// A bar shows the share of the targets done, cut down to whole cells, with
// those that failed at its end in the colour of a failure, at least a cell
// when any did; none where the look has none or the count no total.
func TestBar(t *testing.T) {
	t.Parallel()
	count := func(done, failed, total int) progress.Count {
		return progress.Count{Done: done, Failed: failed, Total: total}
	}
	shape := lookOf(Tide.In(NoColours), false)
	for _, tc := range []struct {
		name string
		c    progress.Count
		want string
	}{
		{"none done", count(0, 0, 480), "◌◌◌◌◌◌◌◌◌◌"},
		{"less than a cell", count(47, 0, 480), "◌◌◌◌◌◌◌◌◌◌"},
		{"a cell", count(48, 0, 480), "◉◌◌◌◌◌◌◌◌◌"},
		{"half", count(240, 0, 480), "◉◉◉◉◉◌◌◌◌◌"},
		{"all but one", count(479, 0, 480), "◉◉◉◉◉◉◉◉◉◌"},
		{"all", count(480, 0, 480), "◉◉◉◉◉◉◉◉◉◉"},
		{"more than all", count(500, 0, 480), "◉◉◉◉◉◉◉◉◉◉"},
		{"no total", count(3, 0, 0), ""},
	} {
		if got := shape.bar(tc.c, 10); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := lookOf(Classic, false).bar(count(1, 0, 2), 10); got != "" {
		t.Errorf("Classic, which has no bar, draws %q", got)
	}
	if got := lookOf(Ember.In(NoColours), true).bar(count(5, 0, 10), 8); got != "[####....]" {
		t.Errorf("a bar in ASCII is %q", got)
	}
	const blue, orange, slate = "\x1b[38;5;32m", "\x1b[38;5;166m", "\x1b[38;5;66m"
	tide := lookOf(Tide, false)
	for _, tc := range []struct {
		name string
		c    progress.Count
		want string
	}{
		{"cells of one colour are set in it once", count(240, 0, 480), blue + "◉◉◉◉◉" + reset + slate + "◌◌◌◌◌" + reset},
		{"those that failed end it", count(240, 96, 480), blue + "◉◉◉" + reset + orange + "◉◉" + reset + slate + "◌◌◌◌◌" + reset},
		{"one failure takes a cell", count(1, 1, 480), orange + "◉" + reset + slate + "◌◌◌◌◌◌◌◌◌" + reset},
		{"all failed", count(480, 480, 480), orange + "◉◉◉◉◉◉◉◉◉◉" + reset},
	} {
		if got := tide.bar(tc.c, 10); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	// A gradient runs from the bar's first cell to its last, whatever is
	// filled; the caps of ASCII are muted.
	const teal, violet = "\x1b[38;5;30m", "\x1b[38;5;134m"
	if got, want := lookOf(Aurora, false).bar(count(12, 0, 12), 6), teal+"⣿"+reset+
		"\x1b[38;5;31m⣿"+reset+"\x1b[38;5;32m⣿"+reset+"\x1b[38;5;68m⣿"+reset+"\x1b[38;5;98m⣿"+reset+violet+"⣿"+reset; got != want {
		t.Errorf("Aurora's gradient is %q, want %q", got, want)
	}
	if got, want := lookOf(Ember, true).bar(count(0, 0, 4), 2), "\x1b[38;5;101m[..]\x1b[0m"; got != want {
		t.Errorf("an empty bar in ASCII is %q, want %q", got, want)
	}
}

// The palettes keep to what decision 19 promises. One of 256 colours sets a
// colour of the xterm palette, 16 to 255, not one of the terminal's own 16,
// of a middle lightness, an L* of 45 to 60, which reads on a light
// background as on a dark one; or bold alone, or nothing. One of 16 sets the
// terminal's own red, green, yellow, cyan or magenta, bold or faint: never
// bold with a colour, never blue, and never a bright colour, which some
// palettes make grey; and it draws no mark bold.
func TestPalettes(t *testing.T) {
	t.Parallel()
	sixteen := []string{"", "1", "2", "31", "32", "33", "35", "36"}
	for _, theme := range Themes() {
		a := theme.art
		for i, params := range append(a.p256.roles[:], a.p256.gradient...) {
			if params == "" || params == "1" {
				continue
			}
			n, ok := colour256Of(params)
			if !ok || n < 16 {
				t.Errorf("%s in 256 colours: %q, colour %d, sets no colour of the xterm palette", a.name, params, i)
				continue
			}
			if l := lightness(n); l < 45 || l > 60 {
				t.Errorf("%s in 256 colours: colour %d, %d, has an L* of %.1f", a.name, i, n, l)
			}
		}
		if a.p16.bold {
			t.Errorf("%s in 16 colours draws its marks bold", a.name)
		}
		for i, params := range append(a.p16.roles[:], a.p16.gradient...) {
			if !slices.Contains(sixteen, params) {
				t.Errorf("%s in 16 colours: colour %d is %q, want one of %q", a.name, i, params, sixteen)
			}
		}
	}
}

// The cells of a bar that stand for the targets that failed are told from
// the cells done and from the track by their colour alone, so in 256 colours
// that colour differs from each of theirs by a ΔE*ab of 20 or more: as a
// reader who sees every colour sees them, and as readers with deuteranopia
// or protanopia, who tell red from green poorly, see them.
func TestTheFailedEndOfABarStandsApart(t *testing.T) {
	t.Parallel()
	for _, theme := range Themes() {
		a := theme.art
		if a.fill == "" {
			continue
		}
		p := a.p256
		failed, _ := colour256Of(p.roles[roleFailed])
		for _, params := range append([]string{p.roles[roleBar], p.roles[roleMuted]}, p.gradient...) {
			other, _ := colour256Of(params)
			for _, sight := range []struct {
				name string
				m    *[3][3]float64
			}{{"in full colour", nil}, {"with deuteranopia", &deuteranopia}, {"with protanopia", &protanopia}} {
				if d := deltaE(lab(xterm(failed), sight.m), lab(xterm(other), sight.m)); d < 20 {
					t.Errorf("%s: a failure, %d, and %d differ by %.1f %s", a.name, failed, other, d, sight.name)
				}
			}
		}
	}
}

// colour256Of returns n of the parameters 38;5;n of a colour of 256, which
// "1;" may set bold before, and false for parameters that set no such
// colour.
func colour256Of(params string) (int, bool) {
	n, ok := strings.CutPrefix(strings.TrimPrefix(params, "1;"), "38;5;")
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(n)
	return i, err == nil
}

// xterm returns the red, green and blue of colour n of the xterm palette,
// 16 to 255, from 0 to 1, as xterm and most terminals draw it.
func xterm(n int) [3]float64 {
	if n >= 232 {
		v := float64(8+10*(n-232)) / 255
		return [3]float64{v, v, v}
	}
	levels := [6]float64{0, 95, 135, 175, 215, 255}
	n -= 16
	return [3]float64{levels[n/36] / 255, levels[n/6%6] / 255, levels[n%6] / 255}
}

// deuteranopia and protanopia turn the linear red, green and blue of a
// colour into those a reader with each sees (Machado, Oliveira and
// Fernandes, 2009, at full severity).
var (
	deuteranopia = [3][3]float64{{0.367322, 0.860646, -0.227968}, {0.280085, 0.672501, 0.047413}, {-0.011820, 0.042940, 1.031925}}
	protanopia   = [3][3]float64{{0.152286, 1.052583, -0.204868}, {0.114503, 0.786281, 0.099216}, {-0.003882, -0.048116, 1.051998}}
)

// lab returns the CIELAB L*, a* and b* of the sRGB colour rgb, as a reader
// whose sight m simulates sees it; nil is full colour.
func lab(rgb [3]float64, m *[3][3]float64) [3]float64 {
	var lin [3]float64
	for i, c := range rgb {
		if c <= 0.04045 {
			lin[i] = c / 12.92
		} else {
			lin[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	if m != nil {
		var seen [3]float64
		for i := range seen {
			seen[i] = min(1, max(0, m[i][0]*lin[0]+m[i][1]*lin[1]+m[i][2]*lin[2]))
		}
		lin = seen
	}
	x := (0.4124*lin[0] + 0.3576*lin[1] + 0.1805*lin[2]) / 0.95047
	y := 0.2126*lin[0] + 0.7152*lin[1] + 0.0722*lin[2]
	z := (0.0193*lin[0] + 0.1192*lin[1] + 0.9505*lin[2]) / 1.08883
	f := func(v float64) float64 {
		if v > 216.0/24389 {
			return math.Cbrt(v)
		}
		return (24389.0/27*v + 16) / 116
	}
	return [3]float64{116*f(y) - 16, 500 * (f(x) - f(y)), 200 * (f(y) - f(z))}
}

// lightness returns the L* of colour n of the xterm palette.
func lightness(n int) float64 { return lab(xterm(n), nil)[0] }

// deltaE returns how far apart two colours are, as ΔE*ab, the distance of
// their L*, a* and b*.
func deltaE(a, b [3]float64) float64 { return math.Hypot(math.Hypot(a[0]-b[0], a[1]-b[1]), a[2]-b[2]) }
