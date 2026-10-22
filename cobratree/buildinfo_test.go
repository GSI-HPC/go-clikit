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

func TestShort(t *testing.T) {
	const rev = "ebed4872343d0123456789abcdef0123456789ab"
	for _, tc := range []struct {
		bi   BuildInfo
		n    int
		want string
	}{
		{BuildInfo{}, 7, ""},
		{BuildInfo{Dirty: true}, 7, ""},
		{BuildInfo{Commit: rev}, 7, "ebed487"},
		{BuildInfo{Commit: rev, Dirty: true}, 12, "ebed4872343d-dirty"},
		{BuildInfo{Commit: rev}, 0, rev},
		{BuildInfo{Commit: rev}, -1, rev},
		{BuildInfo{Commit: rev}, 40, rev},
		{BuildInfo{Commit: "ebed487", Dirty: true}, 12, "ebed487-dirty"},
	} {
		if got := tc.bi.Short(tc.n); got != tc.want {
			t.Errorf("%+v.Short(%d) = %q, want %q", tc.bi, tc.n, got, tc.want)
		}
	}
}

func TestResolve(t *testing.T) {
	const rev = "ebed4872343d0123456789abcdef0123456789ab"
	checkout := BuildInfo{Commit: rev, Time: "2026-09-23T18:09:18Z", Dirty: true}
	for _, tc := range []struct {
		name            string
		bi              BuildInfo
		version, commit string
		want            BuildInfo
	}{
		{"nothing set", checkout, "", "", checkout},
		{"version set", checkout, "v1.2.3", "", BuildInfo{Version: "v1.2.3", Commit: rev, Time: "2026-09-23T18:09:18Z", Dirty: true}},
		{"the same commit", checkout, "v1.2.3", rev, BuildInfo{Version: "v1.2.3", Commit: rev, Time: "2026-09-23T18:09:18Z", Dirty: true}},
		{"the start of the commit", checkout, "", "ebed487", BuildInfo{Commit: "ebed487", Time: "2026-09-23T18:09:18Z", Dirty: true}},
		{"another commit", checkout, "", "0123456", BuildInfo{Commit: "0123456"}},
		{"no checkout", BuildInfo{Version: "v0.9.0"}, "v1.2.3", rev, BuildInfo{Version: "v1.2.3", Commit: rev}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.bi.resolve(tc.version, tc.commit); got != tc.want {
				t.Errorf("resolve(%q, %q) = %+v, want %+v", tc.version, tc.commit, got, tc.want)
			}
		})
	}
	if got := Resolve("v1.2.3", "ebed487"); got.Version != "v1.2.3" || got.Commit != "ebed487" {
		t.Errorf("Resolve = %+v, want what was set", got)
	}
}
