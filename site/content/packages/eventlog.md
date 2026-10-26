---
title: eventlog
description: Read back the event log that a program or its child wrote, as it grows, into runs or into a sink.
import: progress/eventlog
weight: 60
next: ["/concepts/event-log", "/guides/child-process"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`eventlog` reads the event log that `progress.Log` writes. A view of what a
program did reads it, and so does a server that runs a command as a child
and sends on the child's progress.

{{< fig caption="A Tail polls the log and gives each whole line to a Feed." >}}
<svg viewBox="0 0 820 190" role="img" aria-label="A log file is polled by a Tail, which feeds a Reducer for a view or a Forwarder for a sink.">
<g font-size="13" class="mono">
<rect x="10" y="65" width="170" height="56" rx="8" fill="var(--well)" stroke="var(--line)"/><text x="28" y="92" fill="#d0d0d0">progress.jsonl</text><text x="28" y="110" fill="#86a6ab" font-size="11">grows, rotates</text>
<rect x="240" y="65" width="140" height="56" rx="8" fill="var(--deck)" stroke="var(--isl-progress)"/><text x="262" y="98" fill="var(--fg)">Tail.Poll</text>
<rect x="450" y="20" width="160" height="56" rx="8" fill="var(--deck)" stroke="var(--isl-progress)"/><text x="470" y="53" fill="var(--fg)">Reducer</text>
<rect x="450" y="110" width="160" height="56" rx="8" fill="var(--deck)" stroke="var(--isl-progress)"/><text x="470" y="143" fill="var(--fg)">Forwarder</text>
<text x="640" y="53" fill="var(--text)">runs, for a view</text>
<text x="640" y="143" fill="var(--text)">events, to a Sink</text>
<g stroke="var(--mute)" stroke-width="1.5" fill="none"><line x1="182" y1="93" x2="238" y2="93"/><path d="M382 85 L448 50"/><path d="M382 101 L448 136"/></g>
</g>
</svg>
{{< /fig >}}

| Name | What it does |
| --- | --- |
| {{< api "progress/eventlog" "Decode" >}} | one line back into what was written, every text sanitised |
| {{< api "progress/eventlog" "NewTail" >}}, {{< api "progress/eventlog" "Tail" >}} | reads a log as it grows, and follows it when it is rotated |
| {{< api "progress/eventlog" "NewReducer" >}}, {{< api "progress/eventlog" "Reducer" >}} | folds the lines into runs, each the tree of its spans |
| {{< api "progress/eventlog" "NewForwarder" >}}, {{< api "progress/eventlog" "Forwarder" >}} | gives the events of one trace to a sink, once each |
| {{< api "progress/eventlog" "ErrVersion" >}} | a line of another version of the format |

{{< example "progress/eventlog" "ExampleReducer" >}}

{{% call note %}}
A reader of version 1 passes over the keys and the values that it does not
know. So a log that a later release writes, with a key added, still reads.
A line of another version is refused with `ErrVersion`.
{{% /call %}}
