---
title: go-clikit
headline: The runtime that GSI-HPC's command-line tools share.
description: A library reports its work as spans. The command decides how to show them, as a live tree, a counter, plain lines for CI, or a JSONL log for programs. Without a listener, the report costs nothing.
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

<section class="home-sec">
<h2><small>01</small> One stream of events, four views</h2>
<p>The library sends the same events in each case. The command picks the view with one flag, <code>--progress</code>. The tree above is one view. These are the other three.</p>
<div class="views">

{{< cast display="counter" theme="tide" class="short" title="--progress=counter" caption="**Counter.** One line, for a small terminal." >}}

{{< cast display="plain" theme="none" class="short" title="--progress=plain" caption="**Plain lines.** One line for each change worth a line, for a CI log." >}}

</div>

```json
{"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":4,"time":"2026-09-25T12:00:00.000000000Z","type":"start","span":"#3","parent":"#1","kind":"step","name":"run","flags":["fold","show-lines"],"state":"running","total":2,"limit":1}
```

<p><b>Event log.</b> One JSON line for each event, for a program; this one is from the golden log of the tests. The format is an interface: <a href="concepts/event-log/">version 1</a> adds keys and never changes them.</p>
</section>

<section class="home-sec">
<h2><small>02</small> Find your way</h2>
<p>The kit has 22 packages in four groups. An arrow goes from a package to a package that it imports. Select a package to read about it.</p>

{{< map >}}

</section>

<section class="home-sec">
<h2><small>03</small> Start from your job</h2>
<div class="journeys">
<a href="guides/library/"><b>I write a library</b><span>Report the work of your code. Run it on many hosts. Test what it reports.</span><ol><li>progress</li><li>fanout</li><li>progresstest</li></ol></a>
<a href="tour/"><b>I write a command</b><span>Strict flags, a live display, a result as a table or JSON, a doctor.</span><ol><li>cobratree</li><li>cliprogress</li><li>cliout</li><li>doctor</li></ol></a>
<a href="guides/local-web/"><b>I serve a local web UI</b><span>One server for each user, safe from other users and other sites.</span><ol><li>localweb</li><li>session</li><li>sse</li><li>launch</li></ol></a>
</div>
</section>

<section class="home-sec">
<h2><small>04</small> This site and pkg.go.dev</h2>
<p>This site tells why the kit works as it does, how its parts fit together, and what a command looks like when it runs. <a href="https://pkg.go.dev/github.com/GSI-HPC/go-clikit">pkg.go.dev</a> holds each name, its doc comment and its examples. A page here ends each topic with a link to the names on pkg.go.dev. The kit is used by <a href="https://github.com/GSI-HPC/clusterctl">clusterctl</a>, <a href="https://github.com/GSI-HPC/sind">sind</a> and brig.</p>
</section>
