// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// The work of a span, as progress.Meter reads it, is drawn in the forms
// below, which the text drawn with the zero Theme fixes (decision 5).

const (
	// workBar is how many cells wide the bar of a running target, call or
	// step with a bounded share is, in a theme that draws bars.
	workBar = 6
	// giveUps is how many parts of its work a row that does not fit gives
	// up, one at a time: the time left, the rate, the amount and the bar.
	giveUps = 4
)

// scale is how the amounts of a unit are drawn: in its base unit, whole,
// below whole, and otherwise in the largest unit, a power of step, in which
// they read below 1,000.
type scale struct {
	step  float64
	units []string
	// space goes between a number and its unit.
	space string
	whole float64
}

var (
	// byteScale draws bytes in IEC units: 512 B, 85.3 MiB, 312 MiB.
	byteScale = scale{step: 1024, units: []string{"B", "KiB", "MiB", "GiB", "TiB"}, space: " ", whole: 1000}
	// itemScale draws items whole below 10,000, then with SI prefixes:
	// 9312, 48.2k, 1.2M.
	itemScale = scale{step: 1000, units: []string{"", "k", "M", "G", "T"}, whole: 10000}
)

// scaleOf returns how the amounts of u are drawn: bytes as bytes, anything
// else as items.
func scaleOf(u progress.Unit) scale {
	if u == progress.Bytes {
		return byteScale
	}
	return itemScale
}

// unitOf returns the power of s.step that v is drawn in: 0 below s.whole,
// otherwise the first in which v reads below 1,000 once rounded, or the
// largest.
func (s scale) unitOf(v float64) int {
	if v < s.whole {
		return 0
	}
	k := 1
	for k < len(s.units)-1 && v/math.Pow(s.step, float64(k)) >= 999.5 {
		k++
	}
	return k
}

// number draws v in the power k of s.step, without the unit: whole in the
// base unit, and otherwise with one decimal below 100 and none from 100,
// rounded.
func (s scale) number(v float64, k int) string {
	x := v / math.Pow(s.step, float64(k))
	if k > 0 && x < 99.95 {
		return fmt.Sprintf("%.1f", x)
	}
	return fmt.Sprintf("%.0f", x)
}

// suffix is the unit k after a number, with the space before it.
func (s scale) suffix(k int) string {
	if s.units[k] == "" {
		return ""
	}
	return s.space + s.units[k]
}

// amountText reads an amount of work in u: "512 B", "85.3 MiB", "312 MiB",
// "9312", "48.2k".
func amountText(u progress.Unit, v float64) string {
	s := scaleOf(u)
	k := s.unitOf(v)
	return s.number(v, k) + s.suffix(k)
}

// outOfText reads an amount out of a size, both in the size's unit:
// "0.5/2.0 GiB", "48.2k/50.0k", "312/480".
func outOfText(u progress.Unit, amount, size int64) string {
	s := scaleOf(u)
	k := s.unitOf(float64(size))
	return s.number(float64(amount), k) + "/" + s.number(float64(size), k) + s.suffix(k)
}

// rateText reads a rate in the form of an amount with /s, with one decimal
// below 10: "85.3 MiB/s", "6.6k/s", "0.4/s". A rate takes a larger unit once
// it would read 1,000 in its base unit, items as bytes do, since the digits
// of a rate say little: 6.6k/s, not 6600/s, and 1.0k/s, not 1000/s.
func rateText(u progress.Unit, v float64) string {
	s := scaleOf(u)
	if v < 9.95 {
		return fmt.Sprintf("%.1f", v) + s.suffix(0) + "/s"
	}
	s.whole = 999.5
	k := s.unitOf(v)
	return s.number(v, k) + s.suffix(k) + "/s"
}

// shareText reads a share done as a whole percentage, cut, not rounded, so
// that 100% means done: "65%". What float arithmetic leaves short of a
// whole percentage, as 0.57 is of 57, is not cut, but a share short of 1
// never reads 100%.
func shareText(f float64) string {
	p := int(f*100 + 1e-9)
	if f < 1 {
		p = min(p, 99)
	}
	return fmt.Sprintf("%d%%", p)
}

// leftText reads the time left of some work, rounded up to the unit it
// draws, the second below an hour and the minute from it, with a ~ in
// front so that it is not read as the countdown of a pause: "~12s left",
// "~7m03s left", "~1h02m left".
func leftText(d time.Duration) string {
	d = (d + time.Second - 1).Truncate(time.Second)
	if d >= time.Hour {
		d = (d + time.Minute - 1).Truncate(time.Minute)
	}
	return "~" + seconds(d) + " left"
}

// stalledText says how long work has not grown, as a live clock: "stalled
// 6.1s".
func stalledText(d time.Duration) string { return "stalled " + runTime(d) }

// work is what a row draws of the work of a span, each part in its colour,
// "" for a part it does not draw.
type work struct {
	// bar is the bar of a bounded share; amount the amount, out of the
	// size when there is one; share the percentage done; pace the rate,
	// or how long the work has stalled; left the time left.
	bar, amount, share, pace, left string
}

// workOf returns the parts of r as the tree draws them in the look g, with
// a bar of cells, when cells is not 0, r is bounded and g draws bars. Work
// in Percent draws its share alone, with no amount and no rate; work that
// is unbounded draws no share and no time left. The amount and the rate
// are muted, the share in the colour of the bar, and stalled in that of a
// cancellation.
func workOf(g look, r progress.Reading, cells int) work {
	var w work
	bounded := r.Bound != progress.Unbounded
	if bounded {
		w.share = g.paint(roleBar, shareText(r.Fraction))
		if cells > 0 {
			w.bar = g.fractionBar(r.Fraction, progress.Count{}, cells)
		}
	}
	if r.Left > 0 {
		w.left = g.paint(roleMuted, leftText(r.Left))
	}
	if r.Unit == progress.Percent {
		return w
	}
	if r.Size > 0 {
		w.amount = g.paint(roleMuted, outOfText(r.Unit, r.Amount, r.Size))
	} else {
		w.amount = g.paint(roleMuted, amountText(r.Unit, float64(r.Amount)))
	}
	if r.Stalled > 0 {
		w.pace = g.paint(roleCanceled, stalledText(r.Stalled))
	} else {
		w.pace = g.paint(roleMuted, rateText(r.Unit, r.Rate))
	}
	return w
}

// text joins the parts of w with sep, the bar first, split by a space,
// once the row has given up giveUp of them: 1 gives up the time left, 2
// the rate as well, 3 the amount and 4 the bar.
func (w work) text(sep string, giveUp int) string {
	var parts []string
	for _, part := range []struct {
		text string
		// kept is how many parts the row may give up and keep this one.
		kept int
	}{{w.amount, 2}, {w.share, giveUps}, {w.pace, 1}, {w.left, 0}} {
		if part.text != "" && giveUp <= part.kept {
			parts = append(parts, part.text)
		}
	}
	text := strings.Join(parts, sep)
	if w.bar != "" && giveUp < 4 {
		text = w.bar + " " + text
	}
	return text
}

// endedText says what the work of a span that ended came to, from what
// Meter.Add returned at its end: the amount and the mean rate, "2.0 GiB at
// 80.0 MiB/s", both muted; the amount alone when the span took no time,
// and "" for work in Percent, which has no amount to say.
func endedText(g look, r progress.Reading) string {
	if r.Unit == progress.Percent {
		return ""
	}
	text := g.paint(roleMuted, amountText(r.Unit, float64(r.Amount)))
	if r.Rate > 0 {
		text += " at " + g.paint(roleMuted, rateText(r.Unit, r.Rate))
	}
	return text
}
