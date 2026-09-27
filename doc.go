// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package clikit introduces the runtime that GSI-HPC's command-line tools
// share. It holds no API: each part of the kit is a package of its own,
// imported by its path:
//
//	github.com/GSI-HPC/go-clikit/termtext               escaping untrusted text for a terminal, and display width
//	github.com/GSI-HPC/go-clikit/progress               spans carried in a context.Context, the JSONL event log, TRACEPARENT
//	github.com/GSI-HPC/go-clikit/progress/display       the live tree, the counter, plain lines for CI, the summary
//	github.com/GSI-HPC/go-clikit/progress/progresstest  capturing and checking the events a command reports
//	github.com/GSI-HPC/go-clikit/fanout                 bounded worker pools that report their targets as progress
//
// The packages are still being moved here; until then none of them exists.
package clikit
