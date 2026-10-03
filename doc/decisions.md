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
| [5](#5-progress-and-its-displays-are-the-kits-own) | Progress and its displays are the kit's own | accepted |
| [6](#6-the-pools-are-the-kits-own) | The pools are the kit's own | accepted |
| [7](#7-escapes-are-for-a-reader-not-for-decoding) | Escapes are for a reader, not for decoding | accepted |
| [8](#8-widths-err-wide-and-follow-unicode-180) | Widths err wide, and follow Unicode 18.0 | accepted |

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
  an SSH or an OpenPGP key. The body of the tag message, everything after
  its first line, is the release notes.
- The Release workflow refuses a tag that no key in the
  `RELEASE_ALLOWED_SIGNERS` (SSH) or `RELEASE_ALLOWED_PGP_KEYS` (OpenPGP)
  repository variable signed under the name it was pushed as, and a tag the
  go command would not accept as a version of this module. It tests the
  tagged commit on both Go lines, publishes the GitHub release with the tag
  body as its notes, and fetches the version through proxy.golang.org, so
  that pkg.go.dev lists it.
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
- The signing key becomes part of the release process. Two formats are
  accepted, so that a maintainer signs with the key they already use; the
  verification and its tests cover both.
- An OpenPGP key is trusted as the variable holds it: an expired key stops
  verifying on its own, a revoked one only once the variable holds its
  revocation.

## 5. Progress and its displays are the kit's own

Status: accepted

### Context

The kit's packages came from clusterctl, whose
[ADR 0021](https://github.com/GSI-HPC/clusterctl/blob/v0.4.0/doc/adr/0021-progress-as-our-own-events.md)
chose events of its own over OpenTelemetry and measured the two in its
binary: the event model added 69,632 B, one package and no module;
OpenTelemetry's `sdk/trace` 1,056,768 B, 39 packages, 12 modules and 9
requirements. A target's lifecycle cost 1.7 µs and 8 allocations with a
sink, against 2.9 µs and 20 allocations with a span processor that does
nothing; and 19 ns and no allocation without a Bus. ADR 0021 compared no
display library, and a kit that sind and the libraries it exports import too
has to answer for both.

The design review of 27 September 2026 that preceded the kit built the
candidates into sind and clusterctl and measured what each costs a program
that links it:

| Candidate | What it costs | What it lacks |
| --- | --- | --- |
| The kit: `progress` alone | +65,536 B in sind | |
| The kit: `progress`, `display`, `fanout` and `go-nodeset` | +233,472 B in sind (+2.2%), 5 packages, no third-party module | |
| OpenTelemetry's API alone | 3 to 7 requirements | A display, a queued state, an in-process sink |
| [mpb](https://github.com/vbauerster/mpb) | +704 KB, and a `go 1.26` line when the kit's floor was lower | A tree of spans; its bars are the model |
| [bubbletea](https://github.com/charmbracelet/bubbletea) | +1.17 MB, +15 modules | It owns the terminal and the event loop; charmbracelet is ruled out by decision 3 |
| BuildKit's `progressui` | +53 requirements | A module of its own; it lives in BuildKit's |
| [go-pretty](https://github.com/jedib0t/go-pretty)'s progress | Little | A tree, and a region that yields to a question |
| kind's spinner | None | It is internal to kind, and one line |

None of them keeps the promises the kit's displays make to the command they
share the terminal with: a region taken off before every write the command
makes and never drawn over a question, lines from other goroutines held while
one is asked, a count that reaches its total after an interrupt, and nothing
but the command's own output without a display. Nor does any escaper found
for the text a display draws: those of `kenn/termtext`, `runesafe` and
`loglayer` delete or replace characters, `go-gh` covers C0 and C1 alone,
Kubernetes' `EscapeTerminal` ESC and CR alone, and `strconv.Quote` adds
quotes and escapes newlines.

### Decision

- The kit reports progress as its own events, `progress`, and draws them
  with its own displays, `progress/display`, which write to an `io.Writer`
  themselves.
- It escapes text for a terminal with its own escaper, `termtext`, whose
  policy its package comment records.
- A program that wants OpenTelemetry converts the event log
  ([event-log.md](event-log.md)); the kit never imports it, and depguard
  keeps it so (decision 3).

### Costs

- The displays, the event model, the escaper and their fuzz targets are the
  kit's to maintain, and a terminal UI among them has narrow terminals,
  resizes, multiplexers and locales to cope with.
- A change to how a display looks is a release of the kit, and a change to
  the text consumers compare in their tests is a breaking one.
- There is no exporter and no propagation beyond reading a `traceparent`.

## 6. The pools are the kit's own

Status: accepted

### Context

A command on many hosts works on them side by side, a bounded number at a
time, and a display counts them. For the count to be right, the pool has to
announce every item as queued before the first runs, mark each running as it
takes its place, end each before it gives the place up, and end those it
never started, so that the count reaches its total after an interrupt too.
`progresstest.Check` holds every source of events to this.

The design review of 27 September 2026 tried the pools a Go program would
reach for:

- `golang.org/x/sync/errgroup` with `SetLimit` went on starting work after
  its context was cancelled, in 10 of 10 runs against none of 10 for the
  kit's `Each`, and it does not recover a panic, so a panic in one item's
  work ends the process. Its first error cancels the rest, which is right
  for some commands, but the step it reports has to be assembled around it:
  a prototype of sind's that announced and ran the items in one loop broke
  `Check`'s rule that every item is queued before the first runs.
- [`sourcegraph/conc`](https://github.com/sourcegraph/conc) recovers panics,
  but has had no release since v0.3.0, in February 2023.
- [`alitto/pond`](https://github.com/alitto/pond) is a pool that outlives
  the call, with workers to stop, where a command wants one bounded loop per
  step.

None of them reports its items, and none keeps the order of the results.

### Decision

- The kit's pools are its own: `fanout.Each`, the one bounded loop, which
  starts nothing once its context has ended; `Map`, which reports its items
  as the targets of a step, returns what each came to in the order given,
  and turns a panic in one into that item's error; and `Batches`, which runs
  a node set in batches with a pause between them.
- A pool knows no program: the program's name, its rule for the class of an
  error and the error a step ends with are options.
- A pool that stops at the first failure, as errgroup does, is a later
  option of `Map`, specified by tests that compare it with errgroup, when a
  program needs it.

### Costs

- The pools are the kit's to maintain, and their concurrency is the kit's
  to get right; the race detector, `testing/synctest` and `Check` are how
  it is tested.
- A program that wants errgroup's semantics, the first failure cancelling
  the rest, keeps errgroup until `Map` has them, and announces its items
  queued itself.

## 7. Escapes are for a reader, not for decoding

Status: accepted

### Context

`termtext.EscapeText` and `EscapeCell` show a control character, such as
ESC, as a visible escape, `\x1b`, and write a backslash as it is. Text that
holds the four characters `\x1b` therefore looks the same as text whose ESC
was escaped: the escapes are not injective, and what is shown cannot always
be decoded back to what came.

Escaping the backslash as well, as `strconv.Quote` does, would make them
injective, but would double every backslash in a Windows path, a regular
expression or a shell command a host reports, and escaped text would change
each time it was escaped again. `progress.Sanitize`, which the `Bus`
applies to every text it records, escapes with `EscapeCell` and may meet
text sanitised once already, and `progresstest.Check` accepts a text only
if `EscapeCell` leaves it as it is. Both rely on escaping twice changing
nothing, which `termtext.FuzzEscape` and `progress.FuzzSanitize` test.

### Decision

- A backslash is written as it is. The escapes are for a person reading a
  terminal, and escaping is idempotent.
- The package comment of `termtext` says that the escapes cannot always be
  decoded, and that a program that has to tell the two apart keeps the text
  as it came.

### Costs

- A host can print text that looks like an escaped control character
  without one. It cannot move the cursor or change the terminal with it,
  which is what the escaper guards against.
- A program that needs the original, to compare or store it, cannot take it
  from a display, from escaped text or from the event log, whose texts
  `progress.Sanitize` escapes too, and keeps its own copy.

## 8. Widths err wide, and follow Unicode 18.0

Status: accepted

### Context

A display cuts each row with `termtext.Truncate` to the columns of the
terminal, so that it never wraps: a row that wraps pushes the region down
and leaves a copy of it behind with each frame. A width that is less than a
terminal draws breaks this; one that is more only cuts a row short.

`golang.org/x/text/width`, the East Asian Widths `RuneWidth` reads, knows
Unicode 15.0, so emoji and other characters made wide since then were
counted as one column. A character followed by the variation selector
U+FE0F, such as a red heart or a keycap, is drawn as a two-column emoji
picture by terminals but was counted as one. A full grapheme clusterer, or
a module such as `go-runewidth` or `uniseg`, would need a record under
decision 3, and terminals do not agree among themselves on how wide a
joined emoji sequence is.

### Decision

- Widths are meant never to be less than a terminal draws: a row may be
  cut a little short, but never wraps.
- `RuneWidth` adds to the tables of `golang.org/x/text` a table of its own,
  kept by hand in `termtext/width.go`, of the characters whose East Asian
  Width is wide in Unicode 18.0 but not in those tables. The soft hyphen
  and the prepended concatenation marks, format characters that are drawn,
  take one column, as does U+1171E, which the Unicode tables of Go, that
  `RuneWidth` reads for the marks of no width, call a combining mark and
  Unicode 18.0 a spacing mark; it is the only such character.
- `Width` and `Truncate` count a character of one column followed by U+FE0F
  as two columns, and `Truncate` keeps such a character without its
  selector when the selector would reach past the columns given.
- Graphemes are not clustered: a sequence joined with U+200D, a skin tone
  modifier or a flag counts as the sum of its runes, which is more than a
  terminal shows, never less.

### Costs

- The table has to be brought up to date by hand when a version of Unicode
  makes more characters wide, until `golang.org/x/text` catches up; a wide
  character assigned after Unicode 18.0 counts as one column until then.
  A mark that a later version of Unicode makes spacing has to be added to
  `RuneWidth` by hand in the same way.
- A terminal that draws a character with U+FE0F as one column shows less
  than `Width` counts, and a row holding one is cut a column short there.
- A joined emoji sequence is counted wider than it is drawn, and a row
  holding one is cut shorter than it need be.
