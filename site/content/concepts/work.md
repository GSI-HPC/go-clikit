---
title: Work and the Meter
description: A span can report an amount of work, such as bytes, out of a size. The Bus samples it, and a sink rolls it up the tree.
weight: 20
next: ["/concepts/displays", "/packages/progress"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A count of targets shows how far a step got. A download of 2 GiB is one
target for a long time, so it needs another measure: an amount of work, in a
unit, out of a size when the size is known (decision 23).

{{< cast display="tree" theme="tide" still="3" title="reinstall --progress=tty: the work of a step" caption="During the first five seconds, the step reports bytes. The tree shows the bar, the amount, the share, the rate and the time left. The frame at rest is the third." >}}

## Report work

Give the unit and the size when you start the span, with
{{< api "progress" "Work" >}}. Then move the amount:

- {{< api "progress" "Span.Advance" >}} adds to the amount.
- {{< api "progress" "Span.SetAmount" >}} sets it.
- {{< api "progress" "AdvanceWriter" >}} and
  {{< api "progress" "AdvanceReader" >}} wrap a writer or a reader, and
  advance the span of the context by each byte that goes through.

The units are items, bytes and percent. A span in percent has a size of 100.
A span that was given no `Work` counts items. `Advance` does nothing to a
span that does not run, and nothing to a nil span.

{{< example "progress" "ExampleSpan_Advance" >}}

## The Bus samples, nobody waits

A copy can write a hundred thousand times a second. So `Advance` adds to an
atomic amount and takes no lock. The Bus sends the amount as an event at
most every 100 ms for each span, by its own clock, and always with the
span's end, so that the newest amount is never lost. An update of the span
carries the newest amount too, so that work which learns its amount and its
size sets the amount first, and a new size never meets an old amount. The
event log writes an advance at most once a second by default, and counts the
advances it leaves out in `leftOut` on the next advance it writes. It holds
the newest one it would leave out, and writes it once its second has passed,
so that a reader of the log sees where the work stopped while it pauses.

{{% call note %}}
`Advance` and `SetAmount` read the clock of the Bus, `BusOptions.Now`, on
the goroutine that calls them, outside the lock of the Bus. A clock that you
give the Bus must be safe for concurrent use.
{{% /call %}}

## The Meter rolls the work up

An event carries only what its span said. {{< api "progress" "Meter" >}} is
the half on the side of the sink: it is given every event in order, as a sink
is, and keeps the work of each span, rolled up the tree. Each display keeps
a Meter and reads it when it draws a frame. So the rows move between events.

{{< fig caption="Where the share done of a span comes from. The first rule that applies wins." >}}
<svg viewBox="0 0 820 220" role="img" aria-label="Three rules: a span with a size of its own; a step that folds its targets; a span over children that all have sizes.">
<g font-size="14" fill="var(--fg)">
<rect x="10" y="10" width="255" height="200" rx="10" fill="var(--ink)" stroke="var(--run)"/>
<text x="26" y="38" font-weight="700" class="mono" fill="var(--run)">1 BoundSize</text>
<text x="26" y="64" font-size="13" fill="var(--text)">The span has a size.</text>
<text x="26" y="84" font-size="13" fill="var(--text)">Share = amount / size.</text>
<rect x="26" y="120" width="220" height="16" rx="3" fill="none" stroke="var(--mute)"/><rect x="26" y="120" width="123" height="16" rx="3" fill="var(--run)"/>
<text x="26" y="160" font-size="12" class="mono" fill="var(--mute)">1.1/2.0 GiB · 56%</text>
<rect x="282" y="10" width="255" height="200" rx="10" fill="var(--ink)" stroke="var(--ok)"/>
<text x="298" y="38" font-weight="700" class="mono" fill="var(--ok)">2 BoundTargets</text>
<text x="298" y="64" font-size="13" fill="var(--text)">A step that folds, or a batch</text>
<text x="298" y="84" font-size="13" fill="var(--text)">with a Total: ended targets</text>
<text x="298" y="102" font-size="13" fill="var(--text)">and running shares / Total.</text>
<g fill="var(--ok)"><rect x="298" y="122" width="26" height="26" rx="4"/><rect x="330" y="122" width="26" height="26" rx="4"/><rect x="362" y="122" width="26" height="26" rx="4"/></g>
<rect x="394" y="122" width="26" height="26" rx="4" fill="none" stroke="var(--ok)"/><rect x="394" y="135" width="26" height="13" rx="2" fill="var(--ok)"/>
<g fill="none" stroke="var(--mute)"><rect x="426" y="122" width="26" height="26" rx="4"/><rect x="458" y="122" width="26" height="26" rx="4"/></g>
<text x="298" y="176" font-size="12" class="mono" fill="var(--mute)">3.5 of 6 targets</text>
<rect x="554" y="10" width="255" height="200" rx="10" fill="var(--ink)" stroke="var(--isl-web)"/>
<text x="570" y="38" font-weight="700" class="mono" fill="var(--isl-web)">3 BoundChildren</text>
<text x="570" y="64" font-size="13" fill="var(--text)">Any other span: the sum of</text>
<text x="570" y="84" font-size="13" fill="var(--text)">the amounts below, over the</text>
<text x="570" y="102" font-size="13" fill="var(--text)">sum of their sizes.</text>
<rect x="570" y="122" width="100" height="14" rx="3" fill="none" stroke="var(--mute)"/><rect x="570" y="122" width="80" height="14" rx="3" fill="var(--isl-web)"/>
<rect x="690" y="122" width="100" height="14" rx="3" fill="none" stroke="var(--mute)"/><rect x="690" y="122" width="30" height="14" rx="3" fill="var(--isl-web)"/>
<text x="570" y="166" font-size="12" class="mono" fill="var(--mute)">(80 + 30) / 200 = 55%</text>
</g>
</svg>
{{< /fig >}}

A {{< api "progress" "Reading" >}} also has the rate over the last five
seconds, the time left once the span has run for two seconds, and how long
the amount has not grown, once that is five seconds. A display shows this
as "stalled".

{{< example "progress" "ExampleMeter" >}}

## What to use when

| You have | Use |
| --- | --- |
| A step over many targets | `Total` and `FlagFold`; no work needed |
| A copy of bytes with a known size | `Work(UnitBytes, size)` and `AdvanceWriter` |
| A tool that prints its own percent | `Work(UnitPercent, 100)` and `SetAmount` |
| A loop over items of one target | `Work(UnitItems, n)` and `Advance(1)` |
