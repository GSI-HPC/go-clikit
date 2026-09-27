<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# go-clikit

[![Go Reference](https://pkg.go.dev/badge/github.com/GSI-HPC/go-clikit.svg)](https://pkg.go.dev/github.com/GSI-HPC/go-clikit)
[![CI](https://github.com/GSI-HPC/go-clikit/actions/workflows/ci.yml/badge.svg)](https://github.com/GSI-HPC/go-clikit/actions/workflows/ci.yml)

**The runtime GSI-HPC's command-line tools share.** Report progress as spans
carried in a `context.Context`, and let the command choose how to show it: a
live tree on a terminal, a counter, plain lines for a CI log, or a JSONL
event log for programs. Run work in bounded pools that report their targets.
Escape untrusted text before it reaches a terminal. A library that reports
spans costs nothing when no command is listening.

**[API reference on pkg.go.dev →](https://pkg.go.dev/github.com/GSI-HPC/go-clikit)**

## Status

The packages were written for [clusterctl](https://github.com/GSI-HPC/clusterctl), a
command-line tool for administering HPC clusters, and became a module of
their own when [sind](https://github.com/GSI-HPC/sind), which runs Slurm
clusters in Docker, set out to use them too. They move here, with their
history, once they have shipped in a clusterctl release and their API has
held still for a while. Until then the module holds no API.

| Package | What it holds |
| --- | --- |
| `termtext` | escaping untrusted text for a terminal, and display width |
| `progress` | spans in a `context.Context`, the JSONL event log, `TRACEPARENT` |
| `progress/display` | the live tree, the counter, plain lines, the summary |
| `progress/progresstest` | capturing and checking the events a command reports |
| `fanout` | bounded worker pools that report their targets as progress |

## Install

```console
$ go get github.com/GSI-HPC/go-clikit/progress
```

It needs Go 1.26 or newer: the module requires the oldest Go release the Go
project still supports, and follows it up after each Go release
([decision 2](doc/decisions.md#2-the-go-line-is-the-oldest-go-release-still-supported)). It requires few modules,
and never cobra outside a package about cobra
([decision 3](doc/decisions.md#3-what-the-kit-may-require)).

## Versions

Releases are signed tags `vX.Y.Z`, and each has
[release notes](https://github.com/GSI-HPC/go-clikit/releases). The module
is at v0 while its API settles, so a minor release may change it; its notes
say how ([decision 4](doc/decisions.md#4-a-release-is-a-signed-tag)).

## Contributing

The repository carries a `mise.toml`, so the toolchain comes from
[mise](https://mise.jdx.dev) if you use it:

```console
$ mise install    # Go and golangci-lint, at the versions CI uses
$ make lint       # golangci-lint
$ make test       # tests under the race detector
$ make cover      # every file at 100% coverage
$ make help       # every other check CI runs
```

Commits are [Conventional Commits](https://www.conventionalcommits.org/).
How the module is built and why is in [`doc/`](doc/), and a change that
takes a decision adds it to [`doc/decisions.md`](doc/decisions.md).
Instructions for coding agents are in
[`AGENTS.md`](AGENTS.md). Report vulnerabilities as
[`SECURITY.md`](SECURITY.md) says.

## AI disclosure

This project is developed with the help of AI coding tools. Changes written
by Anthropic's Claude Code agent are committed as
`Claude <noreply@anthropic.com>` and/or carry a `Co-Authored-By: Claude …`
trailer.

## Licence

Copyright 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH
<http://www.gsi.de>

Apache-2.0. See [`LICENSE`](LICENSE).
