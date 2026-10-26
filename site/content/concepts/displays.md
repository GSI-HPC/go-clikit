---
title: Displays and themes
description: The command shows its spans as a live tree, a counter or plain lines, and leaves a summary. Each display shares the terminal with the command.
weight: 30
next: ["/packages/display", "/packages/cliprogress", "/guides/add-progress"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

The library sends events. The command picks how to show them. The kit has
three displays and a summary, in `progress/display`. Most programs let
`progress/cliprogress` pick one from the words of `--progress`.

| Display | For | Shows |
| --- | --- | --- |
| Tree | a person at a terminal | the work under way and what of it failed, in a few rows at the bottom |
| Counter | a small terminal, or a person who wants less | one line: how far the work got |
| Plain | a log, such as a CI job | one line for each change worth a line, with the time |
| Summary | everyone | one line once the command ended: how it ended, how long it ran, how many ended how |

{{< cast display="tree" theme="tide" themes="all" caption="The tree. Twelve nodes, four at a time; exe07 fails. Pick a theme." >}}

{{< cast display="counter" theme="tide" themes="none,tide" class="short" title="reinstall --progress=counter" caption="The counter: one line." >}}

{{< cast display="plain" theme="none" themes="none,tide" class="short" title="reinstall --progress=plain" caption="Plain lines, with the summary as the last line." >}}

## The terminal is shared

The tree and the counter own a few rows at the bottom of the terminal, the
region, and nothing else. The command still writes to the terminal, so the
two must take turns. A {{< api "progress/display" "Terminal" >}} does this:

- The command writes through `Terminal.Writer`. It takes the region off
  first, and lets it back only once a line has ended. So a question that
  waits for its answer is never drawn over.
- Goroutines beside the command, such as a logger, write through
  `Terminal.Lines`. It holds their lines, whole lines only, while a
  question is asked or the command has a line open.
- {{< api "progress" "Suspend" >}} takes every display off the terminal, for
  a question, and returns once they are off.

{{< fig caption="The region at the bottom, and what scrolls above it. The command's own lines and the lines a display leaves go above the region." >}}
<svg viewBox="0 0 760 240" role="img" aria-label="A terminal: lines of the command scroll above; the region of the display stays at the bottom.">
<g class="mono" font-size="13">
<rect x="10" y="10" width="740" height="220" rx="10" fill="#181926" stroke="var(--line)"/>
<text x="30" y="40" fill="#a5adcb">$ clusterctl reinstall exe[01-12]</text>
<text x="30" y="64" fill="#d0d0d0">note: exe03 has no BMC; using IPMI</text>
<text x="30" y="88" fill="#d0d0d0">✓ fetching the base image  5.0s  2.0 GiB</text>
<line x1="20" y1="104" x2="740" y2="104" stroke="#00af87" stroke-dasharray="6 5"/>
<text x="560" y="98" fill="#00af87" font-size="11">↑ lines above the region</text>
<text x="30" y="130" fill="#d0d0d0" font-weight="700">reinstall ◦ 0:08.7</text>
<text x="30" y="154" fill="#d0d0d0">╎ reinstalling the nodes  4/12 ◦ 4 running</text>
<text x="30" y="178" fill="#d0d0d0">  ╎ ◞ exe05  1.1s/30s  POST /redfish/v1/Systems/1</text>
<text x="30" y="202" fill="#d0d0d0">  ╎ ✓ exe[01-04]</text>
<text x="560" y="130" fill="#00af87" font-size="11">the region, redrawn</text>
</g>
</svg>
{{< /fig >}}

A display never hides the cursor. A process that is killed while it draws
leaves a terminal that works, at worst with the rows of its last frame.

## Many targets

A step over 480 hosts cannot show 480 rows. The tree folds the targets that
ended the same way into node sets, such as `exe[0001-0480]`, with
[go-nodeset](https://github.com/GSI-HPC/go-nodeset). It folds a large set
again once a second (decision 16), and its cost per frame does not grow with
the number of targets. A test of the kit holds it to that.

## Themes

The zero {{< api "progress/display" "Theme" >}} draws plain text with no
escape code. That text is an interface: programs compare it in their tests.
Five themes add colours and marks: Classic, Aurora, Ember, Neon and Tide.

- A theme only adds to what a display says. Names, errors and output lines
  stay in the terminal's own colour.
- Every glyph of a theme takes one column, in every locale.
- {{< api "progress/display" "Theme.In" >}} draws a theme in 256 colours,
  in the terminal's 16, or in none, with its marks alone.
- The colours and the art of a theme can change in a minor release. Its
  name and its words cannot (decision 20).

{{% call note %}}
The kit reads no environment. The program decides if it draws in colour,
from `NO_COLOR`, `TERM`, `COLORTERM` and whether standard error is a
terminal. {{< api "progress/cliprogress" "ThemeFor" >}} applies the usual
rule to the values that the program read.
{{% /call %}}

## The summary and standard error

Once the command ended, the display leaves one line, and the program writes
the error. The programs that use the kit share one convention for standard
error:

- **Standard output** holds the result, and nothing of the progress.
- **Standard error** holds the progress, the notes of the command, the lines
  of its logger, then the summary and one line for the error, escaped.
- **Without a display**, standard error holds exactly what it would hold
  without the kit, byte for byte. A command that is done in its first second
  draws nothing.
