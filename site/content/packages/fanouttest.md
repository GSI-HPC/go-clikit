---
title: fanouttest
description: Count the calls that a fake is in the middle of, to prove that a fan-out keeps its limit.
import: fanout/fanouttest
weight: 80
next: ["/packages/fanout", "/guides/many-hosts"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A fake that answers at once rarely has two calls under way together, so a
count alone proves little. {{< api "fanout/fanouttest" "InFlight" >}} holds
each call until enough others have joined it. A test of a limit holds calls
until one more than the limit are under way. A fan-out that keeps its limit
never allows that, so each round waits the full time with exactly the limit
in flight. A fan-out that starts one call too many is caught.

Wrap your fake in a few lines:

```go
func (f *fake) Run(ctx context.Context, req Request) (Result, error) {
	defer f.calls.Enter()()
	return f.answer(ctx, req)
}
```

{{< example "fanout/fanouttest" "ExampleInFlight" >}}

{{< example "fanout/fanouttest" "ExampleInFlight_patience" >}}
