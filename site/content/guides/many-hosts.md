---
title: Work on many hosts
description: Run a function on each of many targets, a bounded number at a time, with every target reported, and the failures summed up.
weight: 30
next: ["/packages/fanout", "/packages/fanouttest", "/concepts/displays"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A command for a cluster works on hundreds of nodes. {{< api "fanout" "Map" >}}
runs a function on each, a bounded number at a time. It reports a step with
a target for each item, folded, so that the tree shows node sets.

{{< cast display="tree" theme="tide" class="short" caption="Twelve nodes, four at a time. The ended targets fold into a node set." >}}

## Procedure

1. Give the step a name and a `Limit` in {{< api "fanout" "MapOptions" >}}.
2. Give `Describe` when a display should name an item by its node, its
   host or its role.
3. Write the function for one item. Return its result and its error.
4. Read the outcomes in the order of the items. Each has the value and the
   error of its item.
5. Return the error of `Map`. By default it is a
   {{< api "fanout" "FailureError" >}}, which names the items that failed.

{{< example "fanout" "ExampleMap" >}}

## More than a limit

| Need | Use |
| --- | --- |
| No more than two calls at a time to each gateway, whatever the limit | {{< api "fanout" "Keyed" >}}, and `MapOptions.Acquire` |
| Batches, one after the other, with a pause; stop at the first that fails | {{< api "fanout" "Batches" >}} |
| Leave an item out on purpose | {{< api "progress" "Skip" >}}: its target ends skipped |
| Your own exit code for a failed step | `MapOptions.Summarize`, which builds on `FailureError` |
| Bytes or items for each target | `ItemInfo.Unit` and `.Size`, and `progress.AdvanceWriter` |
| Prove the limit in a test | {{< api "fanout/fanouttest" "InFlight" >}} |

{{< example "fanout" "ExampleKeyed" >}}

{{% call caution %}}
A panic in the function of an item does not end the program. `Map` recovers
it, ends the target as failed with a {{< api "fanout" "PanicError" >}}, and
writes the stack to the `PanicLog`.
{{% /call %}}

## Prove the limit

A test that only counts calls does not prove that four ran at once.
{{< api "fanout/fanouttest" "InFlight" >}} holds each call until the
number you expect is under way together.

{{< example "fanout/fanouttest" "ExampleInFlight" >}}
