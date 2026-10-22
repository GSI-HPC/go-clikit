// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package clikit introduces the runtime that GSI-HPC's command-line tools
// share. It holds no API: each part of the kit is a package of its own,
// imported by its path:
//
//	github.com/GSI-HPC/go-clikit/termtext                 escaping untrusted text for a terminal, and display width
//	github.com/GSI-HPC/go-clikit/progress                 spans carried in a context.Context, the JSONL event log, TRACEPARENT
//	github.com/GSI-HPC/go-clikit/progress/display         the live tree, the counter, plain lines for CI, the summary
//	github.com/GSI-HPC/go-clikit/progress/cliprogress     the words of --progress, a command's display, summary and event log
//	github.com/GSI-HPC/go-clikit/progress/progresstest    capturing and checking the events a command reports
//	github.com/GSI-HPC/go-clikit/progress/mcpprogress     a tool call's progress as MCP progress notifications
//	github.com/GSI-HPC/go-clikit/progress/eventlog        reading back the event log a program or its child wrote
//	github.com/GSI-HPC/go-clikit/fanout                   bounded worker pools that report their targets as progress
//	github.com/GSI-HPC/go-clikit/fanout/fanouttest        counting the calls a fake is in the middle of, to prove a limit
//	github.com/GSI-HPC/go-clikit/cliout                   a command's result as a table and as JSON, escaped for a terminal
//	github.com/GSI-HPC/go-clikit/cobratree                a strict cobra command tree, usage errors, the build info
//	github.com/GSI-HPC/go-clikit/cobratree/cobratreetest  running a command tree in a test, checking a manual's command lines
//	github.com/GSI-HPC/go-clikit/localweb                 the guard of a local web server: listener, Host and cross-site checks, headers
//	github.com/GSI-HPC/go-clikit/localweb/session         the launch token exchanged for a session cookie, and the CSRF token
//	github.com/GSI-HPC/go-clikit/localweb/sse             a page's live fragments as Server-Sent Events, and the topics they change in
//	github.com/GSI-HPC/go-clikit/localweb/localwebtest    a signed-in client and a reader of event streams, for a test of a local server
//	github.com/GSI-HPC/go-clikit/localweb/launch          one server per user, and the browser opened on it without the token in a command line
//	github.com/GSI-HPC/go-clikit/localweb/assets          a page's static files at addresses with a hash, cached for good
//	github.com/GSI-HPC/go-clikit/localweb/page            a page's templates in one set, and the answers to htmx
//	github.com/GSI-HPC/go-clikit/localweb/htmx            htmx, its SSE extension and idiomorph, checked against their releases
//
// A library package reports what it does through progress, which costs
// nothing when no command is listening, and runs work for many targets with
// fanout. A command decides how the work is shown: it creates a
// progress.Bus, attaches a display from progress/display and, if asked, an
// event log, and passes the Bus down in its context.Context, or has
// progress/cliprogress do so from its flags; an MCP server sends a tool
// call's progress with progress/mcpprogress, and a program reads back the
// event log it or a child wrote with progress/eventlog. A command built with cobra
// makes its tree strict about its command line with cobratree, and one
// that serves a page to its user guards the server with localweb. The kit
// knows no program: a program's name, the noun for its targets and its
// rule for exit codes are parameters.
package clikit
