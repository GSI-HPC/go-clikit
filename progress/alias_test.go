// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
	"io"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress"
)

// The old names of the constants stay for one minor release (decision 36),
// and name the same values, with the same words in the log.
func TestOldConstantNames(t *testing.T) {
	for _, tc := range []struct {
		name     string
		old, now any
	}{
		{"Hidden", progress.Hidden, progress.FlagHidden},
		{"Fold", progress.Fold, progress.FlagFold},
		{"ShowLines", progress.ShowLines, progress.FlagShowLines},
		{"DryRun", progress.DryRun, progress.FlagDryRun},
		{"Stdout", progress.Stdout, progress.StreamStdout},
		{"Stderr", progress.Stderr, progress.StreamStderr},
		{"Items", progress.Items, progress.UnitItems},
		{"Bytes", progress.Bytes, progress.UnitBytes},
		{"Percent", progress.Percent, progress.UnitPercent},
		{"Unbounded", progress.Unbounded, progress.BoundNone},
		{"BySize", progress.BySize, progress.BoundSize},
		{"ByTargets", progress.ByTargets, progress.BoundTargets},
		{"ByBelow", progress.ByBelow, progress.BoundChildren},
	} {
		if tc.old != tc.now {
			t.Errorf("%s = %v, want %v", tc.name, tc.old, tc.now)
		}
	}
}

// The old names of the options stay for one minor release (decision 36),
// and set what the new ones set.
func TestOldOptionNames(t *testing.T) {
	ctx, bus, capture := watched(t, progress.BusOptions{})
	_, call := progress.Start(ctx, progress.KindCall, "GET",
		progress.WithFlags(progress.FlagDryRun), progress.HTTP("GET", "/redfish/v1"))
	call.End(nil)
	bus.Close()
	e := capture.Events()[0]
	if e.Flags != progress.FlagDryRun {
		t.Errorf("WithFlags gave %v, want %v", e.Flags, progress.FlagDryRun)
	}
	if got, want := [2]string{e.Method, e.Path}, [2]string{"GET", "/redfish/v1"}; got != want {
		t.Errorf("HTTP gave %q, want %q", got, want)
	}
}

// The old names of RunInfo, RunSink and Bus.TraceID stay for one minor
// release (decision 36).
func TestOldRunNames(t *testing.T) {
	var log progress.TraceSink = progress.NewLog(io.Discard, progress.LogOptions{})
	log.Begin(progress.Run{Program: "prog"})
	bus := progress.NewBus(progress.BusOptions{})
	defer bus.Close()
	if bus.Trace() != bus.TraceID() || bus.TraceID() != bus.TraceContext().ID {
		t.Errorf("Trace = %v, TraceID = %v, want both %v", bus.Trace(), bus.TraceID(), bus.TraceContext().ID)
	}
}

// The old names of AdvanceWriter and AdvanceReader stay for one minor
// release (decision 36), and advance the span as the new ones do.
func TestOldAdvanceNames(t *testing.T) {
	ctx, bus, capture := watched(t, progress.BusOptions{})
	ctx, call := progress.Start(ctx, progress.KindCall, "copy", progress.Work(progress.UnitBytes, 8))
	var out strings.Builder
	if _, err := io.Copy(progress.CountWriter(ctx, &out), progress.CountReader(ctx, strings.NewReader("abcd"))); err != nil {
		t.Fatal(err)
	}
	call.End(nil)
	bus.Close()
	if end := capture.Events()[len(capture.Events())-1]; end.Amount != 8 || out.String() != "abcd" {
		t.Errorf("the copy ended with %d bytes and wrote %q, want 8 and abcd", end.Amount, out.String())
	}
}
