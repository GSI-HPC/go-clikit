---
title: Untrusted text
description: Text from another host can move the cursor, retitle the terminal or reverse a line. termtext shows such characters as visible escapes, and measures text in columns.
weight: 40
next: ["/packages/termtext", "/guides/many-hosts"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A command that works on many hosts prints what those hosts said: their
output, their errors, the values a service reported. None of that text is
trusted. A carriage return can overwrite a line printed before. An escape
sequence can retitle the terminal or write the clipboard. A bidirectional
control can show a name in another order than it has.

{{< steps caps="A remote host sends a line that holds an escape sequence and a carriage return.|Written as it is, the terminal obeys: the title changes and the line is overwritten.|Escaped with termtext, the reader sees what was sent." >}}
<svg viewBox="0 0 760 210" role="img" aria-label="A line from a remote host, as the terminal shows it raw and escaped.">
<g class="mono" font-size="13">
<g data-step="1">
<text x="20" y="30" fill="var(--mute)">exe07 sent</text>
<rect x="20" y="40" width="720" height="34" rx="6" fill="var(--well)" stroke="var(--line)"/>
<text x="34" y="62" fill="#d0d0d0">backup done<tspan fill="#e07a2e">ESC]0;owned BEL</tspan><tspan fill="#e07a2e">CR</tspan>backup FAILED</text>
</g>
<g data-only="2">
<text x="20" y="110" fill="var(--bad)">raw: the terminal obeys</text>
<rect x="20" y="120" width="720" height="70" rx="6" fill="var(--well)" stroke="var(--bad)"/>
<rect x="20" y="120" width="720" height="20" rx="6" fill="#1d3640"/><text x="34" y="135" fill="#d0d0d0" font-size="11">owned</text>
<text x="34" y="170" fill="#d0d0d0">backup FAILED</text>
</g>
<g data-only="3">
<text x="20" y="110" fill="var(--run)">termtext.EscapeLines: the reader sees what was sent</text>
<rect x="20" y="120" width="720" height="70" rx="6" fill="var(--well)" stroke="var(--run)"/>
<text x="34" y="160" fill="#d0d0d0">backup done<tspan fill="#5fd7ff">\x1b</tspan>]0;owned<tspan fill="#5fd7ff">\x07\r</tspan>backup FAILED</text>
</g>
</g>
</svg>
{{< /steps >}}

## One place to fix

A program that escapes everything with `termtext`, and nothing with
anything else, has one place to fix when a character turns out to need
escaping too. The kit escapes what it writes itself: the names, errors and
lines that the displays show, and what `cliout` writes.

| Function | Use it for |
| --- | --- |
| {{< api "termtext" "Escape" >}} | one value on one line, such as a name or an error: newline and tab are escaped too |
| {{< api "termtext" "EscapeLines" >}} | text of many lines, such as the output of a command: newline and tab stay |
| {{< api "termtext" "NewEscapeWriter" >}} | a stream, such as a child's output, escaped line by line as it arrives |

{{< example "termtext" "ExampleEscapeLines" >}}

## The policy

The policy is a deny-list. A rune is escaped when it can move the cursor,
change the state of the terminal, or change the order or the lines that text
is shown in:

- every C0 control and DEL, except newline and tab in `EscapeLines`;
- every C1 control, which some terminals read as the start of a control
  sequence;
- the bidirectional controls and the line and paragraph separators;
- every byte that is not valid UTF-8.

The forms of an escape are stable: `\r`, `\n` and `\t`, `\x` and two digits
for another C0 control, DEL or a bad byte, and `\u` and four digits for any
other rune. Escaping is idempotent: escaped text does not change when you
escape it again.

{{% call caution %}}
The set of runes that is escaped can grow in a minor release (decision 14).
Do not compare escaped text in a test with text that holds a rune outside
plain ASCII and the escapes above.
{{% /call %}}

{{% call note %}}
The CRLF rule (decision 42): in `EscapeLines` and the writer, a carriage
return just before a newline is part of the line end and stays. So the
output of `ssh -t` and of a Windows host keeps its line ends. A carriage
return anywhere else is escaped.
{{% /call %}}

Other invisible characters, such as a zero-width space, stay as they are.
They change neither the cursor nor the terminal. Do not use `termtext` to
make look-alike names visible.

## Columns, not bytes

A display cuts rows to the width of the terminal, and a table aligns its
columns. Both need the columns a terminal gives text, which is not its
length in bytes nor in runes.

{{< example "termtext" "ExampleWidth" >}}

- {{< api "termtext" "Width" >}} counts wide characters as two columns, and
  combining marks as none. It is meant never to count fewer columns than a
  terminal shows, so that a cut row does not wrap.
- {{< api "termtext" "Truncate" >}} cuts text to a number of columns and
  never splits a wide rune.
- {{< api "termtext" "Wrap" >}} fills a paragraph to a number of columns,
  with an indent for the first line and the others.

All three expect text that is already escaped.
