---
title: launch
description: Keep one server for each user, and open the browser on it without the launch token in a command line.
import: localweb/launch
weight: 180
next: ["/guides/local-web", "/packages/localweb"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A second start of the program opens the server of the first, and does not
start another. And the browser opens the address without the token in a
command line, because every user of the machine can read the argument list
of a process ({{< decision 35 >}}).

{{< steps caps="A start takes the lock in the user's runtime directory.|Running finds the record of a server that runs, and whose socket is the user's: open that one, and stop.|Otherwise listen, Publish the address, and let go of the lock.|The Opener writes the address to a file that only the user can read, and opens that file." >}}
<svg viewBox="0 0 820 160" role="img" aria-label="Acquire the lock, check Running, Publish, then open via a private file.">
<g class="mono" font-size="12" text-anchor="middle">
<rect x="10" y="50" width="170" height="50" rx="8" fill="var(--deck)" stroke="var(--isl-web)" data-step="1"/><text x="95" y="80" fill="var(--fg)" data-step="1">Acquire</text>
<rect x="215" y="50" width="170" height="50" rx="8" fill="var(--deck)" stroke="var(--isl-web)" data-step="2"/><text x="300" y="80" fill="var(--fg)" data-step="2">Running?</text>
<rect x="420" y="50" width="170" height="50" rx="8" fill="var(--deck)" stroke="var(--isl-web)" data-step="3"/><text x="505" y="80" fill="var(--fg)" data-step="3">Listen, Publish</text>
<rect x="625" y="50" width="185" height="50" rx="8" fill="var(--deck)" stroke="var(--run)" data-step="4"/><text x="717" y="80" fill="var(--fg)" data-step="4">Opener.Open</text>
<g stroke="var(--mute)" stroke-width="1.5"><line x1="182" y1="75" x2="213" y2="75" data-step="2"/><line x1="387" y1="75" x2="418" y2="75" data-step="3"/><line x1="592" y1="75" x2="623" y2="75" data-step="4"/></g>
<text x="300" y="130" fill="var(--mute)" data-step="2">yes: open it, stop</text>
<text x="717" y="130" fill="var(--mute)" data-step="4">a private redirect file</text>
</g>
</svg>
{{< /steps >}}

{{< example "localweb/launch" "ExampleAcquire" >}}

| Name | What it does |
| --- | --- |
| {{< api "localweb/launch" "Acquire" >}}, {{< api "localweb/launch" "Instance" >}} | the lock, and the record of the server that runs |
| {{< api "localweb/launch" "Instance.Running" >}}, {{< api "localweb/launch" "Instance.Publish" >}}, {{< api "localweb/launch" "Instance.Retire" >}} | find, write and remove the record |
| {{< api "localweb/launch" "Opener" >}} | opens the browser through a private file; `Skip` where no browser of the user's can |
| {{< api "localweb/launch" "Detached" >}} | starts a program that outlives this one |

The kit reads no environment: the program passes the runtime directory,
such as `$XDG_RUNTIME_DIR/prog`, and its name.
