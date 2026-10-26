---
title: cliout
description: Write a result as a table aligned by display width, or as JSON, with every string made safe for a terminal.
import: cliout
weight: 90
next: ["/guides/output", "/packages/termtext"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`cliout` writes the result of a command for a person and for a program.
A cell often holds text from elsewhere, such as the reason a node gave.
`Table` escapes each cell with `termtext` before it measures it, and
`WriteJSON` escapes the runes that `encoding/json` leaves as they are.

```text
NODE     STATE    REASON
exe0001  drained  fan failure\nexe0002 idle
exe0003  idle
```

The newline in the reason cannot start a row that seems to be another.

| Name | What it does |
| --- | --- |
| {{< api "cliout" "Table" >}}, {{< api "cliout" "Column" >}} | columns aligned by display width; wide columns only for `-o wide` |
| {{< api "cliout" "WriteJSON" >}} | JSON with every string safe for a terminal; a slice one element at a time |

{{< example "cliout" "ExampleTable" >}}

## Fixed interfaces

What `cliout` writes ({{< decision 30 >}}). The package parses no flag: the
words of `-o` are the program's.
