---
title: Make a command tree strict
description: Make a cobra tree refuse a misspelt command, word the errors of the command line, and give errors exit codes.
weight: 55
next: ["/packages/cobratree", "/packages/cobratreetest"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

cobra is lenient where a script needs it strict. A command that only holds
subcommands prints its help and succeeds for a word that names none of
them, so a script with a misspelt command goes on to its next step.
{{< api "cobratree" "Strict" >}} fixes this once the tree is built.

## Procedure

1. Build the tree. Add the last command.
2. Call `Strict` on the root. Give it {{< api "cobratree" "Moved" >}} for the
   commands that you renamed, and {{< api "cobratree" "Topics" >}} for the
   help topics that are no command.
3. Set {{< api "cobratree" "FlagError" >}} as the `FlagErrorFunc` of the
   root.
4. In a command, refuse a value with {{< api "cobratree" "InvalidValue" >}}
   or {{< api "cobratree" "Usagef" >}}.
5. Where the cause of a failure is known, give its exit status with
   {{< api "cobratree" "Coded" >}}.
6. In `main`, map the error to the exit status:
   {{< api "cobratree" "IsUsage" >}} first, commonly 2, then
   {{< api "cobratree" "Status" >}}.

{{< example "cobratree" "ExampleStrict" >}}

What the user sees:

```text
prog gte          unknown command "gte" for "prog" (did you mean "get"?)
prog status       unknown command "status" for "prog" (it is now "get cluster")
prog power on     requires at least 1 arg(s), only received 0 (usage: prog power on NODES)
```

The texts of these errors are an interface (decisions 24 and 29).

## The version

{{< api "cobratree" "ReadBuildInfo" >}} returns what the toolchain recorded:
the module version, the commit and its time, and if the checkout was
modified. {{< api "cobratree" "BuildInfo.Resolve" >}} puts in place what a
release build set with `-ldflags`.

{{< example "cobratree" "ExampleBuildInfo_Resolve" >}}

## Test the tree

{{< api "cobratree/cobratreetest" "Run" >}} runs a command line in a test
and keeps the result apart from the diagnostics.
{{< api "cobratree/cobratreetest" "CheckCommandLines" >}} checks that each
command line of the manual names a command and flags that exist.

{{< example "cobratree/cobratreetest" "ExampleRun" >}}
