---
title: Interfaces
description: What a release keeps beyond the Go API, such as the event log, the texts of the displays and the words of --progress. Changing any of them is a breaking change.
weight: 10
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

The Go API is not the only thing that a program relies on. A test of a
program compares the text that a display draws. A script reads the JSON of a
doctor command. A view reads the event log that an older release wrote. So
the kit keeps these as interfaces too. Changing any of them is a breaking
change, and a new or stricter rule of `progresstest.Check` is one too.

| Interface | Package | Decision |
| --- | --- | --- |
| The six span kinds | `progress` | — |
| The JSONL event log, version 1: keys can be added, never renamed or removed | `progress`, `eventlog` | [the event log](../../concepts/event-log/) |
| The text that the displays draw with the zero `Theme` | `display` | — |
| The runes that are escaped can grow in a minor release; the forms of the escapes cannot | `termtext` | {{< decision 14 >}} |
| The colours and art of a theme can change in a minor release | `display` | {{< decision 20 >}} |
| The rules that `Check` enforces | `progresstest` | {{< decision 15 >}} |
| The words of `Modes`, in their order; the texts of the errors and notes, the refusals of the log file included | `cliprogress` | {{< decision 21 >}} |
| The texts of the errors of the checks of `Strict` | `cobratree` | {{< decision 24 >}} |
| The texts of `InvalidValue` and `FlagError` | `cobratree` | {{< decision 29 >}} |
| What `cliout` writes | `cliout` | {{< decision 30 >}} |
| The rate, the rule and the line of the notifications | `mcpprogress` | {{< decision 27 >}}, {{< decision 32 >}} |
| The rule of `ThemeFor`, and the names and texts of decision 31 | `cliprogress` | {{< decision 31 >}} |
| The refusals, headers, cookie and frames | `localweb` and its packages | {{< decision 34 >}} |
| The files, record, addresses, htmx headers, `htmx.Config` and the script versions | `launch`, `assets`, `page`, `htmx` | {{< decision 35 >}}, {{< decision 41 >}} |
| The name and mode of the lock file | `fsutil` | {{< decision 37 >}} |
| The status words, the JSON keys and the text of `Write` | `doctor` | {{< decision 38 >}} |
| The CRLF rule | `termtext` | {{< decision 42 >}} |

## Versions

- The kit uses semantic versions. At v0, a breaking change is allowed in a
  minor release, and the release notes name it.
- Adding API or a package is a minor release.
- A rename keeps a deprecated alias for one minor release. Its
  `Deprecated:` paragraph ends with "(decision 36)".
- The `go` line of `go.mod` is the oldest Go release that the Go project
  supports ({{< decision 2 >}}).
- Nothing in the tree names a version ({{< decision 4 >}}). A pushed tag is
  never moved or deleted.
