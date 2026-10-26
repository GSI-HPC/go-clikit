---
title: cliprogress
description: Turn the flags of a command into a display, a summary and an event log, and take them down in order.
import: progress/cliprogress
weight: 30
next: ["/guides/add-progress", "/packages/display"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`cliprogress` is the command line's side of progress. It has the words of
`--progress`, the rule that picks a display from them and from what the
program found out about its streams, and a
{{< api "progress/cliprogress" "Session" >}}, which makes the Bus, the
display, the summary and the event log of one command, and takes them down
in order.

```text
--progress auto|tty|counter|plain|none
--progress-log FILE
```

{{< fig caption="How auto picks: the tree only where a person watches a terminal." >}}
<svg viewBox="0 0 820 200" role="img" aria-label="auto draws the tree when standard error is a terminal, not dumb, and standard output goes into no pipe; nothing otherwise.">
<g font-size="13" class="mono">
<rect x="10" y="70" width="110" height="44" rx="8" fill="var(--deck)" stroke="var(--isl-progress)"/><text x="40" y="97" fill="var(--fg)">auto</text>
<rect x="160" y="70" width="170" height="44" rx="8" fill="var(--deck)" stroke="var(--line)"/><text x="172" y="97" fill="var(--text)">stderr a terminal?</text>
<rect x="370" y="70" width="140" height="44" rx="8" fill="var(--deck)" stroke="var(--line)"/><text x="382" y="97" fill="var(--text)">TERM not dumb?</text>
<rect x="550" y="70" width="150" height="44" rx="8" fill="var(--deck)" stroke="var(--line)"/><text x="562" y="97" fill="var(--text)">stdout no pipe?</text>
<rect x="730" y="70" width="80" height="44" rx="8" fill="var(--run)"/><text x="748" y="97" fill="var(--run-ink)">tree</text>
<rect x="370" y="150" width="140" height="40" rx="8" fill="var(--deck)" stroke="var(--mute)" stroke-dasharray="4 3"/><text x="400" y="175" fill="var(--mute)">nothing</text>
<g stroke="var(--mute)" stroke-width="1.5" fill="none">
<line x1="122" y1="92" x2="158" y2="92"/><line x1="332" y1="92" x2="368" y2="92"/><line x1="512" y1="92" x2="548" y2="92"/><line x1="702" y1="92" x2="728" y2="92"/>
<path d="M245 116 V170 H368"/><path d="M440 116 V148"/><path d="M625 116 V170 H512"/>
</g>
<g fill="var(--mute)" font-size="11"><text x="250" y="140">no</text><text x="446" y="136">no</text><text x="630" y="140">no</text></g>
</g>
</svg>
{{< /fig >}}

## Use it

The program reads its flag and its variable, asks the system about its
streams, and passes what it found in {{< api "progress/cliprogress" "Options" >}}.
The package reads no environment.

{{< example "progress/cliprogress" "ExampleStart" >}}

| Name | What it does |
| --- | --- |
| {{< api "progress/cliprogress" "Start" >}} | writes the notes, picks the display, opens the log, makes the Bus |
| {{< api "progress/cliprogress" "Session.Writer" >}}, {{< api "progress/cliprogress" "Session.Lines" >}} | the writers that go through the display |
| {{< api "progress/cliprogress" "Session.Finish" >}} | closes the Bus, takes the display off, closes the log, writes the summary |
| {{< api "progress/cliprogress" "Choose" >}} | the rule alone, for a program that draws by itself |
| {{< api "progress/cliprogress" "IsTerminal" >}}, {{< api "progress/cliprogress" "TerminalSize" >}}, {{< api "progress/cliprogress" "InForeground" >}}, {{< api "progress/cliprogress" "UTF8Locale" >}} | what the program asks the system |
| {{< api "progress/cliprogress" "ThemeFor" >}} | the colours that `NO_COLOR`, `TERM` and `COLORTERM` allow |
| {{< api "progress/cliprogress" "Ask" >}}, {{< api "progress/cliprogress" "Yes" >}} | a question, with no display drawn over it |

{{% call caution %}}
The flag refuses what it cannot have: `--progress tty` without a terminal
is a usage error. The variable fails no command, because a profile sets it
once for every script. It shows nothing there, and writes a note only for a
word that it does not take.
{{% /call %}}

## The event log file

The log is appended to a file that only the user can read, mode 0600. A
file that is there already must be the user's, and nobody else may read or
write it. `Options.DefaultLog` keeps a log unasked, and `Options.RotateAt`
moves it aside before it grows without end ({{< decision 31 >}}).

## Fixed interfaces

- The words that {{< api "progress/cliprogress" "Modes" >}} names, in their
  order.
- The texts of the errors and notes that the package returns and writes,
  the refusals of the log file included ({{< decision 21 >}}).
- The rule of `ThemeFor`.
