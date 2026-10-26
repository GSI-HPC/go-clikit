---
title: Write a doctor command
description: Check the machine, and write each check as a line for a person or as JSON for a script, in fixed words.
weight: 60
next: ["/packages/doctor"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

A doctor command looks at the machine and says what is wrong and what to do
about it. {{< api "doctor" "Check" >}} holds one check: a name, a status, a
detail and a remediation.

## Procedure

1. Make a `Check` for each thing that you look at. Use
   {{< api "doctor" "StatusOK" >}}, {{< api "doctor" "StatusWarning" >}} or
   {{< api "doctor" "StatusFailed" >}}.
2. Write the checks for a person with {{< api "doctor" "Write" >}}, or as
   JSON with `cliout.WriteJSON`.
3. Pick the exit status from {{< api "doctor" "Worst" >}}.

{{< example "doctor" "ExampleWrite" >}}

{{< example "doctor" "ExampleCheck" >}}

{{% call caution %}}
`Worst` fails closed. A status that is none of the three words counts as
failed.
{{% /call %}}

The status words, the JSON keys and the text that `Write` writes are an
interface (decision 38). A script can read them.
