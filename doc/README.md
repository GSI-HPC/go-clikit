<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# go-clikit documentation

This directory records how the kit is built and why, for someone who
changes it. How to use each package is in its doc comments and examples,
which pkg.go.dev publishes:
<https://pkg.go.dev/github.com/GSI-HPC/go-clikit>.

| Document | What it covers |
| --- | --- |
| [decisions.md](decisions.md) | What was decided, why, and what it costs |
| [release.md](release.md) | Cutting, withdrawing and verifying a release |

Three more arrive with the packages:

- `architecture.md`: how the kit came to be, with links to the programs it
  was written for, and how the packages fit together: their dependencies,
  the concurrency of the Bus, the hooks a program fills in, and the
  convention for standard error that the programs using the kit follow,
  with progress, notes, logs, and one escaped line for the final error;
- `event-log.md`: the JSONL event log as an interface, with its lines, keys
  and value names, the rule for versions, and what never goes in;
- `testing.md`: what is tested and how, from fake clocks and the Screen
  terminal model to the bounds of the pools, fuzzing, and the path that
  allocates nothing.

The measured comparison with other display and pool libraries arrives with
them, as decisions in `decisions.md`.

## Everything else

| For | Where |
| --- | --- |
| A program that uses the kit | The doc comments and examples, on pkg.go.dev, and the [README](../README.md) for installing and versions |
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
- A decision is never edited; a later one supersedes it.
- Once `event-log.md` is here, the golden log in `testdata/` pins the format
  it describes.

Deliberately absent: a documentation site, a changelog, a `CONTRIBUTING.md`
and a README per package. pkg.go.dev, the signed tags and `AGENTS.md` say
what they would.
