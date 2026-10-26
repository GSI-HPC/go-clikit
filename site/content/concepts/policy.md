---
title: The program decides
description: The kit knows no program. A name, a noun, the exit codes, the environment and the signals stay in the program, behind parameters and hooks.
weight: 50
next: ["/concepts/architecture", "/packages/cliprogress", "/packages/cobratree"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

The kit is shared by programs with other names, other targets and other
rules for their exit codes. So the kit decides nothing that a program
must decide. What depends on a program is a parameter or a hook.

{{< fig caption="What the program reads and decides, and what it hands the kit." >}}
<svg viewBox="0 0 820 250" role="img" aria-label="The program reads the environment, flags and signals, and passes values and hooks to the kit.">
<g font-size="14">
<rect x="10" y="20" width="330" height="210" rx="12" fill="var(--deck)" stroke="var(--isl-command)"/>
<text x="30" y="50" font-weight="700" fill="var(--isl-command)">The program</text>
<g class="mono" font-size="13" fill="var(--text)">
<text x="30" y="82">os.Getenv("NO_COLOR"), TERM</text>
<text x="30" y="106">--progress, CLUSTERCTL_PROGRESS</text>
<text x="30" y="130">signal.NotifyContext(SIGINT)</text>
<text x="30" y="154">"hosts", "clusterctl"</text>
<text x="30" y="178">exit codes, os.Exit</text>
<text x="30" y="202">its paths, its version</text>
</g>
<rect x="480" y="20" width="330" height="210" rx="12" fill="var(--deck)" stroke="var(--isl-progress)"/>
<text x="500" y="50" font-weight="700" fill="var(--isl-progress)">The kit</text>
<g class="mono" font-size="13" fill="var(--text)">
<text x="500" y="82">ThemeFor(t, onTerminal, getenv)</text>
<text x="500" y="106">Options.Mode, Options.Log</text>
<text x="500" y="130">the end of a context</text>
<text x="500" y="154">Options.Noun, .Program</text>
<text x="500" y="178">Classify, Coded, Status</text>
<text x="500" y="202">fsutil, BuildInfo</text>
</g>
<g stroke="var(--mute)" stroke-width="1.5" marker-end="url(#pa)">
<line x1="345" y1="78" x2="474" y2="78"/><line x1="345" y1="102" x2="474" y2="102"/><line x1="345" y1="126" x2="474" y2="126"/>
<line x1="345" y1="150" x2="474" y2="150"/><line x1="345" y1="174" x2="474" y2="174"/><line x1="345" y1="198" x2="474" y2="198"/>
</g>
<defs><marker id="pa" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0L10 5L0 10z" fill="var(--mute)"/></marker></defs>
</g>
</svg>
{{< /fig >}}

## What the kit never does

- **It reads no environment.** Whether to draw in colour, `NO_COLOR` and
  `TERM` included, is the program's to decide. `cliprogress` is told what
  the program read.
- **It sets nothing process-wide.** It installs no signal handler and sets
  no global, such as the default logger.
- **It ends no process.** An error goes back to the program, which picks the
  exit status.

The linter refuses the calls that read or set the environment, handle
signals or end the process, outside the tests (decision 21).

## The hooks

| What the program decides | Where it tells the kit |
| --- | --- |
| Its name, in a line that says something panicked | `BusOptions.Program`, `cliprogress.Options.Program` |
| The noun for its targets: "1 host", "480 hosts" | `PlainOptions.Noun`, `MapOptions.Noun`, `cliprogress.Options.Noun` |
| The class of an error, for its exit codes | {{< api "progress" "BusOptions.Classify" >}}, `MapOptions.Classify` |
| The exit status of an error | {{< api "cobratree" "Coded" >}} where the cause is known, {{< api "cobratree" "Status" >}} in `main` |
| The usage errors, for their exit status | {{< api "cobratree" "Usage" >}}, {{< api "cobratree" "IsUsage" >}} |
| The flag and variable that pick a display and a log | `cliprogress.Options.Mode` and `.Log`, a {{< api "progress/cliprogress" "Setting" >}} each |
| Whether standard error is a terminal, its size, the locale | `cliprogress.Options.OnTerminal`, `.Size`, `.ASCII` |
| The theme, and the colours | {{< api "progress/cliprogress" "ThemeFor" >}} |
| The interrupt | the end of the context given to `cliprogress.Start` |
| The trace another program handed on | {{< api "progress" "ParseTraceContext" >}} of what the program read from `TRACEPARENT` |
| Where its files live, and how long it waits for a lock | the paths it passes `fsutil`, the wait it passes `fsutil.Lock` |
| The checks of `doctor`, and the exit status of a failed one | `doctor.Write` and {{< api "doctor" "Worst" >}} |

The [architecture](../architecture/#what-a-program-fills-in) has the whole
table, for every package.
