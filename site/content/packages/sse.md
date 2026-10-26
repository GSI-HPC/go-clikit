---
title: sse
description: Stream the live parts of a page as Server-Sent Events. Send every fragment first, then each one whose topic changed.
import: localweb/sse
weight: 160
next: ["/packages/page", "/packages/htmx"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

Each event is named after a fragment of the page, and carries its HTML.
htmx's SSE extension swaps it into the element of that name.

{{< steps caps="A stream subscribes, and sends every fragment: the baseline.|The store changes the runs, and notifies the topic runs.|The stream gathers changes for a moment, renders the fragments that depend on runs, and sends only those whose HTML is new." >}}
<svg viewBox="0 0 820 180" role="img" aria-label="Baseline of all fragments, then only changed fragments.">
<g class="mono" font-size="12">
<rect x="20" y="20" width="180" height="140" rx="10" fill="var(--deck)" stroke="var(--isl-web)"/>
<text x="36" y="44" fill="var(--isl-web)">page</text>
<rect x="36" y="56" width="148" height="26" rx="5" fill="var(--well)" stroke="var(--line)"/><text x="46" y="74" fill="#d0d0d0">#runs</text>
<rect x="36" y="90" width="148" height="26" rx="5" fill="var(--well)" stroke="var(--line)"/><text x="46" y="108" fill="#d0d0d0">#hosts</text>
<rect x="36" y="124" width="148" height="26" rx="5" fill="var(--well)" stroke="var(--line)"/><text x="46" y="142" fill="#d0d0d0">#status</text>
<g data-only="1" fill="var(--hi)"><text x="240" y="74">event: runs</text><text x="240" y="108">event: hosts</text><text x="240" y="142">event: status</text></g>
<g data-only="2"><rect x="560" y="60" width="240" height="44" rx="8" fill="var(--deck)" stroke="var(--isl-command)"/><text x="574" y="87" fill="var(--fg)">store.notifier.Notify("runs")</text></g>
<g data-only="3" fill="var(--hi)"><text x="240" y="74">event: runs</text><rect x="36" y="56" width="148" height="26" rx="5" fill="none" stroke="var(--hi)" stroke-width="2"/></g>
</g>
</svg>
{{< /steps >}}

The baseline comes first, because the page can be older than a change that
the stream did not hear of. So a stream needs no event IDs, and a reconnect
loses nothing.

{{< example "localweb/sse" "ExampleServe" >}}

| Name | What it does |
| --- | --- |
| {{< api "localweb/sse" "Notifier" >}}, {{< api "localweb/sse" "Subscription" >}} | the topics that change, told to each subscriber once |
| {{< api "localweb/sse" "Serve" >}}, {{< api "localweb/sse" "Options" >}} | the stream: the baseline, then what changed |
| {{< api "localweb/sse" "Event" >}}, {{< api "localweb/sse" "Format" >}} | one event; a data line for each line of HTML |

The stream ends when `localweb.Serve` starts to stop, so it does not hold
the stop for the grace period.
