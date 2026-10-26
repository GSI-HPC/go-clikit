---
title: display
description: Show spans as a live tree, a counter or plain lines on standard error, and leave a summary.
import: progress/display
weight: 20
next: ["/concepts/displays", "/packages/cliprogress", "/packages/progresstest"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`display` draws the events of a Bus on standard error. Most programs do
not call it directly: `cliprogress` picks a display from `--progress`.
Use `display` when your program picks by a rule of its own.

{{< cast display="tree" theme="tide" themes="all" class="short" caption="The tree, in each theme." >}}

## The displays

| Display | Make it with | It draws |
| --- | --- | --- |
| Tree | {{< api "progress/display" "NewTree" >}} | a few rows at the bottom of the terminal, redrawn |
| Counter | {{< api "progress/display" "NewCounter" >}} | one line at the bottom, redrawn |
| Plain | {{< api "progress/display" "NewPlain" >}} | one line for each change worth one |
| Summary | {{< api "progress/display" "NewSummary" >}} | one line once the command ended |

Each display shares the terminal with the command through a
{{< api "progress/display" "Terminal" >}}. The command's own lines go through
`Terminal.Writer`, and lines of other goroutines through `Terminal.Lines`.

{{< example "progress/display" "ExampleNewPlain" >}}

## Themes

The zero {{< api "progress/display" "Theme" >}} draws no colour. Five themes,
from {{< api "progress/display" "Themes" >}} or
{{< api "progress/display" "ParseTheme" >}}, add colours and marks.
{{< api "progress/display" "Theme.In" >}} draws a theme in 256 colours, in
16, or in none.

{{< example "progress/display" "ExampleParseTheme" >}}

## Fixed interfaces

- The text that the displays draw with the zero `Theme`. Programs compare
  it in their tests.
- The colours and the art of a theme can change in a minor release; its name
  and its words cannot ({{< decision 20 >}}).

## Read more

[Displays and themes](../../concepts/displays/) tells how the region, the
writers and the folding of node sets work.
