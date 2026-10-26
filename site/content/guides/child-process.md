---
title: Follow a child process
description: Hand a child program the trace of your span, and read its event log to show or forward its progress.
weight: 90
next: ["/concepts/event-log", "/packages/eventlog", "/packages/mcpprogress"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A program can run another program that uses the kit, such as its own
command line from a page of its web UI, or a tool call run as a child. The
child writes its own event log. The parent reads it, and the two logs name
one trace.

{{< fig caption="The parent hands the child its trace, and reads the child's event log." >}}
<svg viewBox="0 0 820 220" role="img" aria-label="Parent passes TRACEPARENT to child; child writes JSONL; parent tails the log into a Reducer or Forwarder.">
<g font-size="13">
<rect x="10" y="20" width="250" height="180" rx="12" fill="var(--deck)" stroke="var(--isl-progress)"/>
<text x="30" y="48" font-weight="700" fill="var(--isl-progress)">parent</text>
<text x="30" y="78" class="mono" fill="var(--text)">span.TraceContext()</text>
<text x="30" y="150" class="mono" fill="var(--text)">eventlog.NewTail(path,</text>
<text x="30" y="170" class="mono" fill="var(--text)">  Reducer | Forwarder)</text>
<rect x="560" y="20" width="250" height="180" rx="12" fill="var(--deck)" stroke="var(--isl-progress)"/>
<text x="580" y="48" font-weight="700" fill="var(--isl-progress)">child</text>
<text x="580" y="78" class="mono" fill="var(--text)">ParseTraceContext(…)</text>
<text x="580" y="150" class="mono" fill="var(--text)">progress.NewLog(f, …)</text>
<rect x="330" y="120" width="160" height="60" rx="8" fill="var(--well)" stroke="var(--line)"/>
<text x="410" y="146" text-anchor="middle" class="mono" fill="#d0d0d0">progress.jsonl</text>
<text x="410" y="166" text-anchor="middle" class="mono" font-size="11" fill="#a5adcb">one event a line</text>
<g stroke="var(--mute)" stroke-width="1.5" fill="none" marker-end="url(#ca)">
<line x1="262" y1="74" x2="555" y2="74"/>
<line x1="556" y1="148" x2="494" y2="148"/>
<line x1="328" y1="160" x2="262" y2="160"/>
</g>
<text x="410" y="66" text-anchor="middle" class="mono" font-size="12" fill="var(--hi)">TRACEPARENT</text>
<defs><marker id="ca" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0L10 5L0 10z" fill="var(--mute)"/></marker></defs>
</g>
</svg>
{{< /fig >}}

## Procedure

1. In the parent, put the `Traceparent` of
   {{< api "progress" "Span.TraceContext" >}} in the child's environment, as
   `TRACEPARENT`. Give the child an event log of its own, in a file of
   the parent's, by the flag or the variable that the child takes for it.
2. In the child, read `TRACEPARENT` and parse it with
   {{< api "progress" "ParseTraceContext" >}}. Give it to the Bus as
   `BusOptions.Trace`, or to `cliprogress.Options.Trace`.
3. In the parent, follow the log with {{< api "progress/eventlog" "NewTail" >}}.
   Poll it while the child runs, and once after it ended.
4. Feed the lines to a {{< api "progress/eventlog" "Reducer" >}}, to show the
   runs in a view, or to a {{< api "progress/eventlog" "Forwarder" >}}, to send
   the events of your trace on to a sink.

{{< example "progress/eventlog" "ExampleNewForwarder" >}}

{{% call note %}}
The event log has version 1. Keys can be added in a release, and are never
renamed or removed. A reader passes over the keys and the values that it does
not know. See [the event log](../../concepts/event-log/).
{{% /call %}}
