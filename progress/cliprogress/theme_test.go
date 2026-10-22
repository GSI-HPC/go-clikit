// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress_test

import (
	"testing"

	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
	"github.com/GSI-HPC/go-clikit/progress/display"
)

func TestThemeFor(t *testing.T) {
	for _, tc := range []struct {
		name       string
		onTerminal bool
		env        map[string]string
		want       display.Theme
	}{
		{"no terminal", false, map[string]string{"TERM": "xterm-256color"}, display.Theme{}},
		{"a dumb terminal", true, map[string]string{"TERM": "dumb", "COLORTERM": "truecolor"}, display.Theme{}},
		{"NO_COLOR", true, map[string]string{"TERM": "xterm-256color", "NO_COLOR": "1"}, display.Aurora.In(display.NoColours)},
		{"COLORTERM", true, map[string]string{"TERM": "xterm", "COLORTERM": "truecolor"}, display.Aurora.In(display.Colours256)},
		{"TERM names 256 colours", true, map[string]string{"TERM": "screen-256color"}, display.Aurora.In(display.Colours256)},
		{"TERM alone", true, map[string]string{"TERM": "xterm"}, display.Aurora.In(display.Colours16)},
		{"no TERM", true, nil, display.Aurora.In(display.Colours16)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(name string) string { return tc.env[name] }
			if got := cliprogress.ThemeFor(display.Aurora, tc.onTerminal, getenv); got != tc.want {
				t.Errorf("ThemeFor = %v in %v, want %v in %v", got, got.Colours(), tc.want, tc.want.Colours())
			}
		})
	}
}
