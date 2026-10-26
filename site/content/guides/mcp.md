---
title: Report progress over MCP
description: Send the progress of an MCP tool call as progress notifications, with no MCP SDK in the kit.
weight: 100
next: ["/packages/mcpprogress", "/guides/child-process"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A tool call of an MCP server can take long, such as a call that works on
many hosts. When the call came with a progress token, the client can show
how far it got. {{< api "progress/mcpprogress" "Notifier" >}} is a sink that
sends the progress notifications of the Model Context Protocol.

## Procedure

1. When a call comes with a progress token, make a Bus for that call alone,
   with a Notifier among its sinks.
2. Give the Notifier a `Send` that puts the token in the notification and
   sends it on the call's session. The kit imports no MCP SDK (decision 3).
3. Run the call under the Bus.
4. When the handler returned, close the Bus, then close the Notifier with a
   short deadline. Then answer the call. So nothing is sent after the
   result.

{{< example "progress/mcpprogress" "Example" >}}

## What the client sees

- The progress counts the targets that ended, of how many the call
  expects. It never goes back, and never passes the total.
- A notification goes out at most once per interval, and at once when a step
  or a batch ends. It goes only when more targets ended.
- The message is a line such as `read the groups: 3/16 done, 1 failed`.

The rate, the rule that the progress only grows, and the words of the line
are an interface (decisions 27 and 32).

{{% call note %}}
To forward the progress of a child that runs the call, poll its event log
into a Forwarder with the Notifier as its sink. See
[Follow a child process](../child-process/).
{{% /call %}}
