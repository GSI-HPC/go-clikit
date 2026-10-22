<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# UI convention

The programs that serve a local web page (sind, brig and sdash) look and
behave alike, so that a person who knows one knows the others. The kit
cannot hold this in code, because the pages, the templates and the CSS name
each program's domain
([decision 25](decisions.md#25-the-local-web-server-is-the-kits-in-localweb)).
This page is the rule they follow. A program that departs from a row says
so in its own manual.

## Keys

A key with no modifier acts only when no input, text area or select has the
focus, and when the program has not turned single-character keys off. A
program turns them off with a switch in its settings, as sdash does today,
for people who use a screen reader or a voice tool.

`g` starts a go-to sequence: the second key comes within one second. The
letters of a go-to key name the **role** of the page, not its content.

| Keys | Role | Notes |
| --- | --- | --- |
| `g h` | Home: the overview the program opens on | |
| `g d` | Diagnostics: the doctor and its checks | sind and brig use `g d` for this already; sdash moves its diagnostics here |
| `g c` | Configuration: the settings of the program | sind's `g c` for clusters moves to a domain letter |
| `g a` | Activity: the runs, the events, the audit trail | |
| `g` + other letter | A page of the program's domain, such as clusters or nodes | The program picks a letter that is not in this table, and lists it in its `?` view |
| `?` | A list of the keys that act on this page | One meaning in all programs; contextual help takes another key |
| `/` | Put the focus in the filter of the page | |
| `Esc` | Close the dialog or the menu, then clear the filter | |
| `j` and `k` | Next and previous row of a list | `Enter` opens the row |
| `t` | Switch between the light and the dark theme | See `data-theme` |

The reserved letters (`h`, `d`, `c`, `a`) never name a domain page. A new
role is added to this table before a program uses it.

## Theme

- The theme is the `data-theme` attribute of the `<html>` element, `light`
  or `dark`. Without the attribute the page follows `prefers-color-scheme`.
- The CSS defines colours as custom properties on `:root`, redefines them
  for the dark theme, and never reads the theme from a class.
- The kit ships no font and no palette. The brand is the program's.

## Storage keys

A page that keeps a setting in the browser, such as the theme or a
collapsed section, uses `localStorage` with a key of the form
`PROGRAM:NAME`, such as `sind:theme`. The program's name comes from the
program ([decision 21](decisions.md#21-the-command-lines-glue-for-progress-is-the-kits));
the kit's code never reads or writes a key of its own. Every read and write
sits in a `try`, because the storage can be absent or full, and the page
works without it. Nothing that must survive or be shared goes there.

## Headers

- `Referrer-Policy: no-referrer` on every response, so that the launch URL
  and its token never leave in a `Referer` header.
- The other headers (CSP, `X-Content-Type-Options`, frame options) are the
  defence of `localweb`, not of the convention.

## Accessibility floor

- The first focusable element of every page is a skip link to the main
  content.
- The landmarks are labelled: one `main`, one `nav` with a label, and a
  `header` and a `footer` where the page has them.
- Every control has a name that a screen reader reads, and a visible focus
  ring.
- Colour is never the only sign of a state; a state also has a word or a
  shape.
- Each page passes an automated axe run with no violation, in the program's
  own end-to-end tests.
