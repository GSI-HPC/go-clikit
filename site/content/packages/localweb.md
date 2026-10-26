---
title: localweb
description: Guard a local web server that acts as the user, with its listener, the Host and cross-site checks, and the headers.
import: localweb
weight: 140
next: ["/guides/local-web", "/packages/session", "/packages/sse"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A command-line program can serve a page to the user who started it, on the
user's own machine. Such a server acts as the user. `localweb` keeps out
the three others that can reach it ({{< decision 34 >}}):

| Who | What keeps them out |
| --- | --- |
| another user of the machine, who can reach a loopback port | {{< api "localweb" "Listen" >}} on loopback or a private Unix socket; {{< api "localweb" "PeerListener" >}} on Linux |
| a page of another site that makes its name resolve to 127.0.0.1 | {{< api "localweb" "Hosts" >}}: the Host header must name this server |
| any page in the user's browser | {{< api "localweb" "SameOrigin" >}}, by `Sec-Fetch-Site` and `Origin` |

{{< api "localweb" "Headers" >}} sets the Content-Security-Policy and the
frame, referrer and resource policies, and
{{< api "localweb" "Serve" >}} serves with every timeout set.

{{< example "localweb" "ExampleListen" >}}

## The order

The checks are middleware of the type `func(http.Handler) http.Handler`,
so `http.ServeMux` and a router such as chi both take them. Put them in this
order, outermost first:

1. `Headers`
2. `Hosts`
3. `SameOrigin`
4. `session.Manager.Middleware`
5. your handler

A check that refuses a request answers with a {{< api "localweb" "Refusal" >}}.
A {{< api "localweb" "Refuser" >}} lets the program write it in its own form,
and log the reason.

{{% call caution %}}
Every port of 127.0.0.1 is the same site. A `SameSite` cookie alone does
not keep out the pages that another program serves there.
{{% /call %}}

## Fixed interfaces

The refusals, the headers, the cookie and the frames of `localweb` and its
packages ({{< decision 34 >}}). `net/http` and `html/template` are imported
in `localweb` and its packages alone ({{< decision 25 >}}).
