---
title: localwebtest
description: A client that signs in with the launch token, and a reader of event streams, for a test of a local web server.
import: localweb/localwebtest
weight: 170
next: ["/packages/session", "/packages/sse"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A test of a local web server needs a browser that signed in, and that sends
its requests as a page of the server's own origin does.
{{< api "localweb/localwebtest" "Login" >}} gives it a
{{< api "localweb/localwebtest" "Client" >}}, and
{{< api "localweb/localwebtest" "Reader" >}} reads the events of a stream.

```go
srv := httptest.NewServer(handler)
c := localwebtest.Login(t, srv.URL, token)
c.Header.Set("X-Prog-CSRF", c.CSRF(t, "/"))
resp := c.PostForm(t, "/actions/stop", url.Values{"id": {"1"}})
```

{{% call note %}}
A server behind `localweb.Hosts` answers only to the address it bound. Serve
the handler through `httptest.NewServer`, whose URL is 127.0.0.1 and its
port, and give the Hosts check that Host.
{{% /call %}}

{{< example "localweb/localwebtest" "ExampleLogin" >}}
