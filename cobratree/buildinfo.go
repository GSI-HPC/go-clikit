// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree

import "runtime/debug"

// BuildInfo is what the Go toolchain recorded in the running binary about
// the source it was built from. A field it recorded nothing for is empty.
//
// It holds the data and no words: a program that injects its version and
// commit at build time with -ldflags falls back to these for a build
// without them, names a build that is no release as it likes, such as
// "dev", and prints as much of the commit as it likes.
type BuildInfo struct {
	// Version is the module version of a binary built from the module
	// proxy, as "go install example.com/prog@v1.2.3" builds it, whose
	// version the checksum database vouches for. It is empty for any other
	// build: one from a checkout carries the commit it was built from, and
	// the version the toolchain derives there, from a tag the commit
	// carries or as a pseudo-version, names no release.
	Version string
	// Commit is the full revision the binary was built from, vcs.revision,
	// for a build from a checkout.
	Commit string
	// Time is when that commit was made, vcs.time, in RFC 3339 form.
	Time string
	// Dirty reports whether the checkout held changes not committed,
	// vcs.modified.
	Dirty bool
}

// ReadBuildInfo returns what the toolchain recorded in the running binary,
// as debug.ReadBuildInfo reads it. A binary built without module support
// records nothing, and ReadBuildInfo returns the zero BuildInfo for it.
func ReadBuildInfo() BuildInfo {
	bi, _ := debug.ReadBuildInfo()
	return fromBuildInfo(bi)
}

// fromBuildInfo returns what bi records; bi may be nil.
func fromBuildInfo(bi *debug.BuildInfo) BuildInfo {
	var info BuildInfo
	if bi == nil {
		return info
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Commit = s.Value
		case "vcs.time":
			info.Time = s.Value
		case "vcs.modified":
			info.Dirty = s.Value == "true"
		}
	}
	if info.Commit == "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	return info
}
