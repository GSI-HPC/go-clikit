---
title: assets
description: Serve the static files of a page at addresses with a hash of them all, so that a browser keeps each one for good.
import: localweb/assets
weight: 190
next: ["/packages/page", "/packages/htmx"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A program embeds its files, so it works offline and loads nothing from
elsewhere. {{< api "localweb/assets" "New" >}} serves them at
`PREFIX/VERSION/NAME`, where the version is a hash of every file. A new
build serves its files at new addresses, so a browser can keep each one for
good.

{{< example "localweb/assets" "ExampleNew" >}}

A page links to a file with `URL`, such as from a template function:

```html
<link rel="stylesheet" href="{{asset "app.css"}}">
```

With `Options.Dev`, the files are read again for each request from a
directory that the program names, and the browser checks them each time.
Use it to work on the files without building the program again.
