---
title: termtext
description: Escape untrusted text for a terminal, and measure, cut and wrap it in columns.
import: termtext
weight: 5
next: ["/concepts/untrusted-text", "/packages/cliout"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`termtext` is the one escaper of the kit. Text from elsewhere, such as the
output of a remote host, can move the cursor, retitle the terminal or show a
name in another order. `termtext` shows such characters as visible escapes.
It also measures text in the columns that a terminal gives it.

{{< example "termtext" "ExampleEscapeLines" >}}

## The main names

| Name | What it does |
| --- | --- |
| {{< api "termtext" "Escape" >}} | one value on one line: newline and tab are escaped too |
| {{< api "termtext" "EscapeLines" >}} | text of many lines: newline, tab and a CRLF line end stay |
| {{< api "termtext" "NewEscapeWriter" >}} | a stream, escaped line by line as it comes |
| {{< api "termtext" "Width" >}} | the columns of text: two for a wide rune, none for a mark |
| {{< api "termtext" "Truncate" >}} | text cut to a number of columns, never inside a wide rune |
| {{< api "termtext" "Wrap" >}} | a paragraph filled to a number of columns, with indents |

{{< example "termtext" "ExampleWrap" >}}

## Fixed interfaces

- The forms of the escapes: `\r`, `\n`, `\t`, `\xNN` and `\uNNNN`.
- The CRLF rule ({{< decision 42 >}}).
- The set of escaped runes can grow in a minor release
  ({{< decision 14 >}}).

## Read more

[Untrusted text](../../concepts/untrusted-text/) explains the policy.
