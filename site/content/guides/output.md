---
title: Write a result
description: Write a command's result as a table aligned by display width, or as JSON, with every string escaped.
weight: 50
next: ["/packages/cliout", "/concepts/untrusted-text"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A result goes to standard output, for a person or for a script.
`cliout` writes it in two forms. Both escape every string, because a cell
often holds text from elsewhere, such as the reason a node gave.

## A table for a person

1. Make a {{< api "cliout" "Table" >}} with its
   {{< api "cliout" "Column" >}}s. Mark the columns that only `-o wide`
   shows.
2. Add a row for each item with {{< api "cliout" "Table.Add" >}}.
3. Write it with {{< api "cliout" "Table.Write" >}}, and say if the wide
   columns show.

{{< example "cliout" "ExampleTable" >}}

The columns align by the columns that a terminal gives the text, so a name
in Japanese or with accents does not move the next column.

## JSON for a script

{{< api "cliout" "WriteJSON" >}} writes a value as JSON. It escapes the
runes that `termtext` escapes and `encoding/json` does not, so a string
cannot act on the terminal that it is read in. A slice is written one
element at a time.

{{< example "cliout" "ExampleWriteJSON" >}}

{{% call note %}}
The package parses no flag. The words of `-o`, and which form each one
writes, are the program's to decide.
{{% /call %}}
