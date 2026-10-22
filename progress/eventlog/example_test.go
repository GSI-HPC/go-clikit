// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog_test

import (
	"fmt"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/eventlog"
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
