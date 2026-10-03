<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Architecture

The kit is five packages that a command-line program and the library
packages it calls share: a library reports what it does, and the command
decides how that is shown. This document says where the packages came from,
how they fit together, what runs on which goroutine, what a program fills
in, and the convention for standard error that the programs using the kit
follow.

## How the kit came to be

The packages were written for
[clusterctl](https://github.com/GSI-HPC/clusterctl), a command-line tool
that administers HPC clusters and works on hundreds of hosts at a time, to
show how far such a command has got. They were written in its `internal/`
tree, where clusterctl's records of the decisions behind them still are:
[ADR 0021](https://github.com/GSI-HPC/clusterctl/blob/v0.4.0/doc/adr/0021-progress-as-our-own-events.md)
on reporting progress as events of its own, and
[ADR 0022](https://github.com/GSI-HPC/clusterctl/blob/v0.4.0/doc/adr/0022-bounded-pools-and-power-batches.md)
on the bounded pools.

[sind](https://github.com/GSI-HPC/sind), which runs Slurm clusters in Docker,
set out to report its progress the same way, from a library that other
programs import too. So clusterctl cut what tied the packages to it: its exit
codes became a hook, its name and the noun for its targets parameters, and
the escaper moved out of its output package. The packages then moved here
with their history, as they shipped in clusterctl v0.4.0, under paths of
their own.

## The packages

| Package | Holds | Imports |
| --- | --- | --- |
| `termtext` | Escaping untrusted text for a terminal, and the columns text takes there | `golang.org/x/text/width` |
| `progress` | Spans carried in a `context.Context`, the `Bus` that orders their events, `Tally`, `Sanitize`, the JSONL event log and the W3C trace context | `termtext` |
| `progress/display` | The `Terminal` a display shares with the command, the live `Tree`, the `Counter`, `Plain` lines and the `Summary` | `progress`, `termtext`, `go-nodeset` |
| `progress/progresstest` | `Capture`, `Check`, `Checked`, `Watch`, and `Screen`, a terminal for tests | `progress`, `termtext`, `go-nodeset` |
| `fanout` | `Each`, `Map` and `Batches`, the bounded pools that report their items as targets | `progress`, `go-nodeset` |

Every arrow points down the table: nothing imports `display`, `progresstest`
or `fanout`, `progress` imports only `termtext`, and `termtext` only
`golang.org/x/text`. A library imports `progress`, and `fanout` when it works
on many items; the command imports `display`; tests import `progresstest`.
`go-nodeset` folds the names of targets into node sets, `exe[0001-0480]`, in
the tree, the tests' trees and the summary of a pool that failed.
[Decision 3](decisions.md#3-what-the-kit-may-require) says what else the kit
may require, which is nothing without a record.

## Spans and the Bus

Work is a tree of spans, one level for each kind of unit: the command, its
steps, the batches of a staggered action, the targets a step works on, the
calls made for a target, and the waits in between. The six kinds are an
interface of the kit. A span travels in the `context.Context` the work is
done under, so a call nests under the target it is made for without being
handed down, and a library reports its work without a parameter for it.

A command makes a `Bus` with its sinks, puts it in its context with
`WithBus`, and starts the command's span. Every change to a span, a start,
a run, an update, a line of output or an end, is one `Event` of plain data,
numbered by `Seq`, and handed to every sink in that one order.

`Span.End` takes the error the work ended with. A nil error ends the span
ok; an error that is `progress.ErrSkipped`, as `errors.Is` tells, such as
one `progress.Skip` returns, ends it skipped, work left out on purpose,
before any rule for classes is asked, so that a program's rule cannot turn
a skip into a failure; any other error ends it failed or canceled, as
`progress.Classify` tells. `Span.Skip` is `End` with a skip.

Without a Bus in the context, `Start` returns the context as it is and a nil
`*Span`, whose methods do nothing. The path allocates nothing, which two tests
hold it to, so a library reports its work at no cost when no command is
listening.

What a span says is closed: the fields of `progress.Fields`, and nothing
else. There is no field for an argument vector, a script, standard input, the
environment or a header, so none of them can reach a display or the log.
Every text in an event has been through `Sanitize`, which applies carriage
returns the way a terminal does, escapes the rest with
`termtext.Escape` and cuts it to its bound.

A pool announces every target queued before the first runs, marks each
running as it takes its place, and ends it before it gives the place up, so a
display knows the whole of the work from the start, never counts more running
than the limit, and reaches its total however the work ends.
`progresstest.Check` holds every source of events to this, and `fanout`
keeps it without a line of the caller's.

## Concurrency

- **Sinks run under the Bus's lock.** The Bus calls every sink's `Handle`
  with its lock held, in `Seq` order. A sink updates memory and returns: it
  never blocks, never writes to a terminal or a file there, and never calls
  the Bus back. A sink that panics is taken off the Bus, whatever its type,
  and its stack goes to `BusOptions.PanicLog`. The Bus knows its sinks by
  where they are listed, never by comparing them, and releases its lock
  however a call under it ends, so neither a sink nor a clock that panics
  leaves it locked.
- **Displays draw on their own time.** `Start` gives a display a goroutine
  that draws from what `Handle` kept, ten times a second for the tree and
  the counter; a test calls `Draw` instead, on a clock it moves. The event
  log writes from a goroutine of its own too, in whole lines, and stops,
  rather than hold the work up, when its writer falls 8 MiB behind.
- **The terminal is lent.** `progress.Suspend` calls every sink that is a
  `Suspender` outside the lock, and returns once each display is off the
  terminal, so that a question can be asked there; the function it returns
  puts them back, even a display taken off the Bus for panicking in
  between. Suspensions nest.
- **One order for the terminal.** The command's own writes go through
  `Terminal.Writer`, which takes the display's region off first and lets it
  back only once a line has ended, so a question that waits for its answer is
  never drawn over. The region comes off one line a row, as it was drawn,
  so a line of the command's is never cleared, even on a terminal made
  narrower since (decision 9). Lines from goroutines beside the command,
  a log line or the stack of a panic a pool recovered from, go through
  `Terminal.Lines`,
  which holds them, whole lines only and bounded, while a question is asked
  or the command has a line open. The lines a display leaves for good are
  written above the region, ahead of whatever the command writes after the
  events they tell of.
- **The pools.** `fanout.Each` is the one loop every pool runs on: at most
  its limit at a time, and nothing started once the context has ended, so an
  interrupt stops every pool the same way. `Map` returns what each item came
  to in the order the items were given, whatever order they finished in, and
  turns a panic in one item's work, or in what it acquired and released for
  it, into that item's error, since `recover` reaches only its own
  goroutine. Work, an acquire or a release that calls `runtime.Goexit`, as
  `t.FailNow` does, fails its item with an error that says which of them
  did, rather than pass for done, and loses nothing else that broke the
  item. The end of the context, an interrupt or a deadline alike, ends the
  items it left out, and those whose acquire or work then gave up with an
  error other than a skip, canceled, with the context's error, while each
  outcome keeps the error it was given. Both end in the same class, so the
  class does not tell an interrupt from a deadline; the error text in the
  event log does, `context canceled` or `context deadline exceeded`, as
  does the error of an item the pool left out, and the context itself. A
  panic or a call of `runtime.Goexit` is a bug, and fails its item even
  once the context has ended. An item its acquire refused ends at once,
  before it gives its place up, as every target does. What an item
  acquired is released before its target ends, so that a panic in the
  release is the target's failure; the next item can so take it and run
  before that target has ended. `Batches` runs one batch after the other,
  with a pause between them, and stops after one that failed; a batch whose
  run returns a skip ends skipped and does not stop it. A skip is
  `progress.ErrSkipped`, which the pools test with `errors.Is`; the error
  of a batch not tried, `fanout.ErrNotTried`, is none, since a failure left
  that batch out, not a purpose (decision 13).

## What a program fills in

The kit knows no program. What depends on one is a parameter or a hook:

| What | Where |
| --- | --- |
| The program's name, in the line that says a sink, a display or a pool's work panicked, and in the error a panic becomes | `progress.BusOptions.Program`, `display.Terminal.Program`, `fanout.MapOptions.Program`, `fanout.Recovered` |
| The program's name and version on the event log's first line | `progress.BusOptions.Program`, which the Bus hands the log in `progress.Run`, or `progress.LogOptions.Program` in its place; `progress.LogOptions.Version` |
| The class of an error that says none of its own, such as the program's rule for its exit codes | `progress.BusOptions.Classify`, `fanout.MapOptions.Classify`, `progresstest.Classify` |
| The error a pool's step ends with, such as the program's exit code on the kit's summary | `fanout.MapOptions.Summarize`, with `fanout.Failure` as the default |
| The noun for the targets, "1 host" and "480 hosts" | `display.PlainOptions.Noun`, `fanout.Failure` |
| What a display names an item by, and what an item needs besides its place in the pool | `fanout.MapOptions.Describe`, `fanout.MapOptions.Acquire` |
| The terminal's size, whether the process is in its foreground, whether its locale shows UTF-8, and the interrupt | `display.NewTerminal`, `display.Terminal.Foreground`, the `ASCII` options, `display.TreeOptions.Interrupted` |
| The trace another program handed on | `progress.BusOptions.Trace`, as `progress.ParseTraceContext` reads it from the values the program read from `TRACEPARENT` and `TRACESTATE` |
| Where a panic's stack goes | the `PanicLog` options |

A Bus hands on what it was told: `Bus.Program`, `Bus.PanicLog` and
`Bus.Classify` answer for the Bus that `progress.BusFrom` finds in a
context, and for a nil Bus with the defaults, so that a library that runs
work of its own names the program, writes its panics and classes its errors
as the Bus does.

What stays in the program: its flags and environment variables, which display
to draw and whether the terminal can show one, its exit codes, and reading
the environment. The kit reads no variable and sets nothing process-wide, so
two Buses in one process, one per call of a server, share no state.

## Standard error

The programs that use the kit share one convention for what goes where, so
that a script that reads their streams, and a person who reads their
terminal, finds the same in each:

- **Standard output** carries the command's result, and nothing of its
  progress, whatever display is drawn.
- **Standard error** carries, in this order of precedence:
  - **progress**, drawn by a display only when the program chose one: the
    tree or the counter only on a terminal, plain lines wherever they are
    asked for;
  - **notes**, lines the command writes itself, such as a fallback it took
    or a pause it waits out, through `Terminal.Writer` while a display is
    drawn;
  - **logs**, lines from goroutines beside the command, through
    `Terminal.Lines`, so that none lands inside a question;
  - and last, once the display is gone, the summary a display leaves, and
    **one line for the error**, escaped with `termtext`, which says what
    failed and why.
- **Without a display**, standard error holds exactly what it would without
  the kit, byte for byte. A command done within its first second draws
  nothing and leaves no summary.

Text that came from elsewhere, a remote host, a container or a service, is
escaped with `termtext.Escape`, or `EscapeLines` where its lines are kept,
before it reaches a terminal, and there is no other escaper: a program that
escapes everything with it has one place to fix when a character turns out
to need escaping too. Decision 7 calls the two by their earlier names,
`EscapeCell` and `EscapeText`.
