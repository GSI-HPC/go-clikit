---
title: cobratree
description: Make a cobra command tree strict, mark the errors of a command line as usage errors, and read the build info.
import: cobratree
weight: 120
next: ["/guides/command-tree", "/packages/cobratreetest"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

cobra is lenient where a script needs it strict. A command that only holds
subcommands succeeds for a word that names none of them. `help` succeeds for
a topic that names no command. And an argument check returns an error that
names neither the arguments nor the command.
{{< api "cobratree" "Strict" >}} fixes the three once the tree is built:

```text
prog gte          unknown command "gte" for "prog" (did you mean "get"?)
prog status       unknown command "status" for "prog" (it is now "get cluster")
prog help crate   unknown help topic "crate": unknown command "crate" for "prog" (did you mean "create"?)
prog power on     requires at least 1 arg(s), only received 0 (usage: prog power on NODES)
```

{{< example "cobratree" "ExampleStrict" >}}

## The main names

| Name | What it does |
| --- | --- |
| {{< api "cobratree" "Strict" >}}, {{< api "cobratree" "Moved" >}}, {{< api "cobratree" "Topics" >}} | the strict tree; renamed commands; help topics that are no command |
| {{< api "cobratree" "Usage" >}}, {{< api "cobratree" "Usagef" >}}, {{< api "cobratree" "IsUsage" >}} | mark and tell a usage error, for the exit code |
| {{< api "cobratree" "InvalidValue" >}}, {{< api "cobratree" "FlagError" >}} | one form for a value refused by pflag or by the program |
| {{< api "cobratree" "Coded" >}}, {{< api "cobratree" "Status" >}} | an exit status where the cause is known, read in `main` |
| {{< api "cobratree" "ReadBuildInfo" >}}, {{< api "cobratree" "BuildInfo" >}} | the version, the commit and its time, for a version command |

{{% call note %}}
The package names no exit code. A program maps a usage error to its own,
commonly 2.
{{% /call %}}

## Fixed interfaces

The texts of the errors that the checks of `Strict` return
({{< decision 24 >}}), and those of `InvalidValue` and `FlagError`
({{< decision 29 >}}). cobra and pflag are imported here and in
`cobratreetest` alone.
