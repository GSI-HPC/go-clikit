<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Decisions

One section per decision, in the order they were taken. Each says what the
situation was, what was decided, and what that costs. A decision is never
edited: a later one supersedes or refines it, and the earlier one's status
names it.

| | Decision | Status |
| --- | --- | --- |
| [1](#1-apache-20-and-gsi-holds-the-copyright) | Apache-2.0, and GSI holds the copyright | accepted |
| [2](#2-the-go-line-is-the-oldest-go-release-still-supported) | The go line is the oldest Go release still supported | accepted |
| [3](#3-what-the-kit-may-require) | What the kit may require | accepted, refined by [22](#22-the-kit-may-require-golangorgxsys-in-cliprogress-alone) |
| [4](#4-a-release-is-a-signed-tag) | A release is a signed tag | accepted, superseded in part by [10](#10-a-daily-audit-holds-the-releases-to-their-record) |
| [5](#5-progress-and-its-displays-are-the-kits-own) | Progress and its displays are the kit's own | accepted |
| [6](#6-the-pools-are-the-kits-own) | The pools are the kit's own | accepted, refined by [12](#12-the-programs-identity-lives-on-the-bus) |
| [7](#7-escapes-are-for-a-reader-not-for-decoding) | Escapes are for a reader, not for decoding | accepted, refined by [14](#14-the-escape-deny-list-may-grow-in-a-minor-release) |
| [8](#8-widths-err-wide-and-follow-unicode-180) | Widths err wide, and follow Unicode 18.0 | accepted |
| [9](#9-the-region-comes-off-one-line-a-row) | The region comes off one line a row | accepted |
| [10](#10-a-daily-audit-holds-the-releases-to-their-record) | A daily audit holds the releases to their record | accepted |
| [11](#11-a-deadline-ends-a-pools-items-as-an-interrupt-does) | A deadline ends a pool's items as an interrupt does | accepted, refined by [12](#12-the-programs-identity-lives-on-the-bus) and [13](#13-the-skip-error-lives-in-progress) |
| [12](#12-the-programs-identity-lives-on-the-bus) | The program's identity lives on the Bus | accepted |
| [13](#13-the-skip-error-lives-in-progress) | The skip error lives in progress | accepted |
| [14](#14-the-escape-deny-list-may-grow-in-a-minor-release) | The escape deny-list may grow in a minor release | accepted |
| [15](#15-a-new-or-stricter-check-rule-is-a-breaking-change) | A new or stricter Check rule is a breaking change | accepted |
| [16](#16-a-large-set-of-targets-is-folded-again-once-a-second) | A large set of targets is folded again once a second | accepted |
| [17](#17-the-cost-tests-run-in-ci-alone) | The cost tests run in CI, alone | accepted |
| [18](#18-the-live-clocks-tick-in-tenths-of-a-second) | The live clocks tick in tenths of a second | accepted |
| [19](#19-the-displays-draw-in-themes-the-program-picks) | The displays draw in themes the program picks | accepted |
| [20](#20-a-themes-look-may-change-in-a-minor-release) | A theme's look may change in a minor release | accepted |
| [21](#21-the-command-lines-glue-for-progress-is-the-kits) | The command line's glue for progress is the kit's | accepted |
| [22](#22-the-kit-may-require-golangorgxsys-in-cliprogress-alone) | The kit may require golang.org/x/sys, in cliprogress alone | accepted |

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

Status: accepted, refined by
[decision 22](#22-the-kit-may-require-golangorgxsys-in-cliprogress-alone)

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

Status: accepted, superseded in part by
[decision 10](#10-a-daily-audit-holds-the-releases-to-their-record)

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

Status: accepted, refined by
[decision 12](#12-the-programs-identity-lives-on-the-bus)

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

Status: accepted, refined by
[decision 14](#14-the-escape-deny-list-may-grow-in-a-minor-release)

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

## 9. The region comes off one line a row

Status: accepted

### Context

A display takes its region off the terminal by moving the cursor up and
clearing a line at a time, before every write the command makes and
before every frame. It cannot ask the terminal where the region now is.
Each row is cut to fit the terminal when it is drawn, but a terminal made
narrower since does one of two things with a row wider than it now is.
Most, VTE, Konsole, iTerm2, Terminal.app, Windows Terminal and tmux among
them, reflow it onto as many lines as it takes; xterm, the Linux console
and GNU screen cut it, and it stays one line. Neither says which it does.

Counting the lines a row takes at the new width takes the region off whole
on a reflowing terminal, but on a cutting one it clears as many lines
above the region as the count goes past, and those are the command's own
output. Counting one line a row takes the region off whole on a cutting
terminal, and on a reflowing one leaves the first lines of each such row
above the region.

### Decision

- The region comes off one line for each of its rows, as they were drawn,
  whatever the width is now. The region is all a display owns: it never
  clears a line it cannot be sure is its own.
- The frames drawn after a resize are cut to the new width, so the region
  is exact again from the next frame on.

### Costs

- On a reflowing terminal made narrower under a drawn region, the first
  lines of each row wider than the new width stay on the terminal, above
  the next frame and the command's output, until they scroll away.

## 10. A daily audit holds the releases to their record

Status: accepted

### Context

Decision 4 has the Release workflow verify a tag when it is pushed. A tag
push runs the workflow as the tagged commit has it, so a tag on a commit
from before the workflow, or on one that changes it, verifies nothing. The
module proxy keeps what it fetched first under a version: a tag pushed,
fetched through the proxy and deleted again leaves the version served
without a tag, and a tag deleted and pushed again on another commit, even
a signed one, leaves the proxy serving the first. The keys change as well:
an OpenPGP key that expires stops verifying the tags it signed, and a key
that is retired or leaked has to be removed without failing the releases
it signed.

A bad version cannot be removed from the proxy, only withdrawn with a
`retract` directive, so an alarm that goes on failing for a version that
has been withdrawn would hide the next one. Withdrawing a version answers
for the content the proxy serves under it, though, not for whatever its
tag names later.

### Decision

- The Release workflow also runs every day from `main`, and by hand, and
  then verifies every `v*` tag against the listed keys. It reads the
  versions proxy.golang.org lists for the module, and fails for each that
  has no tag, and for each whose `.info` names, as `Origin.Hash`, another
  commit than its tag. A version whose `.info` has no `Origin`, which the
  proxy leaves out for a version it fetched long ago, is unverified, a
  warning, unless the `Time` in it is not the committer date of the commit
  the tag names, which fails. When the list cannot be read, the audit
  verifies the tags all the same, and fails. Its failure is the alarm.
- The publishing job fetches the version through the proxy, checks that
  the proxy serves it from the verified commit, and only then publishes
  the GitHub release, so that no release is published for a version the
  proxy refused or serves from another commit. Run again, it leaves a
  release an earlier attempt published as it is.
- A published release is pinned in the `RELEASE_VERIFIED_TAGS` repository
  variable: its tag and the id of the tag object verified when it was
  pushed. The audit holds a pinned tag to that object instead of to
  today's keys, and fails once the tag names another object or is gone.
- A key is retired by pinning the releases it signed and then removing it;
  a key that may have leaked is removed at once, and only the releases
  known to be genuine are pinned. A key is never retired with
  `valid-before`, which git checks against the date in the tag, a date
  the signer writes.
- A version that `go.mod` retracts on a line of its own, not as part of a
  range, acknowledges the alarm for what the retraction withdraws, the
  content the proxy serves under it. The audit then reports as a warning,
  and passes, only a missing tag, a version the proxy lists without one or
  a pinned release whose tag is gone, and a tag that is neither pinned nor
  signed while the proxy records, as `Origin.Hash`, that it serves the
  version from the commit the tag names. It still fails a retracted
  version whose tag names another commit than the proxy records, moved or
  deleted and pushed again; a pinned tag that is not its pinned object,
  whatever commit it names; and a bad tag whose version the proxy records
  no commit for, since nothing then says which commit was withdrawn.
- A tag pushed as a release is still never moved or deleted, with two
  exceptions. A tag that was not pushed as a release, such as one an
  intruder moved, or deleted and pushed again, is restored to the pinned tag
  object for a pinned release, or to the tag the proxy fetched, or deleted.
  A bad tag of a retracted version whose commit nothing records is deleted,
  even one pushed as a release, since the retraction can acknowledge only a
  version without a tag then.

`doc/release.md` says how the audit is read, and how a release is pinned
and withdrawn.

### Costs

- The audit detects and does not prevent: it finds a bad tag up to a day
  late. What keeps others from pushing or deleting a `v*` tag is a tag
  ruleset that lets only the maintainers create one and nobody delete it.
- It relies on proxy.golang.org's list of versions for a tag deleted
  between two runs, and on the proxy's `Origin` for the commit a version
  is served from. For a version without `Origin` the time check is only a
  consistency check: a commit made with the same committer date passes it.
- Every release is pinned by hand once it is published; until it is, the
  audit holds it to today's keys, and an unpinned release fails once its
  key expires or is removed.
- GitHub stops the schedule of a repository with no activity for 60 days,
  and the workflow has to be enabled again by hand.
- A version is acknowledged one line at a time, so a range of bad versions
  is retracted version by version. A retracted version's tag that is
  deleted is gone from the repository for good; the proxy, the checksum
  database and the `retract` line are then its only record.
- What a retraction acknowledges stays in every run of the audit as a
  warning.

## 11. A deadline ends a pool's items as an interrupt does

Status: accepted, refined by
[decision 12](#12-the-programs-identity-lives-on-the-bus) and
[decision 13](#13-the-skip-error-lives-in-progress)

### Context

`progress` names `ClassTimeout` for work that ran out of time and
`ClassCanceled` for work that was interrupted, and `progress.Classify`
classes `context.DeadlineExceeded` as a timeout. A pool runs its items
under one context, though, and when that context ends, whether interrupted
or past its deadline, the pool starts no further item, and the work of
those running gives up with whatever error it returns, which need not say
why. Classed by their own errors, the items one deadline cut short would
end as timeouts, failures of their own, or as whatever their work
returned, and the step would end failed, although no item ran out of its
own time.

### Decision

- The end of a pool's context ends its items canceled, `ClassCanceled`,
  whether it was interrupted or ran out of time: those `Map` left out, and
  those whose `Acquire` refused them or whose work returned an error other
  than of `Skip` once the context had ended. Their targets end with the
  context's error, in a type that says `ClassCanceled`, and when every item
  that failed ended so, `Summarize` is told that they were interrupted,
  which `Failure` ends canceled. The pool, not the item, stopped the work.
- `Batches` ends the batches the end of its context left out canceled in
  the same way. Its step, when no batch failed, and a `stagger` wait the
  context ended, end with the context's error as it is, so that a deadline
  ends them failed, as timeouts, and an interrupt canceled; a batch that
  was run ends with what its run returned.
- An item that runs out of its own time, a call whose own deadline passed
  while the pool's context had not ended, is classed by its error as any
  other, and so stays a timeout, a failure of that item.
- A panic, or a call of `runtime.Goexit`, fails its item even once the
  context has ended.

### Costs

- An event log or a display does not tell from the class of an item, or of
  `Map`'s step, whether a pool was interrupted or ran out of time; the
  error text in the event log, `context canceled` or `context deadline
  exceeded`, does, and a program asks its context, with `ctx.Err` or
  `context.Cause`, to tell the two apart, such as to exit with another
  code.
- `Batches` classes its step and its wait otherwise than its batches and
  `Map`'s step: under a deadline its step ends failed, as a timeout, while
  the batches it left out end canceled.
- The outcome of an item cut short keeps the error its work returned,
  which may be a timeout of its own, while its target ends canceled.
- An item whose work fails for a reason of its own just as the context
  ends is counted as cut short, not as failed.

## 12. The program's identity lives on the Bus

Status: accepted

### Context

Decision 6 made the program's name, its rule for the class of an error and
the error a step ends with options of each pool. The name, the panic log
and the class rule were so given separately to the Bus, the event log, the
terminal and every call of `fanout.Map`. A library that calls `Map` knows
none of them: a panic in its work named no program and wrote its stack
straight to standard error, over a live display, and `Map` told an item
canceled by its own `Classify` while the Bus classed the item's target by
the program's rule, so that the step and its targets could disagree.
`Map` worked out the error its step ended with and dropped it, and
`Summarize` was told of the items that failed in parallel slices and a
bool named `interrupted`, which a deadline sets as well (decision 11).

### Decision

- The program's name, its class rule and its panic log are options of the
  Bus, which `Bus.Program`, `Bus.PanicLog` and `Bus.Classify` hand on. A
  pool takes them from the Bus its context carries; its own options,
  `MapOptions.Program`, `PanicLog` and `Classify`, override the Bus's, and
  without either it falls back to "the program", standard error and
  `ClassTarget`.
- The rule that tells an item canceled, for the step's summary, is the one
  that classes the item's target: the pool's override, or else the Bus's.
- The options stay, as overrides: a program whose exit code follows a
  pool's result sets `Classify`, so that the result does not depend on
  whether a Bus is there, as it is not with no display.
- `Recovered(log, program, target, v)` stays the explicit form, for code
  that has a log and no Bus; a library with a Bus calls it with the Bus's
  `PanicLog` and `Program`. `Batches` recovers no panic and has none of
  these options.
- `Map` returns the error its step ended with. `Summarize` and `Failure`
  take a `Summary`, a struct that can grow, and decision 11's
  `interrupted` is its `Canceled`.

### Costs

- Without its own `Classify`, what a pool's step and `Summary.Canceled`
  say follows the Bus's rule when there is a Bus, and `ClassTarget`
  otherwise; a program that needs one answer either way sets the option.
- The program's identity is still given in more than one place for the
  parts that do not take it from a Bus: `Recovered`, and the terminal a
  display draws on.

## 13. The skip error lives in progress

Status: accepted

### Context

`fanout.Skip` made the error of an item left out on purpose, and
`fanout.IsSkipped` told it from a failure. Only `Map` knew the error, though:
`Span.End` ended a span with it failed, so code that ended its own spans had
to set the error and call `Span.Skip` as well, and a program's rule for
classes could turn it into a failure. Commands passed the error on beyond
`Map` too, in results of their own. `Batches` ended a batch whose run
returned it failed, stopped the run, and ended the batches after it as not
tried, "an earlier batch failed", although nothing had failed.
`IsSkipped(err) bool` is the style of `os.IsNotExist`, which Go has moved
away from for `errors.Is` and a sentinel.

### Decision

- `progress` holds the error: `ErrSkipped`, and `Skip(reason)`, an error
  whose text is the reason and in which `errors.Is` finds `ErrSkipped`.
- `Span.End` ends a span whose error is `ErrSkipped`, as `errors.Is` tells,
  skipped, with no class and the error's text, before `Classify` or the
  Bus's fallback is asked, so that no program's rule turns a skip into a
  failure. `Span.Skip(reason)` is `End(Skip(reason))`.
- `fanout.Skip` and `fanout.IsSkipped` are gone, without aliases; the pools
  test `errors.Is(err, progress.ErrSkipped)`.
- A batch whose run returns a skip ends skipped and does not stop
  `Batches`, which ends its step ok when nothing else failed.
- `fanout.ErrNotTried` stays a distinct error, no skip: a batch not tried
  ends skipped on a display, but the error says that a failure left it
  out, and a command that counts failures by `errors.Is(err,
  progress.ErrSkipped)` still counts it.

### Costs

- `Span.End` behaves otherwise for an error that is `ErrSkipped`, which an
  error opts into; one that wraps `ErrSkipped` by accident ends skipped.
  So does one that joins a skip with a failure, since `errors.Is` looks
  through every error joined: a `Batches` run that joins its nodes'
  errors returns a skip only when every node was skipped, which the doc
  comments of `ErrSkipped`, `Skip`, `Span.End`, `Map` and `Batches` say.
- An item whose skip is wrapped, `fmt.Errorf("exe01: %w", progress.Skip(r))`,
  ends with the whole text as its Err, not the reason alone as before.
- `Bus.Classify` and `progress.Classify` know nothing of skips; only `End`
  tells them apart, so code that classes an error itself asks
  `errors.Is` first.
- Programs that called `fanout.Skip` or `fanout.IsSkipped` change, each
  call mechanically.

## 14. The escape deny-list may grow in a minor release

Status: accepted

### Context

`termtext` escapes by a deny-list: the runes that can move the cursor,
change the state of the terminal, or change the order or the lines text is
shown in. Terminals keep adding sequences, and a character the list leaves
out today may turn out to act on one tomorrow. The text the displays draw is
an interface consumers compare in their tests, and so are the escapes in
it. If the set of runes escaped were frozen with that text, adding a rune to
the list, a fix for a terminal's safety, would be a breaking change, and
would wait for a major release or be weighed against one.

### Decision

- The forms of the escapes are stable: `\r`, and `\n` and `\t` where
  `Escape` escapes them; `\x` and two digits for any other C0 control, DEL
  and a byte that is not UTF-8; `\u` and four digits for any other rune;
  hexadecimal digits in lower case. Changing a form is a breaking change.
- The set of runes escaped may grow in a minor release, which names the
  runes it adds in its release notes. Removing a rune from it is a breaking
  change.
- Escaping stays idempotent: an escape is printable ASCII, which the
  deny-list never names.
- The package comment of `termtext` says this, and the list of interfaces
  beyond the Go API in `AGENTS.md` does not name the escapes.

### Costs

- Text that held a rune added to the list is shown, and stored in the event
  log, escaped by a later release where an earlier one wrote it as it was;
  a consumer's test that compares such text changes with the release.
- An event log written before the change and one written after can hold
  the same text in two forms.

## 15. A new or stricter Check rule is a breaking change

Status: accepted

### Context

`progresstest.Check` holds the events of a Bus to the promises the
progress package makes to every sink: the order of spans, the work
announced up front, the limits, the suspends and the bounds of the text.
The kit tests itself with it, and programs call it, directly or through
`Watch`, in their own tests, which pass or fail by its rules. Nothing said
whether a rule may be added or tightened in a minor release, and a rule
added without notice would fail a program's tests on an upgrade although
the program had not changed.

### Decision

- Each rule `Check` enforces is a promise the progress package makes to
  every sink, and the rules are an interface beyond the Go API, next to
  the span kinds, the event log and the text the displays draw.
- A new rule, or a stricter one, is a breaking change, named in the
  release notes.
- A rule that catches what an existing promise already forbade, such as a
  case the rule missed, is a fix, released as any other.
- No option selects which rules run: a test that cannot keep a rule
  breaks a promise a display relies on.

### Costs

- A promise the progress package adds waits for a breaking release before
  `Check` holds anyone's tests to it.
- Telling a fix from a stricter rule is a judgement, made in review by
  whether the promise was already written down.

## 16. A large set of targets is folded again once a second

Status: accepted

### Context

The tree folds the targets of a step that have ended into node sets, one
row for those that ended well, one for each way they failed, and one each
for those interrupted and those left out. A frame folded a set again
whenever it had changed since the frame before, which a step whose targets
end one after another does between any two frames, ten times a second.
Folding costs O(n log n) in the names, and the tree draws a frame under its
lock, which the Bus waits for while it hands the tree an event, and every
worker that reports one waits for the Bus. A frame over a step whose
targets were ending cost 2.1 ms at 10,000 targets, 3.5 ms at 16,000 and
7.1 ms at 30,000, against 50 µs at 300. The row of the step counts the
targets as they end from a tally, which costs nothing to read.

### Decision

- A set of more than 256 names is folded again at most once a second: a
  frame drawn within the second draws its row as it was last folded. A
  smaller set is folded again for every frame it changed for.
- The row of the step counts every target that has ended, and a row that
  says how many failed rows are left out counts the sets as they are.
- The line a step leaves when it ends, with its failures below it, names
  every target.
- The 256 names and the second are constants, not options, and `Tree`'s
  doc comment states them.

### Costs

- A row of more than 256 names can leave out the targets that ended in the
  last second, while the count of the step's row includes them.
- The text the tree draws changes: a consumer's test that draws frames less
  than a second apart over more than 256 targets sees the fold of the
  earlier frame. Decision 5 makes that a breaking change, named in the
  release notes.

## 17. The cost tests run in CI, alone

Status: accepted

### Context

`TestTheCostOfALargeStep` holds what the live tree costs on a large step
to the number of its targets, by timing the same work at two sizes. Two of
the regressions it catches, a pass over a step's targets as each ends and
a search through the rows of failures, allocate nothing, so no count of
allocations could stand in for the clock. The race detector slows some
code more than other code, so the test skips under it, and CI ran every
test under the race detector: nothing in CI would have noticed the tree's
work growing quadratic in its targets again, only `make floor` on a
contributor's machine. A shared runner's timings vary, though, and every
failed check here is root-caused, so a check that fails by chance costs
time and trust.

### Decision

- CI runs the test in a job of its own, Costs, as `make costs` does:
  without the race detector, with the current Go line, alone on its runner
  so that no other package's tests compete for the CPU while it measures.
  It runs on Linux alone: the costs are the tree's, and macOS would say
  nothing more about them.
- Each part logs how many times as long its larger size took, the two
  times and its limit, so that every run's log shows how much room it has.
- A part over its limit is a regression in the tree until shown
  otherwise. If the parts run close to their limits on changes that leave
  the tree alone, the gap between the two sizes is widened, which sets
  work linear and quadratic in the targets further apart. A limit is not
  raised to pass, and the job is not re-run until green.

### Costs

- Every CI run takes one more runner, for about half a minute.
- A check that times code on a shared machine can still fail by chance.
  Its log then says which part, by how much, and how long each size took.

## 18. The live clocks tick in tenths of a second

Status: accepted

### Context

The counter's time and the tree's command row read whole seconds, `0:41`,
and so did how long each running target, step, call and wait had run, `4s`,
so that a row changed once a second rather than at every frame. A display
draws up to ten frames a second, and the terminal writes nothing for a
frame that reads as the one before. A display that changes once a second
looks stalled between changes: a request that has run for a while cannot be
told at a glance from a display that has stopped drawing.

### Decision

- The Counter's time and the Tree's command row read minutes, seconds and
  tenths, `0:41.3`, or hours, minutes, seconds and tenths, `1:02:03.4`.
- How long a running target, step, call or wait has run reads seconds and
  tenths, `4.2s`, or minutes, seconds and tenths, `3m12.4s`, and hours and
  minutes from an hour on, `1h02m`, as before; against the bound of the
  request it waits for, `3m12.4s/10m`.
- The tenths are cut, not rounded, so that a clock never runs ahead.
- The tree lists running targets by the tenth of a second each started
  in, counted from when the tree was made, those that started in the same
  tenth in the order they were queued. A row's place depends on when its
  target started, not on the time of the frame, so that two rows never swap
  places from one frame to the next, though the tenths two such rows show
  may differ by one. The tenths are read on the monotonic clock, as the
  times on the rows are, where the times have one, so that a step of the
  wall clock does not reorder the rows.
- What is no live clock stays as it was: the time in front of a plain
  line, `[0:03]`, which a log keeps; how long a span that has ended took,
  in the lines the tree leaves, the end lines of `Plain` and the summary;
  a pause's countdown, `12s left`, rounded up to the whole second; and the
  bound of a request.

### Costs

- While anything runs, every frame differs from the one before, so the
  terminal is written up to ten times a second rather than about once: a
  region of twelve rows a hundred columns wide is about 12 KB a second,
  which a slow link carries but notices.
- The text the displays draw changes: a consumer's test that compares
  frames sees `0:41.0` and `4.0s` where it saw `0:41` and `4s`. Decision 5
  makes that a breaking change, named in the release notes.

## 19. The displays draw in themes the program picks

Status: accepted

### Context

The displays drew in no colour, with one set of marks, or in ASCII outside
a UTF-8 locale. People who watch them all day asked for colour, and for
marks, spinners and bars that tell a failure, a run and the share done at
a glance. A terminal draws colour through sequences (SGR, ESC [ … m) that
`termtext` counts as columns and `progresstest.Screen` showed as text, so a
row drawn in colour was cut short or wrapped. Which colours read well
depends on the terminal: how many it shows, its background, and whether
its user set NO_COLOR, none of which the kit reads (it reads no
environment). Glyphs have their own traps: a character of ambiguous East
Asian Width is drawn two columns wide in a CJK locale, some are drawn as
emoji pictures, and fonts lack some, DejaVu Sans Mono all of braille.

### Decision

- `display.Theme` is a comparable value whose zero value is no theme, which
  draws byte for byte what the displays drew. There are five themes,
  `Classic`, `Aurora`, `Ember`, `Neon` and `Tide`; `Themes`, `ParseTheme`
  and the text methods serve a flag such as `--theme`.
- `Theme.In` draws a theme in the 256 colours it is designed in, the zero
  `Colours`, in the terminal's own 16, or in none, its art alone, for
  NO_COLOR. The program decides from its flags, NO_COLOR, TERM, COLORTERM
  and whether standard error is a terminal; the kit reads none of them.
- `TreeOptions`, `CounterOptions` and `PlainOptions` take a `Theme`, and
  `Summary` a `Theme` and `ASCII`. `ASCII` stays the switch for the
  character set: with a theme it keeps the theme's colours and draws ASCII
  marks, a bar of `[###...]`, no spinner and no guide.
- The tree and the counter draw a theme's art: its marks and separators, a
  spinner on each running target and in front of the counter's line, a bar
  on each counted step that gives way when the counts would not fit, and a
  guide at the innermost indent. Plain lines and the summary take its
  colours and one mark in front, and keep their words, punctuation and
  paths, so that what a log holds does not change.
- Colour goes on marks, count words, the clock, bars, titles and
  separators. Names, errors, requests and output lines stay in the
  terminal's own colour, and colour is never the only signal: every status
  has a mark and a word of its own.
- Every glyph of a theme takes one column, is of East Asian Width neutral
  or narrow, never ambiguous, and is no emoji, nor a rune Unicode keeps for
  emoji to come. No theme sets a background, reverse video, italic or
  blink. The palettes of 256 colours
  keep to a middle lightness, an L* of 45 to 60, which reads on light and
  dark backgrounds, though some of their colours are as vivid as any; and
  the cells of a bar that stand for the targets that failed, which only
  their colour sets apart, differ from the rest of the bar by a ΔE of 20 or
  more as readers with deuteranopia or protanopia see them too. Those of 16
  use red, green, yellow, cyan and magenta, bold and faint, never bold with
  a colour and never the bright colours, which some palettes make grey.
  Tests hold every theme to all of this.
- Every painted piece sets its colour back, so that no row and no line ends
  in a colour; the Terminal cuts a row by the columns it shows, and starts a
  frame drawn in colour by setting colours back. `progresstest.Screen`
  applies colours when its `Styles` is set, and `Styled` shows them.

### Costs

- A theme in 256 colours on a terminal that shows 16 draws other colours,
  or none; the program steps it down with `In(Colours16)` from what TERM
  says.
- Aurora's braille is drawn from a fallback font where the terminal's font
  lacks it, as DejaVu Sans Mono does; FreeMono and DejaVu Sans have it.
- A theme's colours cost bytes: over a run of 48 targets the tree wrote
  1.5 to 1.9 times as many in a theme of 256 colours as in none, and the
  counter's line, which is short, up to 3 times.
- The API grows by `Theme`, `Colours`, five variables a program could
  assign to and should not, as `io.EOF`, and two fields of `Summary`.
- The look of no theme keeps `·`, `…` and `–`, which are of ambiguous
  width: changing them would change every consumer's frames.

## 20. A theme's look may change in a minor release

Status: accepted

### Context

Decision 5 makes a change to the text consumers compare in their tests a
breaking change. A theme is art: its glyphs and colours will be tuned as
people use it, on terminals, fonts and palettes the first release did not
try, and holding them to the rule for text would freeze them.

### Decision

- What the displays draw with the zero Theme stays the text consumers
  compare: changing it is a breaking change, as decision 5 has it.
- A theme's colours, marks, separators, spinner, bar and guides may change
  in a minor release, which names the change in its release notes. Its
  name, and the words and numbers a display draws in it, may not: removing
  or renaming a theme is a breaking change, and adding one is not.
- A consumer that tests a display drawn in a theme checks the words and
  numbers it draws, such as a count or a target's name in what
  `progresstest.Screen` shows, rather than comparing its frames byte for
  byte, or pins the release. The kit exports nothing that takes a theme's
  art out of a frame, since the art is what may change.

### Costs

- A consumer's test that compares the frames of a theme byte for byte can
  break in a minor release.

## 21. The command line's glue for progress is the kit's

Status: accepted

### Context

clusterctl v0.4.0 (`internal/cli/progress.go`, `internal/fileutil`) and sind
(`cmd/sind/progress.go` and `appendprivate.go`, ported from clusterctl's)
carry the same glue between a command line and the progress packages, about
560 lines in clusterctl and 760 in sind: the five words of `--progress` and
the rule that picks a display from them, with their notes and errors, word
for word; the reading of the locale, byte for byte; the making of the
Terminal, the display, the summary and the event log, and their teardown, in
one order; and the opening of the event log for its user alone. The copies
had drifted already: sind refuses another user's symbolic link in a
directory on the way to the log, which clusterctl follows, and writes a log
on the display's own terminal through the Terminal's Lines, where clusterctl
writes it into the region. They differ in how they find out about the
terminal, go-isatty and golang.org/x/sys in sind, asked for each command,
golang.org/x/term in clusterctl, asked once, and in policy: which commands
run in a span, when TRACEPARENT leaves the environment, where notes go,
whether standard output goes through the Terminal under plain lines, and
whether a command that succeeded leaves a summary.

The kit imports no cobra (decision 3) and reads no environment, NO_COLOR
and TERM included (decision 19); which display to draw, and whether the
terminal can show one, was the program's to decide (`architecture.md`).

### Decision

- `progress/cliprogress` holds the words (`Mode`, `Modes`), the rule
  (`Choose`), a command's `Run` (`Start`, `Run.Finish`, and the writers in
  between), the opening of the event log, and the probes `IsPipe`,
  `TerminalSize` and `InForeground` (decision 22).
- It finds out nothing for itself. The program passes in `Options` the
  name and value of its flag and of its variable for each setting, a
  `Setting` each; whether standard error is a terminal, and a dumb one;
  whether standard output goes into a pipe; the terminal's size and
  foreground; whether the locale shows UTF-8, which `UTF8Locale` tells from
  a getenv the program passes; and the Theme. The kit reads no variable,
  NO_COLOR, TERM and COLORTERM included, and decides no colour: decision 19
  stands.
- It sets nothing process-wide and ends no process. `Options.Trace` is
  called only once a Bus is made, so that a program takes TRACEPARENT and
  TRACESTATE out of its environment there; the end of the command's
  context is the interrupt. forbidigo refuses `os.Getenv`, `os.LookupEnv`,
  `os.Environ`, `os.Setenv`, `os.Unsetenv`, `os.Clearenv`, `os.ExpandEnv`
  and `os.Exit`, their namesakes in `syscall` and `golang.org/x/sys/unix`,
  `log.Fatal`, `log.Fatalf` and `log.Fatalln`, a `log.Logger`'s too, and
  the handlers of `os/signal` in the kit's code, its tests aside.
- It imports no package of flags. What the flag asks for and cannot have
  is an error, which the program reports as its usage error; what the
  variable asks for fails no command, and is said in a note where it names
  no word or no log that can be used. The package exports no error.
- Notes go to `Options.Notes`, escaped, behind "prog: " as
  `TerminalOptions.Program` writes it. The summary goes to `Options.Stderr`
  unescaped, since a Theme colours it and the Bus sanitised its names.
  Without a display or a log nothing is made, so standard error holds what
  it would without the kit.
- Where the programs differed in what they share, the kit takes sind's
  way: a symbolic link on the way to the log, a directory's as well as the
  file's, has to be this user's or root's, and a log on the display's own
  file goes through the Terminal's Lines.
- Beyond both, it refuses a log's file with other names, hard links, which
  another user may have made to a file of this user's where the system
  lets them, as macOS and the BSDs do; it follows no link put at the log's
  own name after its walk of the path, whatever links lead to the
  directory; and it opens the file without blocking, so that a named pipe
  another user put there holds up no command, while one of this user's or
  root's is waited on as before.
- What stays in the program: finding out whether a stream is a terminal,
  reading its environment, the theme and its colours, taking variables
  out of the environment, signals and exit codes, which commands run in a
  span and the span itself, which of its writers go through the Terminal
  and putting them back, and whether a command that succeeded leaves the
  summary, the argument of `Run.Finish`.
- Interfaces beyond the Go API: the five words, in their order, and the
  texts of the errors `Choose` and `Start` return and of the notes `Start`
  and `Finish` write, the refusals of the log's file among them, around
  the names the program gives and the system's errors they wrap. Programs
  quote them in their manuals. Changing one is a breaking change; adding a
  word is not, though the list the texts give grows.

### Costs

- Each program keeps the lines that fill in `Options`, and the library it
  asks whether a stream is a terminal. A program that words its notes
  otherwise, or picks a display by another rule, writes its own glue on
  `display`, as before.
- `Options` has 19 fields, most of them passed on to `display` and
  `progress`, for a program to leave out; `Options.Manual` and `Run.Draw`
  are there for tests, as the displays' `Draw` is.
- clusterctl's walk of the links on the way to its log becomes stricter,
  and its log on the display's terminal moves above the display. On
  `cliprogress`, both programs refuse a log's file with other names, which
  they appended to.
- A word or a text of cliprogress is frozen with the API: rewording one
  waits for a breaking release.

## 22. The kit may require golang.org/x/sys, in cliprogress alone

Status: accepted

### Context

`cliprogress` (decision 21) asks the system about a command's streams: how
many columns and rows the terminal has (TIOCGWINSZ); whether the process
is the job in its foreground (TIOCGPGRP against its own process group);
and, for an event log a path such as /dev/stderr or /dev/fd/3 names,
whether that descriptor of the process's is open for writing (F_GETFL) and
a duplicate of it closed on exec (F_DUPFD_CLOEXEC). sind and clusterctl
make these calls through golang.org/x/sys/unix, which decision 3 lets the
kit require only with a record.

The standard library makes them only as raw system calls by number.
Measured with Go 1.26 and 1.27: on macOS `syscall.Syscall` traps into the
kernel itself rather than through libc, which Apple does not support; on
OpenBSD 7.5 and later it reaches ioctl alone, through libc, and returns
ENOSYS for every other number, fcntl's among them, so a descriptor open
for reading alone could not be told from one open for writing; Solaris,
illumos and AIX define no SYS_IOCTL; and `syscall` has no Winsize, so the
kit would declare one and pass its address through `unsafe`. A write of
no bytes, which needs no fcntl, sends an empty datagram on a Unix socket
of SOCK_DGRAM or SOCK_SEQPACKET, and POSIX leaves what it does to anything
but a regular file unspecified. golang.org/x/sys/unix makes every one of
these calls on every Unix port, through libc where the system asks for it.
Its IoctlGetInt reads TIOCGPGRP's four bytes into an int of eight, which
on a big-endian system of 64 bits puts them in the upper half; the kit
reads both halves.

golang.org/x/sys requires no module, and the go line of v0.48.0, and of
v0.49.0, the newest, is 1.26.0, the kit's floor. sind and clusterctl
v0.4.0 require v0.48.0. A program that builds no package of `cliprogress`
links none of it, but minimal version selection reads the kit's
requirement all the same. The calls cost 4,096 bytes on linux/amd64 and
128 on darwin/arm64 over the same calls through `syscall`, and sind on
`cliprogress` is 12,288 bytes larger than with its own glue.

### Decision

- The kit may require golang.org/x/sys. Only `progress/cliprogress`
  imports it, in its files built for Unix, `probe_unix.go` and
  `appendprivate.go`; its files for other systems answer that a terminal's
  size is not known and the process is in the foreground, and open the log
  as any file. depguard refuses golang.org/x/sys in every other package.
- The kit requires v0.48.0, which sind and clusterctl required when this
  was recorded, so that the first release of `cliprogress` raises neither;
  Dependabot then moves it as it moves golang.org/x/text, and with it the
  golang.org/x/sys of the programs that take a release of the kit.
- golang.org/x/term is not required: the program asks whether a stream is
  a terminal, with the library it has.
- Tests use the standard library still: a pseudo-terminal is opened
  through `syscall`.
- `make vet-other` compiles the module for Windows and Plan 9 in CI, since
  no test runs their files.

### Costs

- Every program that requires the kit has golang.org/x/sys v0.48.0 or later
  in its module graph, whether it builds `cliprogress` or not, and a
  release of the kit after Dependabot has moved the requirement raises the
  program's to it, as golang.org/x/text's is raised.
- These are the kit's first files with build tags outside its tests.
  Coverage runs on Linux, so the files for other systems are compiled and
  never run.
- On AIX, which has no F_DUPFD_CLOEXEC, the duplicate of a descriptor is
  inherited by a child started while the log is open.
- On Windows and Plan 9 a display takes the terminal to be 80 columns
  wide, and the log's file is not checked for its owner, its mode or its
  links.
