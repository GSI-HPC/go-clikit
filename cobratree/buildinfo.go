// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree

import (
	"runtime/debug"
	"strings"
)

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

// Resolve returns b with the version and the commit a build set with
// -ldflags in place of the toolchain's: a release build that sets them
// wins. An empty value sets nothing, so a program declares the variables it
// sets that way empty, rather than holding a word such as "dev", and words
// a build that is no release itself, where Version is empty:
//
//	bi := cobratree.ReadBuildInfo().Resolve(version, commit)
//
// A commit set that is not the one b records, nor the start of it, is
// another commit than the checkout's, and the time and the changes
// recorded are then left out, as they are not that commit's. A test
// applies these rules to a BuildInfo it builds itself.
func (b BuildInfo) Resolve(version, commit string) BuildInfo {
	if version != "" {
		b.Version = version
	}
	if commit != "" {
		if !strings.HasPrefix(b.Commit, commit) {
			b.Time, b.Dirty = "", false
		}
		b.Commit = commit
	}
	return b
}

// Short returns the commit cut to its first n characters, as a version line
// prints it, with "-dirty" after it for a checkout that held changes not
// committed: "ebed487-dirty" for 7. It is "" without a commit, and n of 0
// or less, or more than the commit has, keeps it whole.
func (b BuildInfo) Short(n int) string {
	if b.Commit == "" {
		return ""
	}
	rev := b.Commit
	if n > 0 && n < len(rev) {
		rev = rev[:n]
	}
	if b.Dirty {
		rev += "-dirty"
	}
	return rev
}
