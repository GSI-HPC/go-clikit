---
title: cobratreetest
description: Run a command tree in a test, and check the command lines that a manual shows against the tree.
import: cobratree/cobratreetest
weight: 130
next: ["/packages/cobratree", "/guides/command-tree"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

{{< api "cobratree/cobratreetest" "Run" >}} runs the tree with the words of a
command line, and returns standard output and standard error apart. A
program writes its result to the one and its diagnostics to the other, and a
test of either must tell them apart.

{{< example "cobratree/cobratreetest" "ExampleRun" >}}

## Keep the manual true

{{< api "cobratree/cobratreetest" "CheckCommandLines" >}} reads text as a
shell reads a script, finds the command lines that run the program, and
checks that each names a command and flags that the tree has. When you
rename a command or drop a flag, the test of the manual that still shows it
fails.

```text
$ prog get cluster --realm dev
eval "$(prog env dev)"
prog power on n[1-4] \
    --count 2 2>/dev/null
```

{{< example "cobratree/cobratreetest" "ExampleCheckCommandLines" >}}
