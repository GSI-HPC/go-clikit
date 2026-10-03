// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
)

// printer is a sink that prints each event on a line of its own.
type printer struct{}

func (printer) Handle(e progress.Event) {
	fmt.Printf("%d %s %s %q %s", e.Seq, e.Type, e.Kind, e.Name, e.State)
	if e.Type == progress.TypeEnd {
		fmt.Printf(" %s", e.Status)
		if e.Err != "" {
			fmt.Printf(" (%s): %s", e.Class, e.Err)
		}
	}
	fmt.Println()
}

// reset is library code: it reports its work as spans in ctx, whether a
// command watches or not.
func reset(ctx context.Context, nodes []string) error {
	ctx, step := progress.Start(ctx, progress.KindStep, "reset the machines",
		progress.WithFlags(progress.Fold), progress.Total(len(nodes)))
	// Every target is announced queued before the first runs, so that a
	// display knows the whole of the work from the start.
	targets := make([]*progress.Span, len(nodes))
	for i, node := range nodes {
		_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(), progress.Node(node))
	}
	var err error
	for i, target := range targets {
		target.Run()
		if nodes[i] == "exe02" {
			failed := errors.New("exe02: no answer")
			target.End(failed)
			err = failed
			continue
		}
		target.End(nil)
	}
	step.End(err)
	return err
}

// A command attaches a Bus to its context and ends the spans it started;
// the library it calls reports its work under them.
func Example() {
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{printer{}}})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "power reset")
	command.End(reset(ctx, []string{"exe01", "exe02"}))
	bus.Close()
	// Output:
	// 1 start command "power reset" running
	// 2 start step "reset the machines" running
	// 3 start target "exe01" queued
	// 4 start target "exe02" queued
	// 5 run target "exe01" running
	// 6 end target "exe01" ended ok
	// 7 run target "exe02" running
	// 8 end target "exe02" ended failed (target): exe02: no answer
	// 9 end step "reset the machines" ended failed (target): exe02: no answer
	// 10 end command "power reset" ended failed (target): exe02: no answer
}

// Without a Bus, Start returns the context as it is and a nil span, whose
// methods do nothing: a library reports its work at no cost when no command
// is listening.
func ExampleStart_withoutABus() {
	ctx := context.Background()
	got, span := progress.Start(ctx, progress.KindStep, "reset the machines")
	span.Run()
	span.End(nil)
	fmt.Println(got == ctx, span == nil)
	// Output:
	// true true
}

// A Log writes the events as JSON lines. A clock, a trace and a run of the
// example's own keep them the same on every run, but for the span ids,
// which each Bus draws at random and the example writes as #1, #2 and so
// on.
func ExampleNewLog() {
	var buf bytes.Buffer
	log := progress.NewLog(&buf, progress.LogOptions{Program: "prog", Version: "v1.2.3", Run: "0123456789abcdef"})
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	bus := progress.NewBus(progress.BusOptions{
		Sinks: []progress.Sink{log},
		Now:   func() time.Time { return now },
		Trace: progress.TraceContext{Trace: progress.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}},
	})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "uptime")
	_, call := progress.Start(ctx, progress.KindCall, "ssh", progress.Host("exe01.example.org"), progress.Timeout(time.Minute))
	call.End(nil, progress.Exit(0))
	command.End(nil)
	bus.Close()
	if err := log.Close(); err != nil {
		fmt.Println(err)
	}
	ids := map[string]string{}
	fmt.Print(spanID.ReplaceAllStringFunc(buf.String(), func(field string) string {
		key, id, _ := strings.Cut(field, ":")
		if ids[id] == "" {
			ids[id] = fmt.Sprintf(`"#%d"`, len(ids)+1)
		}
		return key + ":" + ids[id]
	}))
	// Output:
	// {"v":1,"type":"trace","run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","program":"prog","version":"v1.2.3"}
	// {"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":1,"time":"2026-09-25T12:00:00.000000000Z","type":"start","span":"#1","kind":"command","name":"uptime","state":"running"}
	// {"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":2,"time":"2026-09-25T12:00:00.000000000Z","type":"start","span":"#2","parent":"#1","kind":"call","name":"ssh","state":"running","host":"exe01.example.org","timeout":60}
	// {"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":3,"time":"2026-09-25T12:00:00.000000000Z","type":"end","span":"#2","parent":"#1","kind":"call","name":"ssh","state":"ended","host":"exe01.example.org","timeout":60,"exit":0,"status":"ok"}
	// {"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":4,"time":"2026-09-25T12:00:00.000000000Z","type":"end","span":"#1","kind":"command","name":"uptime","state":"ended","status":"ok"}
}

// spanID matches the span ids of an event log.
var spanID = regexp.MustCompile(`"(span|parent)":"[0-9a-f]{16}"`)

// A command continues the trace of the program that started it, such as a
// CI job, from the TRACEPARENT and TRACESTATE it was given.
func ExampleParseTraceContext() {
	tc, ok := progress.ParseTraceContext("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "rojo=00f067aa0ba902b7")
	fmt.Println(ok, tc.Trace, tc.Parent, tc.Flags, tc.State)
	bus := progress.NewBus(progress.BusOptions{Trace: tc})
	defer bus.Close()

	_, ok = progress.ParseTraceContext("00-00000000000000000000000000000000-00f067aa0ba902b7-01", "")
	fmt.Println(ok)
	// Output:
	// true 4bf92f3577b34da6a3ce929d0e0e4736 00f067aa0ba902b7 1 rojo=00f067aa0ba902b7
	// false
}

// exitError is an error that asks a program to exit with a code.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// Classify tells why work failed; a program's own rule, here one for its
// exit codes, decides what the rules before it do not.
func ExampleClassify() {
	byCode := func(err error) progress.Class {
		if exit := (exitError{}); errors.As(err, &exit) && exit.code == 3 {
			return progress.ClassTransport
		}
		return progress.ClassTarget
	}
	for _, err := range []error{
		nil,
		context.Canceled,
		fmt.Errorf("exe01: %w", context.DeadlineExceeded),
		exitError{3},
		exitError{1},
		os.ErrPermission,
	} {
		fmt.Println(progress.Classify(err, byCode))
	}
	// Output:
	// none
	// canceled
	// timeout
	// transport
	// target
	// target
}

// Sanitize makes a line of output fit to show: a carriage return is
// applied as a terminal would, and what is left escaped and cut.
func ExampleSanitize() {
	fmt.Println(progress.Sanitize("copying  50%\rcopying 100%\x1b[K", 0))
	fmt.Println(progress.Sanitize("a line longer than it may be", 13))
	// Output:
	// copying 100%\x1b[K
	// a line longer
}
