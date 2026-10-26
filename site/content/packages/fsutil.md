---
title: fsutil
description: Write a file whole or not at all, change a shared file under a lock, and refuse a file that another user could have written.
import: fsutil
weight: 110
next: ["/guides/shared-files"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

`fsutil` writes, locks and checks the files of a program that several
users, or several runs, share ({{< decision 37 >}}).

{{< steps caps="WriteAtomic writes a temporary file in the same directory.|It syncs the file and closes it.|It renames the file over the old one, and syncs the directory. A reader sees the old file or the new one, never a part." >}}
<svg viewBox="0 0 820 160" role="img" aria-label="A temporary file is written, synced and renamed over the target.">
<g class="mono" font-size="13">
<rect x="20" y="40" width="240" height="80" rx="10" fill="var(--deck)" stroke="var(--isl-files)"/>
<text x="36" y="66" fill="var(--isl-files)">~/.local/state/prog/</text>
<rect x="36" y="78" width="200" height="30" rx="6" fill="var(--well)" stroke="var(--line)" data-step="1"/>
<text x="48" y="98" fill="#d0d0d0" data-only="1">a temporary file</text>
<text x="48" y="98" fill="#d0d0d0" data-only="2">temporary file ✓ synced</text>
<text x="48" y="98" fill="#5fd7ff" data-only="3">state.json (new)</text>
<g data-step="3"><path d="M260 92 H400" stroke="var(--mute)" stroke-width="1.5"/><text x="290" y="84" fill="var(--mute)">rename(2)</text></g>
<rect x="420" y="40" width="380" height="80" rx="10" fill="var(--deck)" stroke="var(--line)"/>
<text x="436" y="70" fill="var(--text)">a reader opens state.json:</text>
<text x="436" y="98" fill="var(--run)">old, or new, never a part</text>
</g>
</svg>
{{< /steps >}}

| Name | What it does |
| --- | --- |
| {{< api "fsutil" "WriteAtomic" >}} | a file replaced whole; a file that exists keeps its mode and group |
| {{< api "fsutil" "ExactMode" >}} | gives a file that exists exactly the mode too |
| {{< api "fsutil" "Lock" >}} | an exclusive lock in a file beside the shared file |
| {{< api "fsutil" "Update" >}} | changes a file under its lock; waits {{< api "fsutil" "DefaultLockWait" >}} |
| {{< api "fsutil" "EnsureDir" >}} | a private directory of the user's |
| {{< api "fsutil" "CheckTrusted" >}} | refuses a file that a user other than this one or root could have written |
| {{< api "fsutil" "ErrUntrusted" >}} | what each refusal matches with `errors.Is` |

{{< example "fsutil" "ExampleLock" >}}

## Fixed interfaces

The name and the mode of the lock file, and `ErrUntrusted`. The texts of the
errors are not fixed.
