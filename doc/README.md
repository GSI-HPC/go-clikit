<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# go-clikit documentation

This directory records how the kit is built and why, for someone who
changes it. How to use each package is in its doc comments and examples,
which pkg.go.dev publishes:
<https://pkg.go.dev/github.com/GSI-HPC/go-clikit>. The documentation
site, <https://gsi-hpc.github.io/go-clikit/>, built from `site/`,
introduces the kit to someone who uses it, and shows these documents as
pages of its own (decision 48).

| Document | What it covers |
| --- | --- |
| [architecture.md](architecture.md) | How the kit came to be, how the packages fit together, what runs on which goroutine, the hooks a program fills in, and the convention for standard error |
| [event-log.md](event-log.md) | The JSONL event log as an interface: its lines, keys and values, the rule for versions, what never goes in, and how the kit reads it back |
| [testing.md](testing.md) | What is tested and how: the contract of the events, fake clocks, the Screen, the command line, the files and the doctor, the pools, fuzzing, the path that allocates nothing and the cost of a large step |
| [decisions.md](decisions.md) | What was decided, why, and what it costs, the measured comparison with other display and pool libraries among it |
| [ui-convention.md](ui-convention.md) | What the programs with a local web page share and the kit cannot hold in code: the key table, the theme, the storage keys, the headers and the accessibility floor |
| [signal-recipe.md](signal-recipe.md) | The signal context of `main`, which the kit leaves to each program |
| [release.md](release.md) | Cutting, withdrawing and verifying a release |

## Everything else

| For | Where |
| --- | --- |
| A program that uses the kit | The [documentation site](https://gsi-hpc.github.io/go-clikit/) for the concepts and guides, the doc comments and examples on pkg.go.dev for each name, and the [README](../README.md) for installing and versions |
| Someone choosing a display in a program that uses the kit | That program's manual: the kit documents the mechanism, each program its flags and environment variables |
| A contributor, person or agent | [AGENTS.md](../AGENTS.md): commands, conventions and rules |
| Someone reporting a vulnerability | [SECURITY.md](../SECURITY.md) |
| Someone citing the kit | [CITATION.cff](../CITATION.cff) |
| Someone reading release notes | The signed tags, published as [GitHub releases](https://github.com/GSI-HPC/go-clikit/releases) |

## Keeping it true

- The examples compile and run with the tests, and golangci-lint refuses an
  exported name without a doc comment.
- A change of behaviour updates the doc comments, and the document here that
  describes it, in the same pull request.
- A decision is never edited; a later one supersedes or refines it, and the
  earlier one's status names it. A note under the status corrects only what
  the decision foresaw and did not happen.
- The golden log, `progress/testdata/log-v1.jsonl`, pins the format
  `event-log.md` describes.
- The tests of `internal/docsite` hold the site to the code: its
  recordings of the displays, its map of the imports, the names it links
  on pkg.go.dev, the examples it shows and the lines of the event log it
  quotes.

Deliberately absent: a changelog, a `CONTRIBUTING.md` and a README per
package. The signed tags, `AGENTS.md` and pkg.go.dev say what they would.
