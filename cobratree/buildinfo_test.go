// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree

import (
	"runtime/debug"
	"testing"
)

func TestFromBuildInfo(t *testing.T) {
	const rev = "ebed4872343d0123456789abcdef0123456789ab"
	vcs := func(modified string) []debug.BuildSetting {
		return []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: rev},
			{Key: "vcs.time", Value: "2026-09-23T18:09:18Z"},
			{Key: "vcs.modified", Value: modified},
		}
	}
	for _, tc := range []struct {
		name string
		bi   *debug.BuildInfo
		want BuildInfo
	}{
		{"no build info", nil, BuildInfo{}},
		{"no module version", &debug.BuildInfo{}, BuildInfo{}},
		{"devel", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, BuildInfo{}},
		{
			// go install …@v0.9.0 builds the module from the proxy, which
			// carries no VCS stamps; the checksum database vouches for it.
			"go install of a release",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.9.0"}},
			BuildInfo{Version: "v0.9.0"},
		},
		{
			"go install of a release, with build settings",
			&debug.BuildInfo{
				Main:     debug.Module{Version: "v0.9.0"},
				Settings: []debug.BuildSetting{{Key: "CGO_ENABLED", Value: "0"}},
			},
			BuildInfo{Version: "v0.9.0"},
		},
		{
			// Go 1.24 and later stamp a pseudo-version from a checkout,
			// which names no release.
			"checkout",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260923180918-ebed4872343d"}, Settings: vcs("false")},
			BuildInfo{Commit: rev, Time: "2026-09-23T18:09:18Z"},
		},
		{
			// A tag in a local checkout is no release.
			"checkout of a tag",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.9.0"}, Settings: vcs("false")},
			BuildInfo{Commit: rev, Time: "2026-09-23T18:09:18Z"},
		},
		{
			"modified checkout",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260923180918-ebed4872343d+dirty"}, Settings: vcs("true")},
			BuildInfo{Commit: rev, Time: "2026-09-23T18:09:18Z", Dirty: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := fromBuildInfo(tc.bi); got != tc.want {
				t.Errorf("fromBuildInfo = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A test binary is no release: go test stamps neither a module version nor
// the checkout.
func TestReadBuildInfoOfATestBinary(t *testing.T) {
	if got := ReadBuildInfo(); got.Version != "" {
		t.Errorf("Version %q, want none", got.Version)
	}
}
