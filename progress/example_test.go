// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
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

// Work left out on purpose returns an error of Skip, which a span ends
// skipped rather than failed, whatever the program's rule for classes
// says, and which errors.Is tells from a failure.
func ExampleSkip() {
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{printer{}}})
	ctx := progress.WithBus(context.Background(), bus)
	_, target := progress.Start(ctx, progress.KindTarget, "exe01")
	err := fmt.Errorf("exe01: %w", progress.Skip("in maintenance"))
	target.End(err)
	bus.Close()
	fmt.Println(errors.Is(err, progress.ErrSkipped))
	// Output:
	// 1 start target "exe01" running
	// 2 end target "exe01" ended skipped (none): exe01: in maintenance
	// true
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

// A library that runs work of its own learns from the Bus in its context
// what the program said of itself: its name, where panics go, and its rule
// for classes. Without a Bus the answers are the defaults.
func ExampleBus_Classify() {
	byCode := func(err error) progress.Class {
		if exit := (exitError{}); errors.As(err, &exit) && exit.code == 3 {
			return progress.ClassTransport
		}
		return progress.ClassTarget
	}
	bus := progress.NewBus(progress.BusOptions{Program: "sind", Classify: byCode})
	defer bus.Close()
	for _, ctx := range []context.Context{
		progress.WithBus(context.Background(), bus),
		context.Background(),
	} {
		b := progress.BusFrom(ctx)
		fmt.Printf("%q %s\n", b.Program(), b.Classify(exitError{3}))
	}
	// Output:
	// "sind" transport
	// "" target
}

// worker is a sink that prints the work each event of a span carries.
type worker struct{}

func (worker) Handle(e progress.Event) {
	if e.Kind == progress.KindCall {
		fmt.Printf("%s %q %d/%d %s\n", e.Type, e.Name, e.Amount, e.Size, e.Unit)
	}
}

// A span reports its work: Work gives its unit and its size, here once the
// size is known, and Advance adds to the amount done. The Bus sends the
// amount at most once each 100 ms by its clock, and the End carries the
// newest, so the advance made 50 ms after the one before reaches the sinks
// with the End.
func ExampleSpan_Advance() {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	bus := progress.NewBus(progress.BusOptions{
		Sinks: []progress.Sink{worker{}},
		Now:   func() time.Time { return now },
	})
	ctx := progress.WithBus(context.Background(), bus)
	_, call := progress.Start(ctx, progress.KindCall, "download", progress.HTTP("GET", "/images/rocky-9.4.qcow2"))
	call.Update(progress.Work(progress.Bytes, 2<<30))
	for range 3 {
		now = now.Add(100 * time.Millisecond)
		call.Advance(8 << 20)
	}
	now = now.Add(50 * time.Millisecond)
	call.Advance(8 << 20)
	call.End(nil)
	bus.Close()
	// Output:
	// start "download" 0/0 unit(0)
	// update "download" 0/2147483648 bytes
	// advance "download" 8388608/2147483648 bytes
	// advance "download" 16777216/2147483648 bytes
	// advance "download" 25165824/2147483648 bytes
	// end "download" 33554432/2147483648 bytes
}

// Work that says only how far it has got, such as a remote task, is in
// Percent, out of 100, and SetAmount sets how far it is.
func ExampleSpan_SetAmount() {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	bus := progress.NewBus(progress.BusOptions{
		Sinks: []progress.Sink{worker{}},
		Now:   func() time.Time { return now },
	})
	ctx := progress.WithBus(context.Background(), bus)
	_, call := progress.Start(ctx, progress.KindCall, "firmware update", progress.Work(progress.Percent, 0))
	for _, done := range []int64{40, 75, 100} {
		now = now.Add(time.Second)
		call.SetAmount(done)
	}
	call.End(nil)
	bus.Close()
	// Output:
	// start "firmware update" 0/100 percent
	// advance "firmware update" 40/100 percent
	// advance "firmware update" 75/100 percent
	// advance "firmware update" 100/100 percent
	// end "firmware update" 100/100 percent
}

// ExampleCountWriter copies an image through CountWriter under a call that
// gives its size. The Bus sends the amount when its place comes, and the
// call's End carries the newest.
func ExampleCountWriter() {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{
		Sinks: []progress.Sink{capture},
		Now:   func() time.Time { return now },
	})
	ctx, step := progress.Start(progress.WithBus(context.Background(), bus), progress.KindStep, "copying the image")
	ctx, call := progress.Start(ctx, progress.KindCall, "copy", progress.Work(progress.Bytes, 4<<20))
	image := bytes.NewReader(make([]byte, 4<<20))
	var dst bytes.Buffer
	n, err := io.Copy(progress.CountWriter(ctx, &dst), image)
	call.End(err)
	step.End(err)
	bus.Close()
	for _, e := range capture.Events() {
		if e.Kind == progress.KindCall {
			fmt.Printf("%s %s %d/%d %s\n", e.Type, e.Name, e.Amount, e.Size, e.Unit)
		}
	}
	fmt.Println("copied", n, "bytes")
	// Output:
	// start copy 0/4194304 bytes
	// advance copy 4194304/4194304 bytes
	// end copy 4194304/4194304 bytes
	// copied 4194304 bytes
}

// A Meter rolls the work of a Bus's spans up the tree. A step that counts
// its targets is as far as the targets that ended, and the share done of
// each one running: here two of four ended and one is half done.
func ExampleMeter() {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{
		Sinks: []progress.Sink{capture},
		Now:   func() time.Time { return now },
	})
	ctx, _ := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "deploy")

	ctx, _ = progress.Start(ctx, progress.KindStep, "copying the image",
		progress.WithFlags(progress.Fold), progress.Total(4))
	targets := make([]*progress.Span, 4)
	for i, node := range []string{"exe01", "exe02", "exe03", "exe04"} {
		_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(),
			progress.Node(node), progress.Work(progress.Bytes, 1<<30))
	}
	for _, target := range targets[:2] {
		target.Run()
		target.Advance(1 << 30)
		target.End(nil)
	}
	targets[2].Run()
	now = now.Add(5 * time.Second)
	targets[2].Advance(1 << 29)

	var meter progress.Meter
	var counted progress.SpanID
	for _, e := range capture.Events() {
		meter.Add(e)
		if e.Kind == progress.KindStep && e.Type == progress.TypeStart {
			counted = e.Span
		}
	}
	r, _ := meter.Read(counted, now)
	fmt.Println(r.Bound, r.Fraction, r.Amount, r.Size)
	bus.Close()
	// Output:
	// targets 0.625 2684354560 0
}
