---
title: fanout
description: Run work on many targets in a bounded pool that reports each target, recovers a panic, and sums up the failures.
import: fanout
weight: 70
next: ["/guides/many-hosts", "/packages/fanouttest"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`fanout` works on many items at once, a bounded number at a time, and
reports them as `progress` asks.

{{< steps caps="Map announces a target for each item, queued, before the first runs.|Four run at once: the Limit.|A place comes free; the next item takes it. exe03 fails.|The step ends. Its error names the items that failed." >}}
<svg viewBox="0 0 820 170" role="img" aria-label="Eight targets queued, four running at a time, one failing.">
<g class="mono" font-size="12">
<g fill="var(--deck)" stroke="var(--mute)" stroke-dasharray="4 3">
<rect x="20" y="40" width="88" height="34" rx="6"/><rect x="118" y="40" width="88" height="34" rx="6"/><rect x="216" y="40" width="88" height="34" rx="6"/><rect x="314" y="40" width="88" height="34" rx="6"/>
<rect x="412" y="40" width="88" height="34" rx="6"/><rect x="510" y="40" width="88" height="34" rx="6"/><rect x="608" y="40" width="88" height="34" rx="6"/><rect x="706" y="40" width="88" height="34" rx="6"/>
</g>
<g fill="var(--run)" opacity=".85" data-only="2"><rect x="20" y="40" width="88" height="34" rx="6"/><rect x="118" y="40" width="88" height="34" rx="6"/><rect x="216" y="40" width="88" height="34" rx="6"/><rect x="314" y="40" width="88" height="34" rx="6"/></g>
<g data-only="3"><rect x="20" y="40" width="88" height="34" rx="6" fill="var(--ok)"/><rect x="118" y="40" width="88" height="34" rx="6" fill="var(--ok)"/><rect x="216" y="40" width="88" height="34" rx="6" fill="var(--bad)"/>
<rect x="314" y="40" width="88" height="34" rx="6" fill="var(--run)"/><rect x="412" y="40" width="88" height="34" rx="6" fill="var(--run)"/><rect x="510" y="40" width="88" height="34" rx="6" fill="var(--run)"/><rect x="608" y="40" width="88" height="34" rx="6" fill="var(--run)"/></g>
<g data-step="4"><g fill="var(--ok)"><rect x="20" y="40" width="88" height="34" rx="6"/><rect x="118" y="40" width="88" height="34" rx="6"/><rect x="314" y="40" width="88" height="34" rx="6"/><rect x="412" y="40" width="88" height="34" rx="6"/><rect x="510" y="40" width="88" height="34" rx="6"/><rect x="608" y="40" width="88" height="34" rx="6"/><rect x="706" y="40" width="88" height="34" rx="6"/></g><rect x="216" y="40" width="88" height="34" rx="6" fill="var(--bad)"/>
<text x="20" y="120" fill="var(--bad)">1 of 8 failed: exe03</text></g>
<g fill="var(--fg)"><text x="36" y="62">exe01</text><text x="134" y="62">exe02</text><text x="232" y="62">exe03</text><text x="330" y="62">exe04</text><text x="428" y="62">exe05</text><text x="526" y="62">exe06</text><text x="624" y="62">exe07</text><text x="722" y="62">exe08</text></g>
</g>
</svg>
{{< /steps >}}

{{< example "fanout" "ExampleMap" >}}

## The main names

| Name | What it does |
| --- | --- |
| {{< api "fanout" "Map" >}}, {{< api "fanout" "MapOptions" >}} | a step with a target for each item; the outcomes in the order of the items |
| {{< api "fanout" "Each" >}} | the bounded loop alone, which reports nothing and recovers nothing |
| {{< api "fanout" "Batches" >}} | a node set in batches, one after the other, with a pause between |
| {{< api "fanout" "Keyed" >}} | at most a limit under way on each key, such as a gateway |
| {{< api "fanout" "FailureError" >}}, {{< api "fanout" "Summary" >}} | the default error of a step, and what it sums up |
| {{< api "fanout" "PanicError" >}}, {{< api "fanout" "Recovered" >}} | a panic in the work of an item, as its error |

A limit below one is {{< api "fanout" "DefaultLimit" >}}, 16, for `Each` and
`Map`, and one for `Keyed`.

{{% call caution %}}
Only `Map` recovers a panic. A panic in the work of `Each` ends the
process, as in any goroutine, unless the work recovers it.
{{% /call %}}
