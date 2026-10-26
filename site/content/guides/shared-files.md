---
title: Keep files safe
description: Replace a file whole, change a shared file under a lock, and refuse a file that another user could have written.
weight: 70
next: ["/packages/fsutil"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A tool that several users, or several runs, share has three problems with
its files. A reader can see a file written in part. Two runs can change a
shared file at once, and one change is lost. And a file that another user
can write can make the tool run their commands. `fsutil` solves the three
(decision 37).

| Problem | Use |
| --- | --- |
| A reader sees half a file | {{< api "fsutil" "WriteAtomic" >}}: a temporary file, a sync and a rename |
| Two runs change one file | {{< api "fsutil" "Update" >}}: the change under {{< api "fsutil" "Lock" >}} |
| Another user put the file there | {{< api "fsutil" "CheckTrusted" >}} and {{< api "fsutil" "EnsureDir" >}}, which refuse with {{< api "fsutil" "ErrUntrusted" >}} |
| A private file was made readable | {{< api "fsutil" "ExactMode" >}} |

## Change a shared file

1. Give `Update` the path, the mode and a function that changes the old
   content into the new.
2. `Update` takes the lock beside the file, waits at most
   {{< api "fsutil" "DefaultLockWait" >}}, calls the function, and writes the
   result whole.
3. When the wait passes, the error is a timeout and names the holder of the
   lock.

{{< example "fsutil" "ExampleUpdate" >}}

## Refuse what others wrote

{{< example "fsutil" "ExampleCheckTrusted" >}}

{{% call warning %}}
Read configuration that names commands to run only after `CheckTrusted`
accepts it. A file that another user could write gives that user the
commands of your program.
{{% /call %}}

The name and the mode of the lock file are an interface of the kit, so two
releases of a program lock the same file.
