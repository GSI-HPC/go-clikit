---
title: Report progress from a library
description: A library reports its work as spans in the context it is given. It draws nothing, and costs nothing when no command listens.
weight: 20
next: ["/concepts/spans", "/guides/many-hosts", "/packages/progress"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A library does not know if a person, a log or nobody watches it. So it
draws nothing. It starts spans in the context it was given, and the command
that called it decides what is shown.

## Procedure

1. Take a `context.Context` as the first argument of each function that
   does work worth showing.
2. Start a span for each step with {{< api "progress" "Start" >}}, and use
   the context that it returns for the work below.
3. Announce the targets of a step as queued before the first runs, so that a
   display knows all of the work from the start.
4. Call {{< api "progress" "Span.Run" >}} when a target starts, and
   {{< api "progress" "Span.End" >}} with its error when it ends.
5. End each span that you started, on every path, also on an error.

{{< example "progress" "reset" >}}

The command attaches a Bus to the context and ends the spans that it started.
The library's spans show up below them:

{{< example "progress" "Example" >}}

## Without a Bus, nothing

When no command attached a Bus, `Start` returns the context as it is and a
nil span. The methods of a nil span do nothing. The library makes no
allocation on this path, and a test of the kit keeps it so.

{{< example "progress" "ExampleStart_withoutABus" >}}

{{% call note %}}
A library imports `progress`, and never `display` nor `cliprogress`.
`progress` has no dependency on a terminal, on cobra or on `net/http`.
{{% /call %}}

## Skip and classify

- A target that is left out on purpose ends with {{< api "progress" "Skip" >}}
  or an error of {{< api "progress" "ErrSkipped" >}}. It is not counted as
  failed.
- The class of an error says what kind of failure it is: of the target, of
  the transport, a timeout, a refused login, a usage error, a cancel.
  {{< api "progress" "Classify" >}} finds it, and a program adds a rule of
  its own for errors that say none.

{{< example "progress" "ExampleClassify" >}}
