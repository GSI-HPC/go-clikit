// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
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
