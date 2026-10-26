---
title: mcpprogress
description: Send the progress of an MCP tool call as progress notifications, with no MCP SDK in the kit.
import: progress/mcpprogress
weight: 50
next: ["/guides/mcp", "/packages/eventlog"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A {{< api "progress/mcpprogress" "Notifier" >}} is a sink that sends the
progress notifications of the Model Context Protocol: how many targets of
the call ended, of how many, and a line for a person, such as
`read the groups: 3/16 done, 1 failed`.

The package imports no MCP SDK. The server gives it a `Send` that puts the
progress token in the notification and sends it on the session. So the kit
follows no release of an SDK ({{< decision 3 >}}).

{{< example "progress/mcpprogress" "Example" >}}

## What it sends

- **The total** is the `Total` of each step or batch that counts its
  targets, with none above it that does. It can grow as another starts,
  and never shrinks.
- **The progress** is the targets that ended. It never goes back, never
  passes the total, and is all of it once every step ended.
- **When**: at most once each {{< api "progress/mcpprogress" "Interval" >}},
  and at once when a step or a batch ends, but only when more targets
  ended. MCP asks that each notification has more progress than the last.
- A call that counts nothing sends nothing.

## Fixed interfaces

The interval, the rule that the progress only grows, and the words of the
line ({{< decision 27 >}} and {{< decision 32 >}}).
