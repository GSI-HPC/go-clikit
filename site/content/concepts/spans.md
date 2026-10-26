---
title: Spans and the Bus
description: A library reports its work as a tree of spans in its context. The Bus gives each change to every sink, in one order.
weight: 10
next: ["/concepts/work", "/packages/progress", "/guides/library"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A command that works on 480 hosts must show how far it got. The code that
does the work is often a library, which other programs use too. So the
library must not draw. It reports, and the command decides what to show.
The kit does this with spans and a Bus.

## The tree of spans

A span is one unit of work. Work is a tree of spans, with one level for each
kind of unit. The six kinds are an interface of the kit: they do not change.

{{< fig caption="The six kinds of span. Each kind goes in the kind above it. A batch is optional: only a staggered action has batches." >}}
<svg viewBox="0 0 760 250" role="img" aria-label="A command holds steps. A step holds batches or targets. A target holds calls and waits.">
<g class="mono" font-size="14">
<rect x="10" y="10" width="740" height="230" rx="12" fill="none" stroke="var(--mute)"/>
<text x="26" y="34" fill="var(--hi)" font-weight="600">command</text><text x="120" y="34" fill="var(--mute)">reinstall</text>
<rect x="30" y="46" width="700" height="182" rx="10" fill="none" stroke="var(--mute)"/>
<text x="46" y="70" fill="var(--hi)" font-weight="600">step</text><text x="120" y="70" fill="var(--mute)">reinstalling the nodes</text>
<rect x="50" y="82" width="660" height="134" rx="10" fill="none" stroke="var(--guide)" stroke-dasharray="5 4"/>
<text x="66" y="106" fill="var(--hi)" font-weight="600">batch</text><text x="140" y="106" fill="var(--mute)">rack 3 of 8 (optional)</text>
<rect x="70" y="118" width="620" height="86" rx="10" fill="var(--ink)" stroke="var(--run)"/>
<text x="86" y="142" fill="var(--hi)" font-weight="600">target</text><text x="160" y="142" fill="var(--mute)">exe07</text>
<rect x="90" y="154" width="290" height="38" rx="8" fill="none" stroke="var(--mute)"/>
<text x="104" y="178" fill="var(--hi)" font-weight="600">call</text><text x="150" y="178" fill="var(--mute)">POST /redfish/v1/…</text>
<rect x="392" y="154" width="284" height="38" rx="8" fill="none" stroke="var(--mute)"/>
<text x="406" y="178" fill="var(--hi)" font-weight="600">wait</text><text x="452" y="178" fill="var(--mute)">for the BMC to answer</text>
</g>
</svg>
{{< /fig >}}

A span goes in the `context.Context` that the work is done under. When the
code for a target makes a call, the call starts its span from that context.
So the call goes under its target, and nobody gives the target to the call
as a parameter. A library reports its work with no new parameter in its API.

Start a span with {{< api "progress" "Start" >}}. It gives back a new
context, for the work below the span, and the span. End the span with
{{< api "progress" "Span.End" >}} and the error of the work.

## The Bus

Each change to a span is one {{< api "progress" "Event" >}} of plain data:
a start, a run, an update, a line of output, an advance of its work, or an
end. The {{< api "progress" "Bus" >}} gives each event a number, `Seq`,
cleans its text, and gives it to each sink, in the order of the list of
sinks. Every sink sees the same events in the same order.

{{< steps caps="The library starts a span for exe07. The event goes to the Bus.|The Bus gives the event the next number. All sinks see this order.|The Bus cleans each text with Sanitize. No sink must clean it again.|The Bus gives the event to each sink, under its lock. A sink keeps the event and returns. It does not block.|The tree draws on its own goroutine, ten times a second. The log writes on its own goroutine. The Bus is free for the next event." >}}
<svg viewBox="0 0 900 260" role="img" aria-label="A library sends an event to the Bus. The Bus numbers it, cleans its text and gives it to the tree, the event log and a test.">
<defs><marker id="sb-a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L10,5 L0,10 z" fill="var(--mute)"/></marker></defs>
<g font-size="14" fill="var(--fg)">
<rect x="20" y="95" width="150" height="70" rx="10" fill="var(--ink)" stroke="var(--line)" stroke-width="1.5"/>
<text x="95" y="125" text-anchor="middle" font-weight="700">library</text>
<text x="95" y="146" text-anchor="middle" font-size="12.5" fill="var(--mute)" class="mono">progress.Start</text>
<rect x="260" y="70" width="230" height="120" rx="12" fill="var(--ink)" stroke="var(--run)" stroke-width="1.5"/>
<text x="375" y="98" text-anchor="middle" font-weight="700" fill="var(--run)">Bus</text>
<text x="375" y="126" text-anchor="middle" font-size="13" class="mono" data-only="1">Seq 41</text>
<text x="375" y="126" text-anchor="middle" font-size="13" class="mono" data-step="2" fill="var(--hi)">Seq 42</text>
<text x="375" y="152" text-anchor="middle" font-size="12.5" fill="var(--mute)" data-only="1">waits for an event</text>
<text x="375" y="152" text-anchor="middle" font-size="12.5" fill="var(--mute)" data-only="2">numbers the event</text>
<text x="375" y="152" text-anchor="middle" font-size="12.5" fill="var(--mute)" data-only="3">escapes and cuts each text</text>
<text x="375" y="152" text-anchor="middle" font-size="12.5" fill="var(--mute)" data-only="4">calls each sink, in order</text>
<text x="375" y="152" text-anchor="middle" font-size="12.5" fill="var(--mute)" data-only="5">free for the next event</text>
<rect x="600" y="20" width="280" height="56" rx="10" fill="var(--ink)" stroke="var(--line)" stroke-width="1.5"/>
<text x="618" y="45" font-weight="700">Tree</text><text x="618" y="64" font-size="12" fill="var(--mute)">keeps it; draws ten times a second</text>
<rect x="600" y="102" width="280" height="56" rx="10" fill="var(--ink)" stroke="var(--line)" stroke-width="1.5"/>
<text x="618" y="127" font-weight="700">Event log</text><text x="618" y="146" font-size="12" fill="var(--mute)">queues one JSONL line</text>
<rect x="600" y="184" width="280" height="56" rx="10" fill="var(--ink)" stroke="var(--line)" stroke-width="1.5"/>
<text x="618" y="209" font-weight="700">Capture</text><text x="618" y="228" font-size="12" fill="var(--mute)">a test keeps every event</text>
<line x1="170" y1="130" x2="256" y2="130" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#sb-a)"/>
<line x1="490" y1="110" x2="596" y2="50" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#sb-a)"/>
<line x1="490" y1="130" x2="596" y2="130" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#sb-a)"/>
<line x1="490" y1="150" x2="596" y2="210" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#sb-a)"/>
<g data-at="95,60|375,205|375,205|545,130|836,89" style="transform: translate(836px, 89px)"><rect x="-34" y="-12" width="68" height="24" rx="12" fill="var(--isl-web)"/><text x="0" y="4.5" text-anchor="middle" font-size="12" font-weight="700" fill="var(--ink)" class="mono">event</text></g>
</g>
</svg>
{{< /steps >}}

A sink is anything with a `Handle` method: a display, the event log, an
MCP notifier, a test. Three rules keep the order safe:

- **A sink does not block.** The Bus calls `Handle` with its lock held. A
  sink updates its memory and returns. It does not write to a terminal or a
  file there, and it does not call the Bus.
- **A sink that panics comes off the Bus.** Its stack goes to
  `BusOptions.PanicLog`, and the work goes on.
- **Displays draw on their own time.** A display draws from what `Handle`
  kept, on a goroutine of its own. The event log writes from its own
  goroutine too.

{{% call caution %}}
Do not write to a terminal, a file or a network in `Handle`. The Bus holds
its lock while it calls the sink, so a slow sink stops all the work that
reports to the Bus.
{{% /call %}}

## How a span ends

`End` takes the error of the work, and the error decides the status:

{{< fig caption="The life of a span. Only a target that a pool announced starts queued. The class says in one word why a span failed." >}}
<svg viewBox="0 0 820 230" role="img" aria-label="A span goes from queued to running to ended. It ends ok, failed, canceled or skipped.">
<defs><marker id="st-a" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L10,5 L0,10 z" fill="var(--mute)"/></marker></defs>
<g font-size="14" fill="var(--fg)">
<rect x="20" y="90" width="120" height="46" rx="23" fill="var(--ink)" stroke="var(--mute)" stroke-dasharray="5 4"/><text x="80" y="118" text-anchor="middle" class="mono">queued</text>
<rect x="210" y="90" width="120" height="46" rx="23" fill="var(--ink)" stroke="var(--run)"/><text x="270" y="118" text-anchor="middle" class="mono">running</text>
<line x1="140" y1="113" x2="206" y2="113" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#st-a)"/><text x="173" y="104" text-anchor="middle" font-size="12" fill="var(--mute)" class="mono">Run</text>
<line x1="330" y1="105" x2="476" y2="34" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#st-a)"/>
<line x1="330" y1="110" x2="476" y2="88" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#st-a)"/>
<line x1="330" y1="116" x2="476" y2="142" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#st-a)"/>
<line x1="330" y1="121" x2="476" y2="196" stroke="var(--mute)" stroke-width="1.5" marker-end="url(#st-a)"/>
<text x="380" y="58" font-size="12" fill="var(--mute)" class="mono">End</text>
<rect x="480" y="14" width="110" height="38" rx="8" fill="var(--ink)" stroke="var(--ok)"/><text x="535" y="38" text-anchor="middle" fill="var(--ok)" class="mono">ok</text>
<rect x="480" y="68" width="110" height="38" rx="8" fill="var(--ink)" stroke="var(--bad)"/><text x="535" y="92" text-anchor="middle" fill="var(--bad)" class="mono">failed</text>
<rect x="480" y="122" width="110" height="38" rx="8" fill="var(--ink)" stroke="var(--isl-command)"/><text x="535" y="146" text-anchor="middle" fill="var(--isl-command)" class="mono">canceled</text>
<rect x="480" y="176" width="110" height="38" rx="8" fill="var(--ink)" stroke="var(--mute)"/><text x="535" y="200" text-anchor="middle" fill="var(--mute)" class="mono">skipped</text>
<g font-size="13" fill="var(--text)"><text x="610" y="38">a nil error</text><text x="610" y="92">an error; Classify tells its class</text><text x="610" y="146">an interrupt or a deadline</text><text x="610" y="200">an error that is ErrSkipped</text></g>
</g>
</svg>
{{< /fig >}}

- A nil error ends the span **ok**.
- An error that is {{< api "progress" "ErrSkipped" >}}, as `errors.Is`
  tells, ends it **skipped**: work left out on purpose, such as a call that
  a dry run does not make. {{< api "progress" "Skip" >}} makes such an error
  with a reason. A rule of the program cannot turn a skip into a failure.
- Any other error ends it **failed** or **canceled**. The class, such as
  `target`, `transport` or `timeout`, comes from
  {{< api "progress" "Classify" >}}, and from the rule the program gives the
  Bus for errors that tell no class.

`errors.Is` looks through every error that an error joins. So an error that
joins a skip and a failure ends the span skipped. Work that gathers the
errors of its parts returns a skip only when every part was skipped.

## No listener, no cost

Without a Bus in the context, `Start` gives back the same context and a nil
`*Span`. Every method of a nil span does nothing. This path allocates
nothing, and two tests hold it to that. So a library can report all its
work, and a program that does not listen pays only for one lookup in the
context.

## A closed vocabulary

An event carries the fields of {{< api "progress" "Fields" >}} and nothing
else. There is no field for an argument vector, a script, standard input,
the environment or a header. So a secret in one of these cannot reach a
display or the log. Each text in an event goes through
{{< api "progress" "Sanitize" >}} first, which escapes it with `termtext`
and cuts it to its bound.

## Runs and traces

The events of one Bus are one run, and they map one to one onto a W3C
trace: the trace ID has 16 bytes and a span ID 8. A command that starts a
child can give it the trace context of the span that the child works for,
in `TRACEPARENT`. The child's log then names that span as its parent, and
{{< api "progress/eventlog" "NewForwarder" >}} can put the child's events
under it. See [Show the progress of a child process](../../guides/child-process/).
