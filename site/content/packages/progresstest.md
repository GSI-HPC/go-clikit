---
title: progresstest
description: Capture the events that a command reports, draw them as a tree that does not depend on the order of the work, and check every promise.
import: progress/progresstest
weight: 40
next: ["/guides/test-progress", "/packages/progress"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`progresstest` captures the events of a test, draws them as a tree that
reads the same however the concurrent work was scheduled, and checks that
they keep the promises of `progress` to every sink.

{{< example "progress/progresstest" "ExampleWatch" >}}

## The main names

| Name | What it does |
| --- | --- |
| {{< api "progress/progresstest" "Watch" >}}, {{< api "progress/progresstest" "Watcher" >}} | a context with a Bus of its own; `Finish` checks and returns the tree |
| {{< api "progress/progresstest" "Capture" >}} | a sink that keeps the events; `Tree` draws them |
| {{< api "progress/progresstest" "Check" >}} | the rules, on events that you have |
| {{< api "progress/progresstest" "Screen" >}} | a terminal in memory, for a test of what a display draws |

## What Check holds an emitter to

Three rules shape how parallel work is reported. A pool written by hand
breaks them most often:

1. Every target of a folded step or a batch is announced queued before the
   first of them runs. A pool that starts each target in its own goroutine,
   as `errgroup.Go` does, breaks this.
2. When a folded step or a batch ends, the targets that ended under it are
   its `Total`. A count never passes its `Total`, and a `Total` never
   shrinks.
3. No more targets run at once below a span than its `Limit`.

The work of a span has rules too: two advances of a span come at least
100 ms apart, an amount is never negative, and the unit of a span never
changes.

{{% call note %}}
The rules are an interface ({{< decision 15 >}}). A new or stricter rule is
a breaking change.
{{% /call %}}

{{< example "progress/progresstest" "ExampleScreen_Styled" >}}
