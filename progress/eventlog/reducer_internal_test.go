// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog

import "testing"

// The spans of a run a Reducer keeps by default, which a test that starts
// as many would take seconds to show.
func TestReducerMaxSpansDefault(t *testing.T) {
	for _, o := range []Options{{}, {MaxSpans: -1}} {
		if got := NewReducer(o).maxSpans; got != 1<<17 {
			t.Errorf("%+v keeps %d spans of a run, want %d", o, got, 1<<17)
		}
	}
}
