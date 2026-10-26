// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package docsite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// widths are the columns of the recorded terminal: plain lines are written
// for a log, which wraps nothing, so they get a wider one.
var widths = map[string]int{"tree": 72, "counter": 72, "plain": 100}

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// drawer is a display that the recording draws on its fake clock.
type drawer interface {
	progress.Sink
	Draw()
	Close()
}

// recording is the file the site's player reads: the frames of one
// display in one theme, each the screen after a draw, with the
// attributes of its text as progresstest.Screen.Styled shows them.
type recording struct {
	Display string  `json:"display"`
	Theme   string  `json:"theme"`
	Width   int     `json:"width"`
	Frames  []frame `json:"frames"`
}

type frame struct {
	// T is the time of the frame in seconds since the command started.
	T    float64 `json:"t"`
	Text string  `json:"text"`
}

// record runs the site's scenario, a command that fetches an image and
// then reinstalls twelve nodes four at a time, one of which fails, and
// keeps a frame after each step of the fake clock.
func record(t *testing.T, name string, theme display.Theme, newDisplay func(*display.Terminal, func() time.Time, display.Theme) drawer) recording {
	t.Helper()
	width := widths[name]
	clk := &clock{now: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}
	screen := &progresstest.Screen{Width: width, Styles: true}
	term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return width, 24, nil }})
	d := newDisplay(term, clk.Now, theme)
	summary := display.NewSummary(display.SummaryOptions{Theme: theme})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{d, summary}, Now: clk.Now})
	start := clk.Now()
	rec := recording{Display: name, Theme: theme.String(), Width: width}
	if rec.Theme == "" {
		rec.Theme = "none"
	}
	snap := func() {
		d.Draw()
		rec.Frames = append(rec.Frames, frame{T: clk.Now().Sub(start).Seconds(), Text: screen.Styled()})
	}
	tick := func(dt time.Duration) {
		clk.Add(dt)
		snap()
	}

	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "reinstall")

	fctx, fetch := progress.Start(ctx, progress.KindStep, "fetching the base image", progress.Work(progress.UnitBytes, 2<<30))
	_, get := progress.Start(fctx, progress.KindCall, "http", progress.HTTP("GET", "/images/rocky-9.4.qcow2"))
	for range 10 {
		fetch.Advance(int64(2<<30) / 10)
		tick(500 * time.Millisecond)
	}
	get.End(nil)
	fetch.End(nil)
	tick(300 * time.Millisecond)

	const nodes = 12
	sctx, step := progress.Start(ctx, progress.KindStep, "reinstalling the nodes",
		progress.Flagged(progress.FlagFold), progress.Total(nodes), progress.Limit(4))
	ctxs := make([]context.Context, nodes)
	targets := make([]*progress.Span, nodes)
	for i := range nodes {
		node := fmt.Sprintf("exe%02d", i+1)
		ctxs[i], targets[i] = progress.Start(sctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	tick(200 * time.Millisecond)
	calls := make([]*progress.Span, nodes)
	var running []int
	next := 0
	run := func() {
		i := next
		next++
		targets[i].Run()
		_, calls[i] = progress.Start(ctxs[i], progress.KindCall, "redfish",
			progress.HTTP("POST", "/redfish/v1/Systems/1"), progress.Timeout(30*time.Second))
		running = append(running, i)
	}
	for range 4 {
		run()
	}
	tick(400 * time.Millisecond)
	failed := errors.New("BMC answered 503")
	for len(running) > 0 {
		tick(400 * time.Millisecond)
		i := running[0]
		running = running[1:]
		calls[i].End(nil)
		var err error
		if i == 6 {
			err = failed
		}
		targets[i].End(err)
		if next < nodes {
			run()
		}
		tick(300 * time.Millisecond)
	}
	err := fmt.Errorf("1 of %d nodes failed: %w", nodes, failed)
	step.End(err)
	command.End(err)
	clk.Add(200 * time.Millisecond)
	bus.Close()
	d.Close()
	_, _ = fmt.Fprintln(screen, summary.Line())
	rec.Frames = append(rec.Frames, frame{T: clk.Now().Sub(start).Seconds(), Text: screen.Styled()})
	return rec
}

// TestRecordings holds the recordings the site plays to what the displays
// draw now: the tree in every theme, the counter and plain lines in the
// zero Theme and in Tide.
func TestRecordings(t *testing.T) {
	displays := map[string]func(*display.Terminal, func() time.Time, display.Theme) drawer{
		"tree": func(term *display.Terminal, now func() time.Time, th display.Theme) drawer {
			return display.NewTree(term, display.TreeOptions{Now: now, Theme: th})
		},
		"counter": func(term *display.Terminal, now func() time.Time, th display.Theme) drawer {
			return display.NewCounter(term, display.CounterOptions{Now: now, Theme: th})
		},
		"plain": func(term *display.Terminal, now func() time.Time, th display.Theme) drawer {
			return display.NewPlain(term, display.PlainOptions{Now: now, Theme: th})
		},
	}
	tide, err := display.ParseTheme("tide")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		display string
		themes  []display.Theme
	}{
		{"tree", append([]display.Theme{{}}, display.Themes()...)},
		{"counter", []display.Theme{{}, tide}},
		{"plain", []display.Theme{{}, tide}},
	}
	for _, c := range cases {
		for _, th := range c.themes {
			th = th.In(display.Colors256)
			rec := record(t, c.display, th, displays[c.display])
			b, err := json.MarshalIndent(rec, "", "\t")
			if err != nil {
				t.Fatal(err)
			}
			golden(t, "static/casts/"+c.display+"-"+rec.Theme+".json", append(b, '\n'))
		}
	}
}
