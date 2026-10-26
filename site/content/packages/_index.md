---
title: Packages
description: The 22 packages of the kit on one map, in four groups. An arrow points from a package to one that it imports.
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

Import each package by its own path. A library imports only what it
needs: `progress` and `fanout` pull in no terminal, no cobra and no
`net/http`. The map shows each import between the packages of the kit,
without the imports that a longer path already implies. Pick a package for
its page.
