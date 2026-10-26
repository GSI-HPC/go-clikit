---
title: htmx
description: The scripts of a local page, htmx, its SSE extension and idiomorph, byte for byte as their projects publish them.
import: localweb/htmx
weight: 210
next: ["/packages/assets", "/packages/sse"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

The package holds the scripts that a local web page of the kit runs: htmx,
its extension for Server-Sent Events, and idiomorph, which swaps a fragment
by changing only what changed. They are the files their projects publish,
byte for byte, under the Zero-Clause BSD licence. A test holds each to its
checksum.

{{< example "localweb/htmx" "Example" >}}

Serve them with `assets`, and put {{< api "localweb/htmx" "Config" >}} into
the page, so that htmx keeps to the Content-Security-Policy of
`localweb.Headers`.

| Name | What it is |
| --- | --- |
| {{< api "localweb/htmx" "Files" >}} | the scripts, as an `fs.FS` |
| {{< api "localweb/htmx" "Config" >}} | the `htmx-config` of the page; it restores history as a page ({{< decision 41 >}}) |
| {{< api "localweb/htmx" "ScriptVersion" >}}, {{< api "localweb/htmx" "SSEVersion" >}}, {{< api "localweb/htmx" "MorphVersion" >}} | the versions of the scripts |

A new version of a script comes in a minor release of the kit.
