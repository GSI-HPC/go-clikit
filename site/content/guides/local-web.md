---
title: Serve a page to the user
description: Serve a local web page that only the user who started the program can reach, with live fragments, one server per user.
weight: 80
next: ["/packages/localweb", "/packages/session", "/packages/sse", "/packages/launch"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A program can show a page in the user's browser, served from the user's own
machine. Such a server acts as the user. So whoever can send it a request
acts as the user too: another user of the machine, any page in the user's
browser, and a page of another site that makes its name resolve to
127.0.0.1. `localweb` keeps the three out, each with a check of its own
(decision 34).

{{< steps caps="A request comes in on a loopback port. The listener closes a connection that another user opened.|Hosts refuses a Host header that names another server: DNS rebinding.|SameOrigin refuses what a page of another site caused, by Sec-Fetch-Site and Origin.|The session lets in only the browser that opened the printed address, and checks the CSRF token.|The handler of the program sees the request." >}}
<svg viewBox="0 0 820 200" role="img" aria-label="A request passes the listener, Hosts, SameOrigin and the session before the handler.">
<g font-size="13" text-anchor="middle">
<g class="mono">
<rect x="10" y="70" width="140" height="56" rx="10" fill="var(--deck)" stroke="var(--isl-web)"/><text x="80" y="103" fill="var(--fg)">listener</text>
<rect x="175" y="70" width="140" height="56" rx="10" fill="var(--deck)" stroke="var(--isl-web)"/><text x="245" y="103" fill="var(--fg)">Hosts</text>
<rect x="340" y="70" width="140" height="56" rx="10" fill="var(--deck)" stroke="var(--isl-web)"/><text x="410" y="103" fill="var(--fg)">SameOrigin</text>
<rect x="505" y="70" width="140" height="56" rx="10" fill="var(--deck)" stroke="var(--isl-web)"/><text x="575" y="103" fill="var(--fg)">session</text>
<rect x="670" y="70" width="140" height="56" rx="10" fill="var(--deck)" stroke="var(--run)"/><text x="740" y="103" fill="var(--run)">handler</text>
</g>
<g fill="var(--bad)" font-size="12">
<text x="80" y="160" data-only="1">other user: closed</text>
<text x="245" y="160" data-only="2">421 Misdirected</text>
<text x="410" y="160" data-only="3">403 cross-site</text>
<text x="575" y="160" data-only="4">403 no session</text>
</g>
<circle r="9" fill="var(--hi)" data-at="80,45|245,45|410,45|575,45|740,45"/>
</g>
</svg>
{{< /steps >}}

## Procedure

1. Take the lock of the user's server with
   {{< api "localweb/launch" "Acquire" >}}. When a server of the user runs
   already, open that one and stop.
2. Listen with {{< api "localweb" "Listen" >}} on a loopback address or a
   Unix socket. On Linux, wrap the TCP listener with
   {{< api "localweb" "PeerListener" >}}.
3. Make a launch token with {{< api "localweb/session" "NewToken" >}}, and a
   {{< api "localweb/session" "Manager" >}}.
4. Chain the checks, outermost first: {{< api "localweb" "Headers" >}},
   {{< api "localweb" "Hosts" >}}, {{< api "localweb" "SameOrigin" >}},
   `Manager.Middleware`, then your handler.
5. Serve with {{< api "localweb" "Serve" >}}, which sets every timeout and
   stops within a grace period when the context ends.
6. Print the address with the token, and open the browser with a
   {{< api "localweb/launch" "Opener" >}}. The Opener writes the address to
   a private file, because every user can read a command line.

{{< example "localweb" "ExampleListen" >}}

{{% call warning %}}
Do not leave out a check because the server listens on a loopback address.
Every user of the machine, and every page in the browser, can reach a
loopback port.
{{% /call %}}

## Live fragments

A page names its fragments, and the topics that each depends on. The store
of the program notifies a topic when it changes. {{< api "localweb/sse" "Serve" >}}
sends every fragment first, then each fragment whose topic changed and whose
HTML is new. {{< api "localweb/page" "Set" >}} renders the page and its
fragments from one template set, and {{< api "localweb/htmx" "" >}} holds the
scripts that swap them in.

{{< example "localweb/sse" "ExampleServe" >}}

## Test the server

{{< api "localweb/localwebtest" "Login" >}} gives a test a client that
signed in with the token, and {{< api "localweb/localwebtest" "Reader" >}}
reads the events of a stream.
