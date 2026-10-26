---
title: progress
description: Report work as spans in a context.Context, and write the JSONL event log. The package draws nothing.
import: progress
weight: 10
next: ["/concepts/spans", "/concepts/work", "/packages/display"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`progress` is the vocabulary that every other package of the kit speaks.
Work is a tree of spans: the command, its steps, the batches, the targets,
the calls and the waits. A span travels in the `context.Context` of the
work. Each change of a span is one event, sent in one total order to the
sinks of the Bus in the context.

{{< steps caps="The command attaches a Bus to its context and starts its span.|A library starts a step in the context it was given. It does not know the Bus.|The step announces its targets queued, then runs them.|Each change is one event. The Bus sends it to every sink in one order: a display, the event log, a test." >}}
<svg viewBox="0 0 820 230" role="img" aria-label="Spans in a context send events through the Bus to the sinks.">
<g font-size="13">
<g class="mono">
<rect x="20" y="20" width="180" height="40" rx="8" fill="var(--deck)" stroke="var(--isl-progress)" data-step="1"/><text x="34" y="45" fill="var(--fg)" data-step="1">command reinstall</text>
<rect x="50" y="80" width="200" height="40" rx="8" fill="var(--deck)" stroke="var(--isl-progress)" data-step="2"/><text x="64" y="105" fill="var(--fg)" data-step="2">step reinstalling…</text>
<g data-step="3"><rect x="80" y="140" width="70" height="30" rx="6" fill="var(--deck)" stroke="var(--ok)"/><text x="90" y="160" fill="var(--fg)">exe01</text>
<rect x="160" y="140" width="70" height="30" rx="6" fill="var(--deck)" stroke="var(--run)"/><text x="170" y="160" fill="var(--fg)">exe02</text>
<rect x="240" y="140" width="70" height="30" rx="6" fill="var(--deck)" stroke="var(--mute)" stroke-dasharray="4 3"/><text x="250" y="160" fill="var(--mute)">exe03</text></g>
<g data-step="4">
<rect x="400" y="80" width="110" height="50" rx="10" fill="var(--deck)" stroke="var(--hi)"/><text x="432" y="110" fill="var(--hi)">Bus</text>
<rect x="600" y="20" width="200" height="40" rx="8" fill="var(--deck)" stroke="var(--line)"/><text x="614" y="45" fill="var(--text)">display.Tree</text>
<rect x="600" y="85" width="200" height="40" rx="8" fill="var(--deck)" stroke="var(--line)"/><text x="614" y="110" fill="var(--text)">progress.Log</text>
<rect x="600" y="150" width="200" height="40" rx="8" fill="var(--deck)" stroke="var(--line)"/><text x="614" y="175" fill="var(--text)">progresstest.Capture</text>
<g stroke="var(--mute)" stroke-width="1.5"><line x1="320" y1="140" x2="398" y2="110"/><line x1="512" y1="100" x2="598" y2="40"/><line x1="512" y1="105" x2="598" y2="105"/><line x1="512" y1="110" x2="598" y2="170"/></g>
</g>
</g>
</g>
</svg>
{{< /steps >}}

## Use it

A library takes a context, and starts spans in it with
{{< api "progress" "Start" >}}. Without a Bus, `Start` returns a nil span
whose methods do nothing, at no cost. A command attaches a Bus with
{{< api "progress" "WithBus" >}}, or lets `cliprogress` do it.

{{< example "progress" "Example" >}}

## The main names

| Name | What it is |
| --- | --- |
| {{< api "progress" "Start" >}}, {{< api "progress" "Span" >}} | a span: `Run`, `Update`, `Advance`, `SetAmount`, `Skip`, `End` |
| {{< api "progress" "Kind" >}} | command, step, batch, target, call, wait |
| {{< api "progress" "Bus" >}}, {{< api "progress" "BusOptions" >}} | sends the events, in one order, to the sinks |
| {{< api "progress" "Event" >}}, {{< api "progress" "Fields" >}} | one change of a span, as plain data; the closed vocabulary |
| {{< api "progress" "Sink" >}} | what a display, a log or a test is |
| {{< api "progress" "NewLog" >}}, {{< api "progress" "LogOptions" >}} | the JSONL event log |
| {{< api "progress" "Meter" >}}, {{< api "progress" "Reading" >}} | the work of each span, rolled up the tree |
| {{< api "progress" "Classify" >}}, {{< api "progress" "Skip" >}} | the class of an error; a target left out on purpose |
| {{< api "progress" "Suspend" >}} | takes every display off the terminal for a question |
| {{< api "progress" "ParseTraceContext" >}} | continues the W3C trace of another program |

## Fixed interfaces

- The six span kinds.
- The JSONL event log, version 1: keys can be added, and are never renamed
  or removed. See [the event log](../../concepts/event-log/).
- No allocation without a Bus. A test of the kit keeps it so.

## Read more

[Spans and the Bus](../../concepts/spans/) and
[Work and the Meter](../../concepts/work/) explain the model.
