// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/eventlog"
	"github.com/GSI-HPC/go-clikit/progress/mcpprogress"
)

// A reader decodes each line of a log, and skips those it cannot read.
func ExampleDecode() {
	for _, s := range []string{
		`{"v":1,"type":"trace","run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","program":"prog","version":"v1.2.3"}`,
		`{"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":1,"time":"2026-09-25T12:00:00.000000000Z","type":"start","span":"b7e3a1c09d2f4e10","kind":"command","name":"exec","state":"running"}`,
		`{"v":2,"run":"0123456789abcdef","type":"start"}`,
		`{"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":2,"time":"2026-09-25T12:00:03.000000000Z","type":"end","span":"b7e3a1c09d2f4e10","kind":"command","name":"exec","state":"ended","status":"failed","class":"transport","err":"no route to host"}`,
	} {
		l, err := eventlog.Decode([]byte(s))
		switch {
		case err != nil:
			fmt.Println("skipped:", err)
		case l.Begin:
			fmt.Println(l.Run, l.Program, l.Version)
		case l.Event.Type == progress.TypeEnd:
			e := l.Event
			fmt.Println(e.Seq, e.Type, e.Kind, e.Name, e.Status, e.Class, e.Err)
		default:
			e := l.Event
			fmt.Println(e.Seq, e.Type, e.Kind, e.Name, e.State)
		}
	}
	// Output:
	// 0123456789abcdef prog v1.2.3
	// 1 start command exec running
	// skipped: a line of version 2 of the event log, not 1
	// 2 end command exec failed transport no route to host
}

// A server runs a tool call's command as a child, with the child's event
// log in a file of its own, and sends on the child's progress as it
// polls the log, to an mcpprogress.Notifier. Here the child's Bus runs in
// the example, and the server polls once, after it has ended; a server
// polls every so often while the child runs, and once more after it.
func ExampleForward() {
	dir, err := os.MkdirTemp("", "child")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "progress.jsonl")

	// The child, run with its event log in path.
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	log := progress.NewLog(f, progress.LogOptions{})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{log}, Program: "prog"})
	fanout.Map(progress.WithBus(context.Background(), bus), []string{"exe1", "exe2", "exe3"},
		fanout.MapOptions[string]{Step: "probe the nodes"},
		func(context.Context, string) (struct{}, error) { return struct{}{}, nil })
	bus.Close()
	if err := log.Close(); err != nil {
		panic(err)
	}
	_ = f.Close()

	// The server; Send would send the notification on the call's
	// session, and here keeps the last.
	var last string
	n := mcpprogress.New(mcpprogress.Options{
		Program: "prog",
		Send: func(progress, total float64, message string) error {
			last = fmt.Sprintf("%v/%v %s", progress, total, message)
			return nil
		},
	})
	tail := eventlog.NewTail(path, eventlog.Forward(n))
	if _, err := tail.Poll(); err != nil {
		panic(err)
	}
	n.Close()
	fmt.Println(last)
	// Output:
	// 3/3 probe the nodes: 3/3 done
}
