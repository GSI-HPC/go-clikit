---
title: session
description: Let in only the browser that opened the printed address. Exchange the launch token for a session cookie, and keep the CSRF token.
import: localweb/session
weight: 150
next: ["/packages/localweb", "/packages/localwebtest"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

The address that a program prints carries a launch token. The first
navigation exchanges it for a session cookie. Each session has a CSRF token,
which each request that can change something sends back.

{{< steps caps="The program makes a token with NewToken, and prints the address with it.|The browser opens the address. The Manager sets the session cookie and redirects to the address without the token.|A form or a script sends the CSRF token back with each request that can change something." >}}
<svg viewBox="0 0 820 170" role="img" aria-label="Token in the address, exchanged for a cookie, then CSRF on each change.">
<g class="mono" font-size="12">
<rect x="10" y="30" width="240" height="44" rx="8" fill="var(--deck)" stroke="var(--isl-web)" data-step="1"/>
<text x="22" y="57" fill="var(--fg)" data-step="1">http://127.0.0.1:8421/?token=…</text>
<g data-step="2"><path d="M250 52 H300" stroke="var(--mute)" stroke-width="1.5"/>
<rect x="300" y="30" width="230" height="44" rx="8" fill="var(--deck)" stroke="var(--isl-web)"/>
<text x="312" y="50" fill="var(--fg)">Set-Cookie: …_8421</text><text x="312" y="66" fill="var(--mute)">HttpOnly; SameSite=Strict</text></g>
<g data-step="3"><path d="M530 52 H580" stroke="var(--mute)" stroke-width="1.5"/>
<rect x="580" y="30" width="230" height="44" rx="8" fill="var(--deck)" stroke="var(--run)"/>
<text x="592" y="57" fill="var(--fg)">POST + CSRF token</text></g>
</g>
</svg>
{{< /steps >}}

{{< example "localweb/session" "ExampleManager_Middleware" >}}

## The cookie

- `HttpOnly`, so no script reads it.
- `SameSite=Strict`, so a browser leaves it out of a request that another
  site starts.
- Its name ends in the port of the Host, because a browser keeps cookies by
  host name and not by port. Two servers on one machine each keep their own
  session.
- No `Secure` attribute, which a browser refuses over the plain HTTP of a
  loopback listener, and no lifetime.

The names and texts that this package sends are an interface
({{< decision 34 >}}).
