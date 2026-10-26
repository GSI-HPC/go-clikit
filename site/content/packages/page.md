---
title: page
description: Keep the templates of a page in one set, render a fragment the same everywhere, and answer the requests of htmx.
import: localweb/page
weight: 200
next: ["/packages/sse", "/packages/htmx"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A program parses its templates into one {{< api "localweb/page" "Set" >}}.
Each page and each fragment is a block of its own name. So a fragment renders
the same in the page, in the answer to htmx, and in an event of the stream.

```html
{{define "layout"}}<main>{{include .Content .}}</main>{{end}}
{{define "page-hosts"}}<table>{{include "hosts-table" .}}</table>{{end}}
```

{{< example "localweb/page" "ExampleSet_Respond" >}}

| Name | What it does |
| --- | --- |
| {{< api "localweb/page" "Parse" >}}, {{< api "localweb/page" "Set" >}} | the templates in one set; `include` runs a block by name |
| {{< api "localweb/page" "Set.Respond" >}} | renders the whole block before it writes anything |
| {{< api "localweb/page" "IsHTMX" >}}, {{< api "localweb/page" "Redirect" >}}, {{< api "localweb/page" "Retarget" >}} | the requests and headers of htmx |
| {{< api "localweb/page" "MaxIncludeDepth" >}} | a block that includes itself fails, and does not overflow the stack |

{{% call note %}}
Check at the start of the program that each block it names is there, with
`Set.Has`.
{{% /call %}}
