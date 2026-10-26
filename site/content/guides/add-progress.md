---
title: Add progress to a command
description: Give a command the --progress flag, a display on standard error, an event log and a summary, with cliprogress.
weight: 10
next: ["/guides/library", "/guides/many-hosts", "/packages/cliprogress"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A command that runs for more than a second shows how far it got. This
guide adds the flag `--progress`, the displays, the event log and the
summary to the `main` of a program.

{{< cast display="tree" theme="tide" themes="all" class="short" caption="What the user gets with --progress=tty." >}}

## Procedure

1. Read the flag and the variable that stands in for it, such as
   `CLUSTERCTL_PROGRESS`. Put them in a {{< api "progress/cliprogress" "Setting" >}}.
2. Ask the system about standard error:
   {{< api "progress/cliprogress" "IsTerminal" >}},
   {{< api "progress/cliprogress" "TerminalSize" >}},
   {{< api "progress/cliprogress" "InForeground" >}} and
   {{< api "progress/cliprogress" "UTF8Locale" >}}.
3. Pick the theme, and let {{< api "progress/cliprogress" "ThemeFor" >}}
   take out the colours that `NO_COLOR` and `TERM` forbid.
4. Call {{< api "progress/cliprogress" "Start" >}} with a context that ends
   on the interrupt. An error is a usage error: report it as such.
5. Put the writer of your logger through
   {{< api "progress/cliprogress" "Session.Lines" >}}, and your own writes to
   the terminal through {{< api "progress/cliprogress" "Session.Writer" >}}.
6. Start the command's span in {{< api "progress/cliprogress" "Session.Context" >}},
   run the work, and end the span with the error.
7. Call {{< api "progress/cliprogress" "Session.Finish" >}}. Then put your
   writers back, and write the error.

{{< example "internal/docsite" "Example_mainWithProgress" >}}

{{% call caution %}}
End the command's span before you call `Finish`. `Finish` closes the Bus,
and a span that is open then ends as canceled.
{{% /call %}}

## What the user gets

| `--progress` | Standard error shows |
| --- | --- |
| `auto` (the default) | the tree on a terminal, nothing elsewhere |
| `tty` | the tree, or the counter on a terminal too small for it |
| `counter` | one line, redrawn |
| `plain` | one line for each change worth one, also in a log |
| `none` | what it shows without the kit, byte for byte |

The words, and their order, are an interface (decision 21). The texts of
the errors and notes that `cliprogress` writes are an interface too.

{{< example "progress/cliprogress" "ExampleStart" >}}

## Ask a question

A question waits for its answer with no display drawn over it.
{{< api "progress/cliprogress" "Ask" >}} takes the displays off the terminal
while it waits, and {{< api "progress/cliprogress" "Yes" >}} reads the
answer.

{{< example "progress/cliprogress" "ExampleAsk" >}}
