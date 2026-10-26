---
title: Test what a command reports
description: Check the spans a command reports, and the text a display draws, in a Go test, with progresstest.
weight: 40
next: ["/packages/progresstest", "/concepts/spans"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

The progress of a command is part of what it does. A test can check it as
it checks the result: which spans started, how each ended, and that the
events keep every promise of the vocabulary.

## Procedure

1. Call {{< api "progress/progresstest" "Watch" >}} with the context of the
   test. It returns a context with a Bus, and a
   {{< api "progress/progresstest" "Watcher" >}}.
2. Run the code under test with that context.
3. Compare the tree of the Watcher with the tree you expect. Targets that
   read the same fold into one line, so the tree is the same however the
   work was scheduled.
4. Call `Finish`. It checks every rule of the events. A test that does not
   call it is checked when it ends.

{{< example "progress/progresstest" "ExampleWatch" >}}

{{% call note %}}
The rules that {{< api "progress/progresstest" "Check" >}} enforces are an
interface (decision 15). A new or stricter rule is a breaking change, so a
test that passes today does not fail after a minor release for a rule.
{{% /call %}}

## Test what a display draws

A {{< api "progress/progresstest" "Screen" >}} is a terminal in memory. It
applies the cursor moves and erases that a display writes, and shows what a
person would see. With `Styles` set, it records the colours of each run of
text too.

{{< example "progress/progresstest" "ExampleScreen" >}}

The recordings on this site come from a Screen. A test of the site checks
them against the displays, so they show what the code draws.
