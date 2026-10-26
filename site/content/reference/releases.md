---
title: Releases
description: Each release of the kit, newest first, with its notes. A breaking change in a v0 minor release is named here.
weight: 20
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

go-clikit uses semantic versions, and is at v0: a minor release can break
the API, and its notes name the break. Each release is a signed tag. A
release that is broken is retracted in `go.mod`, and its tag stays.

{{< releases >}}
