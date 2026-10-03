<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# AGENTS.md

go-clikit is the Go module `github.com/GSI-HPC/go-clikit`: the runtime that
GSI-HPC's command-line tools share, as packages imported by their own paths
(`…/go-clikit/progress` and so on). The root package `clikit` only documents
the kit. API reference: <https://pkg.go.dev/github.com/GSI-HPC/go-clikit>.
`doc/README.md` maps the rest of the documentation. Personal, uncommitted
instructions belong in `AGENTS.local.md` or `CLAUDE.local.md` (both
gitignored).

## Status

The packages, moved here with their history from the repository where they
were written, which `README.md` names. v0: the API may still change in a
minor release (see Compatibility).

| Package | What it holds | Release |
| --- | --- | --- |
| `termtext` | escaping untrusted text for a terminal, and display width | v0.1.0 |
| `progress` | spans in a `context.Context`, the JSONL event log, `TRACEPARENT` | v0.1.0 |
| `progress/display` | the live tree, the counter, plain lines, the summary | v0.1.0 |
| `progress/progresstest` | capturing and checking the events a command reports | v0.1.0 |
| `fanout` | bounded worker pools that report their targets as progress | v0.1.0 |
| `progress/cliprogress`, `cobratree`, `progress/mcpprogress` | the command-line glue for progress, cobra helpers, progress over MCP | later, each once a second program needs it |

## Layout

- `doc.go`: the package comment of `clikit`, which lists the packages.
- `termtext/`: `escape.go`, the escaper and its policy (`doc.go`);
  `width.go`, display widths and `Truncate`.
- `progress/`: `progress.go`, the vocabulary (kinds, states, classes,
  flags, `Fields`, `Event`, the sink interfaces and the options); `bus.go`,
  the `Bus`, spans and `Suspend`; `classify.go`; `lines.go`, `Tee` and the
  lines of output; `sanitize.go`; `tally.go`; `log.go`, the event log;
  `trace.go`, the W3C trace context. `testdata/log-v1.jsonl` is the golden
  event log.
- `progress/display/`: `terminal.go`, the `Terminal` and its writers;
  `tree.go`, `counter.go`, `plain.go` and `summary.go`, one display each.
  `contract_test.go` holds the displays to their promises on a `Screen`.
- `progress/progresstest/`: `progresstest.go`, `Capture`, `Check`,
  `Watch`, `Watcher` and `Tree`; `screen.go`, `Screen`.
- `fanout/`: `each.go`, `Each` and the package comment; `map.go`, `Map`
  and `Failure`; `batches.go`; `recover.go`, `Recovered`.
- Every package has an `example_test.go`. Failing fuzz inputs go under the
  package's `testdata/fuzz/`.
- `doc/`: `README.md` maps the documentation; `architecture.md` says how
  the packages fit together, `event-log.md` describes the event log,
  `testing.md` says how the kit is tested, `decisions.md` records what was
  decided and why, and `release.md` says how a release is cut.
- `.github/workflows/ci.yml`: tests on both Go lines and on macOS, coverage,
  fuzzing, lint, Markdown, REUSE, govulncheck and the tag verification test.
  `release.yml`: verifies a pushed `v*` tag and publishes the release.
- `.github/actions/setup-go`: Go at the newest patch of the `floor` (go.mod)
  or `current` (mise.toml) release line.
- `.agents/skills/`: skills for coding agents; `.claude/skills` links there.

## Commands

```bash
mise install          # Go and golangci-lint at the versions CI uses
make lint             # golangci-lint v2 (.golangci.yml, gofmt + goimports)
make test             # go test -race ./...
make floor            # vet and test with the go line of go.mod
make cover            # go-test-coverage: every file at 100% (.testcoverage.yml)
make fuzz             # every fuzz target for 60 s, as CI runs them (FUZZTIME, FUZZ)
make tidy             # go mod tidy and go mod verify
make vuln             # govulncheck
make reuse            # reuse lint (pip install reuse)
make lint-docs        # markdownlint-cli2 (needs npx)
make test-release     # the tag verification script against scratch tags
```

go-test-coverage: `go install github.com/vladopajic/go-test-coverage/v2@latest`.

## Code conventions

- Every file carries the two SPDX lines this one starts with, the copyright
  holder and the licence, in its own comment style (decision 1); a `SKILL.md`
  carries them after its front matter. A file that cannot hold a comment,
  such as `go.sum`, is annotated in `REUSE.toml`.
- Requirements follow decision 3: `go-nodeset` and `golang.org/x/text`, and
  anything else only with a record. Never the MCP Go SDK, charmbracelet,
  testify or OpenTelemetry. cobra only in `cobratree`; `termtext`,
  `progress` and `fanout`, which library packages import too, never import
  it. depguard enforces this.
- Test helpers ship as `xxxtest` packages and report through `testing.TB`.
  Tests use the standard library, table-driven, with `t.Errorf`/`t.Fatalf`.
- Every file stays at 100% statement coverage. A branch no test can reach is
  deleted, not excluded.
- Every exported identifier has a doc comment. Usage is shown in `Example`
  functions, which pkg.go.dev renders.
- Wrap errors with `fmt.Errorf("context: %w", err)`. An exported error value
  or type is API.
- The kit knows no program: a program's name and the noun for its targets,
  such as "hosts", are parameters, and exit-code rules stay in the program,
  behind hooks such as `Classify`.
- Interfaces beyond the Go API: the six span kinds, the JSONL event log
  (version 1: keys may be added, never renamed or removed), and the text the
  displays draw, which consumers compare in their tests. Changing any of
  them is a breaking change.
- A library that reports spans costs nothing without a Bus: no allocation
  on that path, which a benchmark or `testing.AllocsPerRun` keeps honest.
- Fuzz targets sit next to the code, and a failing input found by CI is
  committed under `testdata/fuzz/`.
- Comments and documentation are plain British English.
- Never commit a `go.work` or a `replace` directive. To try a change in a
  program that imports the module, put an uncommitted `go.work` in a parent
  directory.

## Compatibility

- The `go` line is the oldest Go release the Go project supports, as a `.0`
  release, with no `toolchain` line (decision 2). Raising it is a commit of
  its own; the `bump-go` skill covers it.
- Semantic versions. v0 for now: a breaking change is allowed in a minor
  release and is named in the release notes. Adding API or a package is a
  minor release.
- Nothing in the tree names a version (decision 4). A pushed tag is never
  moved or deleted; a broken release is retracted in `go.mod`.

## Branches, releases and commits

- PRs target `main`. Update a feature branch by rebasing it onto `main`
  (never merge `main` in), then `git push --force-with-lease`.
- Releases are signed `vX.Y.Z` tags, created by the maintainer
  (`doc/release.md`). Never push to `main`, create tags or releases, or
  merge PRs.
- Ask the maintainer before commenting on issues or PRs, and never
  @-mention anyone.
- Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`,
  `build:`, `ci:`), with the package as the scope (`feat(progress): …`),
  one logical change per commit, bullet-list bodies that say what changed
  and why, without development narrative.
- Commits by Claude Code are authored as `Claude <noreply@anthropic.com>`
  and carry a `Co-Authored-By: Claude …` trailer. Keep both; the README AI
  disclosure relies on them.
- A change that takes a decision adds it to `doc/decisions.md`. A change
  of behaviour updates the doc comments, and the document in `doc/` that
  describes it, in the same PR.

## Skills

- `.agents/skills/steward`: the PR and CI routine.
- `.agents/skills/bump-go`: moving the Go release lines after a Go release.
