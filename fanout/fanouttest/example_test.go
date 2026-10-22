// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanouttest_test

import (
	"context"
	"fmt"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/fanout/fanouttest"
)

// uptime is a fake of whatever a program asks a host: it answers at once,
// and counts each request for as long as it is under way, in the one line
// of Enter.
type uptime struct {
	calls fanouttest.InFlight
}

func (u *uptime) Ask(host string) string {
	defer u.calls.Enter()()
	return host + ": up"
}

// A fake that holds each call until three are under way together proves
// that a fan-out with a limit of three runs three at once: no call goes on
// before the third has joined, so the peak is three exactly, and every call
// was counted.
func ExampleInFlight() {
	fake := &uptime{}
	fake.calls.Hold = 3
	fake.calls.Patience = time.Minute

	fanout.Each(context.Background(), 3, 3, func(i int) {
		fake.Ask(fmt.Sprintf("exe%02d", i+1))
	})
	fmt.Println(fake.calls.Peak(), fake.calls.Started())
	// Output:
	// 3 3
}

// A fan-out that keeps to a limit of one never has a second call to join
// the first, so each call waits for its patience to run out, and the peak
// shows the limit was kept. Had the fan-out started a second call at once,
// the peak would have been two.
func ExampleInFlight_patience() {
	fake := &uptime{}
	fake.calls.Hold = 2
	fake.calls.Patience = time.Millisecond

	fanout.Each(context.Background(), 3, 1, func(i int) {
		fake.Ask(fmt.Sprintf("exe%02d", i+1))
	})
	fmt.Println(fake.calls.Peak(), fake.calls.Started())
	// Output:
	// 1 3
}
