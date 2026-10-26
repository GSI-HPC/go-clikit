---
title: doctor
description: Report the checks of a doctor command in fixed words, as text for a person and as JSON for a script.
import: doctor
weight: 100
next: ["/guides/doctor"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A doctor command looks at the machine and says what is wrong. `doctor`
holds the checks and writes them:

```text
✓ Docker Engine: 28.1.1 (>= 25.0)
✗ cgroupv2: not mounted (sind requires cgroupv2)
! inotify: max_user_instances 128 (optional)

Boot with the unified cgroup hierarchy.

Raise the limit: sudo sysctl fs.inotify.max_user_instances=1024
```

| Name | What it does |
| --- | --- |
| {{< api "doctor" "Check" >}} | one check: name, status, detail, remediation |
| {{< api "doctor" "Status" >}} | {{< api "doctor" "StatusOK" >}}, {{< api "doctor" "StatusWarning" >}}, {{< api "doctor" "StatusFailed" >}} |
| {{< api "doctor" "Write" >}} | the checks for a person, with the remediations |
| {{< api "doctor" "Worst" >}}, {{< api "doctor" "Count" >}} | sum up a list, for the exit status |

{{< example "doctor" "ExampleWrite" >}}

## Fixed interfaces

The status words, the JSON keys `name`, `status`, `detail` and
`remediation`, and the text that `Write` writes ({{< decision 38 >}}). The
checks, their texts and the exit status are the program's.
