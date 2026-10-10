// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"fmt"
	"strings"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// Theme is how a display looks: the colours it draws in, and the marks,
// spinner, bar, separators and guides it draws with. A Theme is a value:
// compare it with ==, and pass it in the options of a display and in
// Summary.Theme.
//
// The zero Theme is none: a display draws as it always has, plain text with
// no escape code, which is the text a program's tests compare. The five
// themes, Classic, Aurora, Ember, Neon and Tide, each have a name, which
// Themes lists and ParseTheme reads, for a flag such as --theme. Assign to
// none of their variables: they are values to pass, as io.EOF is.
//
// A theme only adds to what a display says. The tree and the counter draw
// its marks, separators, spinner, bar and guides; plain lines and the
// summary take its colours and one mark in front, and keep their words,
// punctuation and paths. Names, errors, requests and output lines stay in
// the terminal's own colour. Every glyph of a theme takes one column: it is
// of an East Asian Width that no locale draws wide, and no emoji, which a
// terminal may draw as a picture two columns wide. The colours and art of a
// theme may change in a minor release; its name and the words it draws may
// not (decision 20).
//
// The kit reads no environment. Whether to draw in colour, and in how many
// colours, is the program's to decide: In draws a theme in the 256 colours
// it is designed in, in the terminal's own 16, or in none, its art alone,
// for NO_COLOR. A program that draws for a log, or for a terminal whose TERM
// is dumb, passes the zero Theme. With the ASCII options a display draws a
// theme's colours with ASCII marks.
type Theme struct {
	art     *art
	colours Colours
}

// The themes. Each is drawn in 256 colours; In draws it in others.
var (
	// Classic draws the marks of the zero Theme in green, red, amber and
	// blue, with a minus for what was skipped, and separators that take one
	// column in any locale: ⋅ and ⋯ where the zero Theme draws · and ….
	// It has no bar or guide, and its running mark, ▸, stands still; it
	// starts the counter's line too, which the zero Theme starts with no
	// mark.
	Classic = Theme{art: &classicArt}
	// Aurora draws in dots, as night lights: a braille spinner, a braille
	// bar that runs from teal through blue to violet, and a dotted guide,
	// with red for what failed.
	Aurora = Theme{art: &auroraArt}
	// Ember draws a warm instrument panel: a lamp that turns round a
	// square, a bar of segments in orange, dashed panel lines, ok in the
	// terminal's own colour, bold, and what failed in raspberry, which
	// stands apart from the orange of what runs for the readers who tell
	// red from green poorly too.
	Ember = Theme{art: &emberArt}
	// Neon draws an arcade at night: a sweeping quadrant, chevrons, and a
	// bar of slanted segments in a gradient from electric blue to orchid,
	// with hot pink for what failed.
	Neon = Theme{art: &neonArt}
	// Tide draws the sea: a turning arc and a bar of bubbles, with ok in
	// blue and failed in orange, which stay apart for the readers who tell
	// red from green poorly.
	Tide = Theme{art: &tideArt}
)

// Themes returns every Theme but the zero one, in a new slice: Classic,
// Aurora, Ember, Neon and Tide, each named by its String.
func Themes() []Theme { return []Theme{Classic, Aurora, Ember, Neon, Tide} }

// ParseTheme returns the Theme name names, in any case, as String writes it,
// drawn in 256 colours; "none" and "" are the zero Theme. Any other name is
// an error that quotes it as given and lists the names.
func ParseTheme(name string) (Theme, error) {
	lower := strings.ToLower(name)
	if lower == "" || lower == "none" {
		return Theme{}, nil
	}
	names := []string{"none"}
	for _, t := range Themes() {
		if t.art.name == lower {
			return t, nil
		}
		names = append(names, t.art.name)
	}
	last := len(names) - 1
	return Theme{}, fmt.Errorf("display: unknown theme %q: want %s or %s", name, strings.Join(names[:last], ", "), names[last])
}

// String returns the name of t, "aurora", or "none" for the zero Theme.
func (t Theme) String() string {
	if t.art == nil {
		return "none"
	}
	return t.art.name
}

// MarshalText returns the name of t, so that a Theme can be the value of a
// flag, with flag.TextVar.
func (t Theme) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText sets t to the Theme text names, as ParseTheme reads it, in
// the colours t is drawn in, and leaves t as it was for a name that is no
// Theme's. The zero Theme keeps no colours, so a theme read over it is
// drawn in 256: a program that picks the colours sets them with In once
// its flags are read.
func (t *Theme) UnmarshalText(text []byte) error {
	p, err := ParseTheme(string(text))
	if err != nil {
		return err
	}
	*t = p.In(t.colours)
	return nil
}

// In returns t drawn in c colours, with its own marks, spinner, bar and
// guides. The zero Theme stays the zero Theme, which draws in no colour
// whatever c is.
func (t Theme) In(c Colours) Theme {
	if t.art == nil {
		return t
	}
	return Theme{art: t.art, colours: c}
}

// Colours returns how many colours t draws in: NoColours for the zero
// Theme.
func (t Theme) Colours() Colours {
	if t.art == nil {
		return NoColours
	}
	return t.colours
}

// Colours is how many colours a Theme draws in: as many as the terminal
// shows, which the program finds out, from TERM or COLORTERM, say.
type Colours uint8

const (
	// Colours256 draws a theme as it is designed, in colours of the xterm
	// palette of 256, which xterm, VTE, Konsole, iTerm2, Terminal.app,
	// Windows Terminal, tmux and screen show, chosen to read on a light
	// background as on a dark one. It is the zero Colours.
	Colours256 Colours = iota
	// Colours16 draws a theme in the terminal's own palette, which its user
	// chose for its background: green, red, yellow, blue's neighbours cyan
	// and magenta, bold and faint, never bold with a colour and never the
	// bright colours, which some palettes make grey.
	Colours16
	// NoColours draws a theme's art alone, with no escape code: for
	// NO_COLOR. A Colours that is none of these draws as NoColours.
	NoColours
)

// String returns "256", "16" or "none", and "Colours(n)" for a value that
// is none of the Colours.
func (c Colours) String() string {
	switch c {
	case Colours256:
		return "256"
	case Colours16:
		return "16"
	case NoColours:
		return "none"
	}
	return fmt.Sprintf("Colours(%d)", uint8(c))
}

// role is what a part of a row is, which its colour follows.
type role int

const (
	// roleTitle is the name of the command, of a step, a batch and a wait.
	roleTitle role = iota
	// roleOK, roleFailed, roleCanceled and roleSkipped are how a span
	// ended: its mark, the word and the count of targets that ended so.
	roleOK
	roleFailed
	roleCanceled
	roleSkipped
	// roleRunning is what runs: the spinner, the running mark and count,
	// and the start of a step.
	roleRunning
	// roleClock is the time since the command started.
	roleClock
	// roleMuted is what frames the rest: separators, guides, durations,
	// what is queued or waits, rows that say how many more, a bar's track.
	roleMuted
	// roleBar is a bar's filled cells, unless the palette has a gradient.
	roleBar
	roleCount
)

// statusRole returns the role of a span that ended with status.
func statusRole(status progress.Status) role {
	switch status {
	case progress.StatusFailed:
		return roleFailed
	case progress.StatusCanceled:
		return roleCanceled
	case progress.StatusSkipped:
		return roleSkipped
	}
	return roleOK
}

// palette is the colours of a theme in one number of colours, as the
// parameters of the sequences that set them (SGR, ESC [ … m).
type palette struct {
	roles [roleCount]string
	// bold says a mark is drawn bold as well as in its role's colour.
	bold bool
	// gradient are the colours of a bar's filled cells from its first to
	// its last, nil for roleBar's colour throughout.
	gradient []string
}

// art is a theme's marks and colours.
type art struct {
	name                          string
	ok, failed, canceled, skipped string
	// spin are the frames of the running mark, each drawn for every.
	spin  []string
	every time.Duration
	// still is the running mark where nothing turns: plain lines.
	still                    string
	more, sep, path, divider string
	// guide is drawn at the innermost level of a row's indent, two columns
	// wide; "" for none.
	guide string
	// fill and track are a bar's cells, done and not; "" for no bar.
	fill, track string
	p256, p16   palette
}

// colour256 is the parameters of colour n of the 256.
func colour256(n int) string { return fmt.Sprintf("38;5;%d", n) }

var (
	classicArt = art{
		name: "classic", ok: "✓", failed: "✗", canceled: "⊘", skipped: "−",
		spin: []string{"▸"}, every: time.Second, still: "▸",
		more: "⋯", sep: " ⋅ ", path: " › ", divider: " | ",
		p256: palette{bold: true, roles: [roleCount]string{
			roleTitle: "1", roleOK: colour256(28), roleFailed: colour256(196), roleCanceled: colour256(136),
			roleSkipped: colour256(244), roleRunning: colour256(31), roleClock: colour256(31), roleMuted: colour256(244),
		}},
		p16: palette{roles: [roleCount]string{
			roleTitle: "1", roleOK: "32", roleFailed: "31", roleCanceled: "33",
			roleSkipped: "2", roleRunning: "36", roleClock: "36", roleMuted: "2",
		}},
	}
	auroraArt = art{
		name: "aurora", ok: "✓", failed: "✗", canceled: "⊘", skipped: "◌",
		spin: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}, every: 100 * time.Millisecond, still: "✦",
		more: "⋯", sep: " ∙ ", path: " › ", divider: " ⡇ ", guide: "⋮ ", fill: "⣿", track: "⣀",
		p256: palette{bold: true, roles: [roleCount]string{
			roleTitle: "1", roleOK: colour256(30), roleFailed: colour256(196), roleCanceled: colour256(103),
			roleSkipped: colour256(244), roleRunning: colour256(98), roleClock: colour256(68), roleMuted: colour256(244),
			roleBar: colour256(30),
		}, gradient: []string{colour256(30), colour256(31), colour256(32), colour256(68), colour256(98), colour256(134)}},
		p16: palette{roles: [roleCount]string{
			roleTitle: "1", roleOK: "36", roleFailed: "31", roleCanceled: "33",
			roleSkipped: "2", roleRunning: "35", roleClock: "35", roleMuted: "2", roleBar: "36",
		}, gradient: []string{"36", "35"}},
	}
	emberArt = art{
		name: "ember", ok: "✓", failed: "✘", canceled: "⊟", skipped: "▢",
		spin: []string{"▖", "▘", "▝", "▗"}, every: 200 * time.Millisecond, still: "▮",
		more: "⋯", sep: " ╏ ", path: " » ", divider: " ❚ ", guide: "╎ ", fill: "▮", track: "▯",
		p256: palette{bold: true, roles: [roleCount]string{
			roleTitle: "1", roleOK: "1", roleFailed: colour256(162), roleCanceled: colour256(130),
			roleSkipped: colour256(101), roleRunning: colour256(166), roleClock: "1;" + colour256(136), roleMuted: colour256(101),
			roleBar: colour256(166),
		}},
		p16: palette{roles: [roleCount]string{
			roleTitle: "1", roleOK: "1", roleFailed: "31", roleCanceled: "35",
			roleSkipped: "2", roleRunning: "33", roleClock: "33", roleMuted: "2", roleBar: "33",
		}},
	}
	neonArt = art{
		name: "neon", ok: "✓", failed: "✘", canceled: "⊘", skipped: "⇥",
		spin: []string{"◴", "◷", "◶", "◵"}, every: 200 * time.Millisecond, still: "➤",
		more: "⋯", sep: " ⋄ ", path: " ❯ ", divider: " ╏ ", guide: "╏ ", fill: "▰", track: "▱",
		p256: palette{bold: true, roles: [roleCount]string{
			roleTitle: "1", roleOK: colour256(33), roleFailed: colour256(197), roleCanceled: colour256(136),
			roleSkipped: colour256(103), roleRunning: colour256(135), roleClock: "1;" + colour256(134), roleMuted: colour256(103),
			roleBar: colour256(33),
		}, gradient: []string{colour256(33), colour256(69), colour256(63), colour256(99), colour256(135), colour256(170)}},
		p16: palette{roles: [roleCount]string{
			roleTitle: "1", roleOK: "36", roleFailed: "31", roleCanceled: "33",
			roleSkipped: "2", roleRunning: "35", roleClock: "35", roleMuted: "2", roleBar: "36",
		}, gradient: []string{"36", "35"}},
	}
	tideArt = art{
		name: "tide", ok: "✓", failed: "✕", canceled: "⊖", skipped: "↷",
		spin: []string{"◜", "◠", "◝", "◞", "◡", "◟"}, every: 200 * time.Millisecond, still: "↝",
		more: "⋯", sep: " ◦ ", path: " ⟩ ", divider: " ╎ ", guide: "╎ ", fill: "◉", track: "◌",
		p256: palette{bold: true, roles: [roleCount]string{
			roleTitle: "1", roleOK: colour256(32), roleFailed: colour256(166), roleCanceled: colour256(103),
			roleSkipped: colour256(66), roleRunning: colour256(29), roleClock: colour256(31), roleMuted: colour256(66),
			roleBar: colour256(32),
		}},
		p16: palette{roles: [roleCount]string{
			roleTitle: "1", roleOK: "36", roleFailed: "31", roleCanceled: "35",
			roleSkipped: "2", roleRunning: "32", roleClock: "32", roleMuted: "2", roleBar: "36",
		}},
	}
)

// look is what a display draws with: the marks and separators of a theme,
// or of none, in Unicode or ASCII, and the colours to draw them in.
type look struct {
	ok, failed, canceled, skipped string
	// running is the running mark that stands still, and spin the frames
	// of the one that turns, each drawn for every.
	running string
	spin    []string
	every   time.Duration
	more    string
	// sep splits the parts of a row, path the names of a step and the
	// steps it is part of, and divider the counter's segments.
	sep, path, divider string
	// guide is drawn at the innermost level of a row's indent; "" for
	// none.
	guide string
	// lead says the counter's line starts with the spinner.
	lead bool
	// open, fill, track and close draw a bar; fill is "" for no bar.
	open, fill, track, close string
	// pal is the colours, nil for none.
	pal *palette
}

var (
	// unicodeLook is the look of no theme.
	unicodeLook = look{
		ok: "✓", failed: "✗", canceled: "⊘", skipped: "–", running: "▸", spin: []string{"▸"}, every: time.Second,
		more: "…", sep: " · ", path: " › ", divider: " | ",
	}
	// asciiLook is for a terminal whose locale is not UTF-8, which would
	// show the others as garbage, with no theme or with a theme's colours.
	// Its running mark that stands still, which starts a plain line, is a
	// star, not >, which splits the path after it.
	asciiLook = look{
		ok: "+", failed: "x", canceled: "~", skipped: "-", running: "*", spin: []string{">"}, every: time.Second,
		more: "...", sep: " - ", path: " > ", divider: " | ",
	}
)

// lookOf returns what a display draws with in t: in ASCII marks, in t's
// colours, when ascii is set. A theme in ASCII has no spinner and no
// guide, and draws its bar as [###...].
func lookOf(t Theme, ascii bool) look {
	l := unicodeLook
	if ascii {
		l = asciiLook
	}
	a := t.art
	if a == nil {
		return l
	}
	l.lead = true
	if ascii {
		if a.fill != "" {
			l.open, l.fill, l.track, l.close = "[", "#", ".", "]"
		}
	} else {
		l.ok, l.failed, l.canceled, l.skipped = a.ok, a.failed, a.canceled, a.skipped
		l.running, l.spin, l.every = a.still, a.spin, a.every
		l.more, l.sep, l.path, l.divider = a.more, a.sep, a.path, a.divider
		l.guide, l.fill, l.track = a.guide, a.fill, a.track
	}
	switch t.colours {
	case Colours256:
		l.pal = &a.p256
	case Colours16:
		l.pal = &a.p16
	}
	return l
}

// painted returns s set in the attributes params, and set back after; s
// itself when params or s is "".
func painted(params, s string) string {
	if params == "" || s == "" {
		return s
	}
	return "\x1b[" + params + "m" + s + reset
}

// colour returns the parameters of r's colour, "" when the look has none.
func (l look) colour(r role) string {
	if l.pal == nil {
		return ""
	}
	return l.pal.roles[r]
}

// paint returns s in r's colour, set back after it, so that no colour runs
// on into what follows; s itself when the look has no colour for r.
func (l look) paint(r role, s string) string { return painted(l.colour(r), s) }

// mark returns the mark s in r's colour, bold as well where the palette
// draws its marks bold.
func (l look) mark(r role, s string) string {
	params := l.colour(r)
	if params != "" && l.pal.bold && params != "1" && !strings.HasPrefix(params, "1;") {
		params = "1;" + params
	}
	return painted(params, s)
}

// endGlyph returns the mark of what ended with status, uncoloured.
func (l look) endGlyph(status progress.Status) string {
	switch status {
	case progress.StatusFailed:
		return l.failed
	case progress.StatusCanceled:
		return l.canceled
	case progress.StatusSkipped:
		return l.skipped
	}
	return l.ok
}

// endMark returns the mark of what ended with status, in the colour of how
// it ended, as mark draws it.
func (l look) endMark(status progress.Status) string {
	return l.mark(statusRole(status), l.endGlyph(status))
}

// counts says how many of the targets c counts failed, were interrupted,
// were left out, run and wait their turn, "2 failed", each in the colour
// of how they stand, those queued muted, and leaves out each way that none
// stand.
func (l look) counts(c progress.Count) []string {
	var parts []string
	for _, n := range []struct {
		n    int
		what string
		r    role
	}{
		{c.Failed, "failed", roleFailed},
		{c.Canceled, "canceled", roleCanceled},
		{c.Skipped, "skipped", roleSkipped},
		{c.Running, "running", roleRunning},
		{c.Queued, "queued", roleMuted},
	} {
		if n.n > 0 {
			parts = append(parts, l.paint(n.r, fmt.Sprintf("%d %s", n.n, n.what)))
		}
	}
	return parts
}

// spinner returns the running mark at age since the display started, in
// the colour of what runs: a frame of the spinner, which turns once every
// len(spin) frames, or the mark that stands still.
func (l look) spinner(age time.Duration) string {
	frame := int(max(0, age)/l.every) % len(l.spin)
	return l.paint(roleRunning, l.spin[frame])
}

// indent returns the indent of a row at depth: two columns a level, the
// innermost the guide where the look has one.
func (l look) indent(depth int) string {
	if l.guide == "" || depth < 1 {
		return strings.Repeat("  ", depth)
	}
	return strings.Repeat("  ", depth-1) + l.paint(roleMuted, l.guide)
}

// bar returns the bar of a count, cells wide besides its caps: the share of
// the targets done, cut down to whole cells, those that failed at the end
// of it in the colour of a failure, at least one cell when any did. It is
// "" when the look draws no bar or the count knows no total.
func (l look) bar(c progress.Count, cells int) string {
	if l.fill == "" || c.Total <= 0 {
		return ""
	}
	done := min(max(c.Done, 0), c.Total)
	return l.cells(cells*done/c.Total, l.failedCells(c, cells), cells)
}

// fractionBar returns a bar, cells wide besides its caps, filled with the
// share f of some work done, cut down to whole cells and never full while f
// is short of 1, with the targets of c that failed at the end of it as bar
// draws them. It is "" when the look
// draws no bar.
func (l look) fractionBar(f float64, c progress.Count, cells int) string {
	if l.fill == "" {
		return ""
	}
	n := int(float64(cells)*f + 1e-9)
	if f < 1 {
		n = min(n, cells-1)
	}
	return l.cells(n, l.failedCells(c, cells), cells)
}

// failedCells returns how many cells of a bar cells wide the targets of c
// that failed take: their share, at least one when any did.
func (l look) failedCells(c progress.Count, cells int) int {
	failed := min(max(c.Failed, 0), max(c.Done, 0), c.Total)
	if failed <= 0 {
		return 0
	}
	return max(1, cells*failed/c.Total)
}

// cells draws a bar cells wide besides its caps with filled cells done, the
// last bad of them, at least that many, in the colour of a failure.
func (l look) cells(filled, bad, cells int) string {
	filled = max(filled, bad)
	// Cells of one colour side by side, the caps among them, are set in it
	// once.
	var b, run strings.Builder
	runParams := ""
	add := func(params, cell string) {
		if params != runParams {
			b.WriteString(painted(runParams, run.String()))
			run.Reset()
			runParams = params
		}
		run.WriteString(cell)
	}
	muted := l.colour(roleMuted)
	add(muted, l.open)
	for i := range cells {
		switch {
		case i < filled-bad:
			add(l.barColour(i, cells), l.fill)
		case i < filled:
			add(l.colour(roleFailed), l.fill)
		default:
			add(muted, l.track)
		}
	}
	add(muted, l.close)
	b.WriteString(painted(runParams, run.String()))
	return b.String()
}

// barColour returns the parameters of the colour of a bar's filled cell i
// of cells: its place in the gradient, or the bar's colour.
func (l look) barColour(i, cells int) string {
	if l.pal == nil || l.pal.gradient == nil {
		return l.colour(roleBar)
	}
	g := l.pal.gradient
	return g[i*len(g)/cells]
}
