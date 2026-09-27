<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Decisions

One section per decision, in the order they were taken. Each says what the
situation was, what was decided, and what that costs. A decision is never
edited: a later one supersedes it, and the earlier one's status names it.

| | Decision | Status |
| --- | --- | --- |
| [1](#1-apache-20-and-gsi-holds-the-copyright) | Apache-2.0, and GSI holds the copyright | accepted |
| [2](#2-the-go-line-is-the-oldest-go-release-still-supported) | The go line is the oldest Go release still supported | accepted |
| [3](#3-what-the-kit-may-require) | What the kit may require | accepted |
| [4](#4-a-release-is-a-signed-tag) | A release is a signed tag | accepted |

## 1. Apache-2.0, and GSI holds the copyright

Status: accepted

### Context

The kit's packages were written for [clusterctl](https://github.com/GSI-HPC/clusterctl),
which GSI licenses under LGPL-3.0-or-later as its copyright holder. As a
module of their own they are meant for any command-line program, and for
library packages that report progress through them, whose importers then
take the kit too. For all of them the Lesser GPL is a burden in Go, where
every program links its packages statically: a proprietary program that
imports the kit has to let its users relink it (section 4 of the LGPL).
Libraries in the Go ecosystem are permissive; the Go project's own
modules, `golang.org/x/...`, are BSD-3-Clause.

### Decision

The module is licensed under the Apache License, Version 2.0, and GSI
Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> holds the
copyright.

- `LICENSE` holds the licence text verbatim, so that GitHub and pkg.go.dev
  recognise it. pkg.go.dev shows the documentation only of a module whose
  licence it recognises as redistributable.
- `LICENSES/Apache-2.0.txt` holds it again for the REUSE specification.
- Every file carries `SPDX-FileCopyrightText` and `SPDX-License-Identifier`
  in its own comment style, and `reuse lint` checks this in CI.
- The year is 2026, when the work was first written, and it is not moved
  forward.
- There is no `NOTICE` file.

### Why Apache-2.0

- It grants a licence to every patent a contributor holds that the
  contribution uses, and ends that licence for anyone who sues over one.
  BSD-3-Clause, Go's own licence, says nothing about patents; the Go project
  adds a grant of Google's in a separate `PATENTS` file. MIT is silent
  too.
- It is compatible with LGPL-3.0 and GPL-3.0, so programs under either
  licence can import the module.

### Moving code in

GSI holds the copyright of the code that moves here, so it can license this
copy under Apache-2.0. The commit that moves a file changes its
`SPDX-License-Identifier` line. The code keeps its LGPL-3.0-or-later
licence in the history of the repository it comes from.

### Costs

- Apache-2.0 is not compatible with GPL-2.0-only, so a program under that
  licence cannot import the module.
- A redistributor keeps the copyright and licence notices of every file.
- A contributor from outside GSI keeps the copyright of their contribution
  and licenses it under Apache-2.0 (section 5); a file they change then
  names them in an `SPDX-FileCopyrightText` line of their own.

## 2. The go line is the oldest Go release still supported

Status: accepted

### Context

Since Go 1.21 the `go` line in `go.mod` is a requirement, not a hint.
An older toolchain refuses the module, or downloads a newer one, and every
module that requires this one inherits at least this `go` line when it
runs `go mod tidy`. The line is therefore the oldest Go that every
importer has to use.

The Go project ships a release in February and in August. It supports the
two newest, and only those receive security fixes. In September 2026 those
are Go 1.27 and Go 1.26.

The kit's code needs Go 1.25, the first release with `sync.WaitGroup.Go`,
which `fanout` uses. Go 1.25 has had no security fixes since August 2026,
and at 1.25 the kit would have to hold `golang.org/x/text` at v0.41.0,
since v0.42.0 requires Go 1.26.

### Decision

- `go.mod` names the oldest Go release that the Go project still supports,
  as the `golang.org/x` modules do: `go 1.26.0` today. It names a `.0`
  release, never a patch release, and `go.mod` has no `toolchain` line.
- When a new Go release ships, the line moves up to the release before it,
  in a commit of its own, and the next release of the module is a minor
  version. Raising it further takes a record of its own.
- `mise.toml` names the newest Go release line. Contributors and CI use its
  newest patch.
- CI tests both ends: the newest patch of the release `go.mod` names, and
  that of the one `mise.toml` names.

### Costs

- A program built with a Go release that is no longer supported cannot take
  a new version of the module. It keeps the version it has.
- A requirement whose newest release needs a newer Go than the floor is held
  back until the floor moves.
- The line is moved twice a year, by hand, since Dependabot does not; the
  `bump-go` skill describes how.

## 3. What the kit may require

Status: accepted

### Context

Every requirement of the kit becomes a requirement of each tool that uses
it, and, through library packages that report progress with it, of every
program that imports those. Minimal version selection then raises their
versions of shared modules to the highest any requirement asks for: a
single package of the MCP Go SDK inside the kit would raise every such
program's version of that SDK to the kit's.

### Decision

- The kit may require `github.com/GSI-HPC/go-nodeset`, to fold node names,
  and `golang.org/x/text`, for display widths. Anything else takes a record
  here first.
- Never: the MCP Go SDK (an MCP bridge takes a send callback instead),
  charmbracelet (the displays write to an `io.Writer` themselves), testify
  and OpenTelemetry (progress is reported as the kit's own events).
- cobra only in a package about cobra, `cobratree`, when it arrives.
  `termtext`, `progress` and `fanout` are imported by library packages too,
  and never import cobra.
- Test helpers ship as `xxxtest` packages, such as `progress/progresstest`,
  and report through `testing.TB`. Tests use the standard library.
- golangci-lint's depguard refuses the imports this record rules out.

With the Go floor at 1.26 (decision 2), `golang.org/x/sys` and
`golang.org/x/term` no longer need a newer Go than the kit; either one still
takes a record here before it is required.

### Costs

- A display, a test assertion or an exporter that a third-party module
  would offer is written here, or done without.
- A program's exit codes stay in the program, behind a `Classify` hook, so
  that the kit needs no package of exit codes.

## 4. A release is a signed tag

Status: accepted

### Context

A version of a Go module is a tag. The module proxy serves any tag of a
public repository as soon as someone asks for it, and the checksum database
records its content for good: deleting or moving the tag afterwards does not
unpublish it.

### Decision

- Nothing in the tree names a version: no `VERSION` file, no constant, no
  `CHANGELOG.md`, and `CITATION.cff` has no `version`.
- The maintainer releases by pushing an annotated tag `vX.Y.Z`, signed with
  an SSH key. The body of the tag message, everything after its first line,
  is the release notes.
- The Release workflow refuses a tag that no key in the
  `RELEASE_ALLOWED_SIGNERS` repository variable signed under the name it
  was pushed as, and a tag the go command would not accept as a version of
  this module. It tests the tagged commit on both Go lines, publishes the
  GitHub release with the tag body as its notes, and fetches the version
  through proxy.golang.org, so that pkg.go.dev lists it.
- A tag is never moved or deleted. A broken release is withdrawn with a
  `retract` directive in `go.mod`, which ships in the next release.
- The module stays at v0 while its API settles, and a v0 minor release may
  break it; the release notes say how. A new package arrives in a minor
  release. v1.0.0 is a decision of its own, once programs have used the kit
  through a release without changing its API.

`doc/release.md` says how to cut a release and how to set up the
verification.

### Costs

- The workflow cannot stop the module proxy from serving a tag it refused.
  Its failure is the alarm, and retraction is the remedy; a tag ruleset that
  lets only the maintainers create `v*` tags is what keeps others from
  pushing one.
- The signing key becomes part of the release process. OpenPGP signatures
  are refused.
