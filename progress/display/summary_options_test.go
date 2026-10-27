// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package display_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/display"
)

// summaryLineOf returns the line summary leaves for a command that fails
// one of three targets.
func summaryLineOf(t *testing.T, summary *display.Summary) string {
	t.Helper()
	c := &clock{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{summary}, Now: c.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "exec")
	step(ctx, "run", three, map[string]error{"exe2": errors.New("connection refused")})
	c.Add(1300 * time.Millisecond)
	command.End(errors.New("1 of 3 hosts failed: exe2"))
	bus.Close()
	return summary.Line()
}

// NewSummary fixes the theme and ASCII of a Summary when it is made
// (decision 46). NewSummary(SummaryOptions{}) draws as the zero Summary
// does, and so does ASCII with the zero Theme.
func TestNewSummaryDrawsInItsOptions(t *testing.T) {
	t.Parallel()
	plain := summaryLineOf(t, &display.Summary{})
	if want := "exec: failed in 1.3s: 2 ok, 1 failed"; plain != want {
		t.Fatalf("the zero Summary leaves %q, want %q", plain, want)
	}
	for _, o := range []display.SummaryOptions{
		{},
		{ASCII: true},
		{Theme: display.Ember},
		{Theme: display.Tide.In(display.Colors16), ASCII: true},
		{Theme: display.Neon.In(display.ColorsNone)},
	} {
		got := summaryLineOf(t, display.NewSummary(o))
		if themed := o.Theme != (display.Theme{}); themed == (got == plain) {
			t.Errorf("NewSummary(%v, ascii=%t) leaves %q; the zero Summary leaves %q", o.Theme, o.ASCII, got, plain)
		}
	}
}
