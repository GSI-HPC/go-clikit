<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Architecture

The kit is a set of packages that a command-line program and the library
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
their own. `progress/cliprogress` was written here afterwards, from the glue
between the command line and the progress packages that clusterctl and sind
each carried
([decision 21](decisions.md#21-the-command-lines-glue-for-progress-is-the-kits)),
and `cobratree` from the code both carried to make a cobra command tree
strict and to read the build info
([decision 24](decisions.md#24-the-strict-command-tree-is-the-kits-in-cobratree)).

## The packages

| Package | Holds | Imports |
| --- | --- | --- |
| `termtext` | Escaping untrusted text for a terminal, as a string or as a stream, the columns text takes there, and filling text to a number of them | `golang.org/x/text/width` |
| `progress` | Spans carried in a `context.Context`, the `Bus` that orders their events, `Tally`, `Meter`, `Sanitize`, the JSONL event log and the W3C trace context | `termtext` |
| `progress/display` | The `Terminal` a display shares with the command, the live `Tree`, the `Counter`, `Plain` lines and the `Summary` | `progress`, `termtext`, `go-nodeset` |
| `progress/cliprogress` | The words of `--progress` and the rule that picks a display from them, a command's `Session`, which makes and takes down its Bus, display, summary and event log, the private file the log is appended to, a default log kept unasked and moved aside, the colours a terminal shows, a question asked with the display off, and the probes of a command's streams | `progress`, `progress/display`, `termtext`, `golang.org/x/sys` |
| `progress/progresstest` | `Capture`, `Check`, `Watch` and its `Watcher`, and `Screen`, a terminal for tests | `progress`, `termtext`, `go-nodeset` |
| `progress/mcpprogress` | `Notifier`, a sink that sends a tool call's progress as MCP progress notifications, through a `Send` the server gives it | `progress` |
| `progress/eventlog` | `Decode`, which reads a line of the event log back into the first line of a run or one of its events, as a `progress.Event`; `Tail`, which reads a log as it grows and is rotated; `NewForwarder`, which gives the events of the run a child logged to a sink; and `Reducer`, which folds a log into runs, each the tree of its command's spans, for a view of what a program did | `progress` |
| `fanout` | `Each`, `Map` and `Batches`, the bounded pools that report their items as targets, and `Keyed`, which bounds the work under way on each key, such as a host | `progress`, `go-nodeset` |
| `fanout/fanouttest` | `InFlight`, which counts the calls a test's fake is in the middle of, and holds them to prove a limit | none |
| `cliout` | `Table`, a command's result in columns aligned by the columns a terminal gives them, and `WriteJSON`, every cell and string escaped | `termtext` |
| `doctor` | `Check`, one check of a doctor command, with its `Status`, `ok`, `warning` or `failed`; `Worst` and `Count`; and `Write`, which writes the checks for a person in a text that is an interface | `termtext` |
| `fsutil` | `WriteAtomic`, which replaces a file whole or not at all; `Lock`, a lock beside a file that several processes share, and `Update`, a change of the file under it; `EnsureDir` and `CheckTrusted`, which refuse a directory or a file that another user could have written | `golang.org/x/sys`, where the lock needs it |
| `cobratree` | `Strict`, which makes a cobra command tree strict about its command line, the usage errors and the words of a flag's value refused, exit statuses carried by errors, help topics, and the build info | cobra, pflag |
| `cobratree/cobratreetest` | `Run`, which runs a command tree in a test and keeps its standard output and standard error apart, and `CheckCommandLines`, which checks the command lines of a manual against the tree | `cobratree`, cobra, pflag |
| `localweb` | `Listen`, which binds a loopback address or a private Unix socket, `PeerListener`, which closes another user's connections, `ListenerOwner`, which names the user of a listening socket, the checks `Hosts`, `SameOrigin` and `Headers`, and `Serve`, which serves with every timeout set | none |
| `localweb/session` | `Manager`, which exchanges the launch token for a session cookie and holds each session's CSRF token, and `NewToken` | `localweb` |
| `localweb/sse` | `Notifier` and `Subscription`, which tell a stream the exact topics that changed, and `Serve`, which streams a page's fragments as Server-Sent Events, every one first and then those that changed | none |
| `localweb/localwebtest` | `Login`, a client that signs in and sends requests as the server's own pages do, and `Reader`, which reads the events of a stream | `localweb/session`, `localweb/sse` |
| `localweb/launch` | `Opener`, which opens the browser on an address through a private file, and `Instance`, the lock and the record that keep one server per user | `localweb`, `golang.org/x/sys` |
| `localweb/assets` | `Set`, which serves a page's static files at addresses with a hash of them all | none |
| `localweb/page` | `Set`, a program's templates in one set, which renders a block before it answers, and `IsHTMX`, `Redirect` and `Retarget` for the requests of htmx | none |
| `localweb/htmx` | `Files`, the scripts of htmx, its SSE extension and idiomorph, and `Config`, the configuration a page gives htmx | none |

Every arrow points down the table: nothing imports `cliprogress`,
`progresstest`, `mcpprogress`, `eventlog` or `fanout`, only `cliprogress` imports `display`,
`progress` imports only `termtext`, and `termtext` only
`golang.org/x/text`. `cliout` and `doctor` import only `termtext`, and
`fsutil` no package of the kit. `cobratree` stands
apart: it imports no package of the kit, and none imports it but
`cobratree/cobratreetest`. A library imports `progress`, and `fanout` when
it works on many items; the command imports `cliprogress`, or `display`
when it picks its displays by a rule of its own, an MCP server
`mcpprogress`, a program that reads back the event log it or a child
wrote `eventlog`, `cliout` to write its
result, `doctor` for its doctor command, `fsutil` for the files it
shares or keeps private, and `cobratree` when it is built with cobra; tests import
`progresstest`, `fanouttest` for a limit, and `cobratreetest` for a
command tree. The `localweb` packages stand apart as `cobratree` does,
on the web side: they import no package of the kit but their own, and
are the only ones that import `net/http` and `html/template`. `go-nodeset` folds the names of targets into node sets,
`exe[0001-0480]`, in the tree, the tests' trees and the summary of a pool
that failed.
[Decision 3](decisions.md#3-what-the-kit-may-require) says what else the kit
may require, which is nothing without a record;
decisions
[22](decisions.md#22-the-kit-may-require-golangorgxsys-in-cliprogress-alone),
[26](decisions.md#26-golangorgxsys-also-in-fsutil-and-localweb) and
[35](decisions.md#35-localweb-starts-a-server-once-opens-the-browser-on-it-and-serves-its-files-and-pages)
let `cliprogress`, `fsutil` and `localweb/launch` alone import
`golang.org/x/sys`, and
[decision 24](decisions.md#24-the-strict-command-tree-is-the-kits-in-cobratree)
`cobratree` alone cobra and pflag.

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
`progress.Classify` tells. `Span.Skip` is `End` with a skip. `errors.Is`
looks through every error an error joins, so one that joins a skip with a
failure, by `errors.Join` or two `%w` verbs, ends skipped, and the failure
goes uncounted: work that gathers the errors of its parts returns a skip
only when every part was skipped.

Without a Bus in the context, `Start` returns the context as it is and a nil
`*Span`, whose methods do nothing. The path allocates nothing, which two tests
hold it to, so a library reports its work at no cost when no command is
listening.

What a span says is closed: the fields of `progress.Fields`, and nothing
else. There is no field for an argument vector, a script, standard input, the
environment or a header, so none of them can reach a display or the log.
Every text in an event has been through `Sanitize`, which applies carriage
returns the way a terminal does, escapes the rest with
`termtext.Escape` and cuts it to its bound. The Bus calls it, so neither the
work that reports a span nor a sink needs to; a program that shows text from
elsewhere outside a Bus may call it too.

A pool announces every target queued before the first runs, marks each
running as it takes its place, and ends it before it gives the place up, so a
display knows the whole of the work from the start, never counts more running
than the limit, and reaches its total however the work ends.
`progresstest.Check` holds every source of events to this, and `fanout`
keeps it without a line of the caller's.

## Work

A span may also report its work: an amount in a unit, out of a size when one
is known (decision 23). The span's own `Advance`, `SetAmount`,
`AdvanceWriter` and `AdvanceReader` add to an atomic amount and nothing else, so
a copy that writes a hundred thousand times a second takes no lock. The Bus
samples that amount: it sends it as a `TypeAdvance` event at most every
100 ms a span, by its own clock, the newest amount never lost, and with the
span's `End`. `Advance` and `SetAmount` read that clock, `BusOptions.Now`,
on the goroutine that calls them and outside the Bus's lock, so the clock
must be safe for concurrent use. A span without a Bus costs nothing, as
everything else on a nil span.

An event carries only what its span said. `progress.Meter` is the sink-side
half: given every event in order, as `Tally` is, it keeps the work of each
span and rolls it up the tree, an amount within a unit, a share done from the
span's own size, else from the targets it counts, short of done until their
count is complete, else from the work below, where a span with a size of its
own counts for everything below it, and the rate, the time left and a stall from the amounts' times. The
displays each keep a `Meter` and read it at the time of a frame, so the
rows move between events; `progresstest.Capture` is sent every advance, and
`progresstest.Check` holds them to their rules.

The event log samples on its own. It writes an advance of a span at most once
a second by default (`LogOptions.AdvanceEvery`), by the events' times, and
counts the advances it leaves out in `leftOut`; they are the only gaps in
`seq`. The displays, the `Meter` and `Capture` miss none.

## Concurrency

- **Sinks run under the Bus's lock.** The Bus calls every sink's `Handle`
  with its lock held, in `Seq` order. A sink updates memory and returns: it
  never blocks, never writes to a terminal or a file there, and never calls
  the Bus back. A sink that panics is taken off the Bus, whatever its type,
  and its stack goes to `BusOptions.PanicLog`. The Bus knows its sinks by
  where they are listed, never by comparing them, and releases its lock
  however a call under it ends, so neither a sink nor a clock that panics
  leaves it locked. The clock, `BusOptions.Now`, is the one thing the Bus
  calls outside its lock as well, from `Advance` and `SetAmount`, so it must
  be safe for concurrent use.
- **Displays draw on their own time.** `Start` gives a display a goroutine
  that draws from what `Handle` kept, ten times a second for the tree and
  the counter; a test calls `Draw` instead, on a clock it moves. The event
  log writes from a goroutine of its own too, in whole lines, and stops,
  rather than hold the work up, when its writer falls 8 MiB behind.
  `mcpprogress.Notifier` sends from a goroutine of its own as well, at
  most every 500 ms and at once as a step or a batch ends; its `Close`,
  after the Bus's, sends what is left, so that nothing follows the
  answer to the call, and waits no longer than the context it is given.
  `fsutil.Lock` and `launch.Acquire` wait for their locks no longer than
  their contexts either.
  `eventlog.Tail` starts no goroutine: the program polls it from one of
  its own, and the Feed it gives the lines to, and a sink behind
  an `eventlog.Forwarder`, are called there, under no lock of the kit's.
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
  events they tell of. A `Terminal` carries one display, from the
  constructor that makes it to its `Close`, after which the writers pass
  bytes through; making a second display on it panics.
- **The pools.** `fanout.Each` is the one loop every pool runs on: at most
  its limit at a time, and nothing started once the context has ended, so an
  interrupt stops every pool the same way. `Map` returns what each item came
  to in the order the items were given, whatever order they finished in,
  with the error its step ended with, and turns a panic in one item's
  work, or in what it acquired and released for it, into that item's
  error, since `recover` reaches only its own goroutine; `Each` and
  `Batches` recover nothing, and a panic in a batch goes up the goroutine
  that called `Batches`. Work, an acquire or a release that calls
  `runtime.Goexit`, as `t.FailNow` does, fails its item with an error that says which of them
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
| The program's name, in the line that says a sink, a display or a pool's work panicked, and in the error a panic becomes | `progress.BusOptions.Program`, `display.TerminalOptions.Program`, `fanout.MapOptions.Program` in place of the Bus's, `fanout.Recovered`, `cliprogress.Options.Program` |
| The program's name and version on the event log's first line | `progress.BusOptions.Program`, which the Bus hands the log in `progress.RunInfo`, or `progress.LogOptions.Program` in its place; `progress.LogOptions.Version`; `cliprogress.Options.LogOptions` |
| The class of an error that says none of its own, such as the program's rule for its exit codes | `progress.BusOptions.Classify`, `fanout.MapOptions.Classify` in place of the Bus's, `progresstest.Classify`, `cliprogress.Options.Classify` |
| The error a pool's step ends with, such as the program's exit code on the kit's summary | `fanout.MapOptions.Summarize`, which is given a `fanout.Summary`, with a `*fanout.FailureError` as the default |
| The noun for the targets, such as "1 host" and "480 hosts"; Plain says "1 target" and "480 targets" without one | `display.PlainOptions.Noun`, `fanout.MapOptions.Noun` and `cliprogress.Options.Noun`, a function of the count in each |
| What a display names an item by, and what an item needs besides its place in the pool, such as the unit and size of its work | `fanout.MapOptions.Describe`, which returns a `fanout.ItemInfo`, and `fanout.MapOptions.Acquire` |
| The terminal's size, whether the process is in its foreground, whether its locale shows UTF-8, and the interrupt | `display.TerminalOptions`, the `ASCII` options, `display.TreeOptions.Interrupted`; `cliprogress.Options.Size`, `.Foreground` and `.ASCII`, which `cliprogress.TerminalSize`, `cliprogress.InForeground` and `cliprogress.UTF8Locale` find out, and the end of the context `cliprogress.Start` is given |
| The trace another program handed on | `progress.BusOptions.Trace`, as `progress.ParseTraceContext` reads it from the values the program read from `TRACEPARENT` and `TRACESTATE`; `cliprogress.Options.Trace`, which `cliprogress.Start` calls only once it makes a Bus |
| The trace a program hands a child it starts, such as its own command line run from a page of its web UI, and whether it passes one on at all | the program, which puts the `Traceparent` of `Span.TraceContext`, the trace context of the span the child works for, in the child's environment, so that the child's log names that span as its parent; a program without a Bus takes `progress.NewTraceContext`, read from `crypto/rand.Reader`, which starts a trace |
| Where a panic's stack goes | the `PanicLog` options |
| How the command line and the environment ask for a display and an event log: the names of the flag and of the variable that stands in for it, and what each holds | `cliprogress.Options.Mode` and `cliprogress.Options.Log`, a `cliprogress.Setting` each |
| Whether standard error is a terminal, and a dumb one, and whether standard output goes into a pipe | `cliprogress.Options.OnTerminal`, `.Dumb` and `.IntoPipe`; `cliprogress.IsTerminal` tells the first and `cliprogress.IsPipe` the last |
| An event log the program keeps unasked, and how large it grows | `cliprogress.Options.DefaultLog` and `.RotateAt` (decision 31) |
| Whether a command asks before it acts, where standard input is no terminal, and what an answer means | the program, which asks with `cliprogress.Ask`, tells a terminal with `cliprogress.IsTerminal` and reads a yes with `cliprogress.Yes` |
| Where the notes go, and the display and the summary | `cliprogress.Options.Notes`, `cliprogress.Options.Stderr` |
| Which of the program's writers, and its logger's, go through the display's Terminal | the writers of `cliprogress.Session.Writer` and `cliprogress.Session.Lines`, which the program puts in place of its own and puts back once `cliprogress.Session.Finish` has returned |
| Whether a command that succeeded leaves the summary | the argument of `cliprogress.Session.Finish` |
| How an error of the command line is reported | the program marks the errors `cliprogress.Choose` and `cliprogress.Start` return as its usage errors, with `cobratree.Usage` when it is built with cobra; `cobratree.Strict` marks those of its tree's argument checks, and `cobratree.IsUsage` tells them, and pflag's, from other errors, for the program's exit code; `cobratree.FlagError`, set as the tree's `FlagErrorFunc`, and `cobratree.InvalidValue` word a flag's value refused |
| The exit status of an error that is no usage error, such as that of a host that could not be reached | the program's numbers, given where the cause is known with `cobratree.Coded` and read in main with `cobratree.Status` |
| The topics help shows beside the commands, and their texts | the lookup and the list of names the program passes `cobratree.Topics`, an option of `Strict`, whose names `help` completes |
| The commands a program dropped, and what replaced them | `cobratree.Moved`, an option of `Strict` |
| Whether a tool call's progress is sent over MCP, and on which session, with which token | the server, which gives a call that came with a progress token a Bus of its own with an `mcpprogress.Notifier` among its sinks, and puts the token in what the `Send` of `mcpprogress.Options` sends; a server that runs the call as a child gives the child an event log of its own and polls it with `eventlog.Tail`, through the `eventlog.NewForwarder` of the trace it handed the child to the Notifier, while the child runs and once after |
| How a view of what the program did reads its event log: which log, how often it polls it, how many runs it keeps, and when a run that has not ended is taken for one that was killed | the program, which polls an `eventlog.Tail` on its own time into an `eventlog.Reducer`, keeps `eventlog.ReducerOptions.KeepRuns` runs, and judges a run by its `Last` event |
| How the program words its version: the word for a build that is no release, how much of the commit it shows, and a version injected at build time | the program, from the fields of `cobratree.BuildInfo`, which `cobratree.ReadBuildInfo` returns, and `BuildInfo.Resolve` with what a release build set with -ldflags; `BuildInfo.Short` cuts the commit to the digits the program shows |
| How the displays look: a theme, in how many colours, or none | `display.TreeOptions.Theme`, `display.CounterOptions.Theme`, `display.PlainOptions.Theme` and `display.SummaryOptions.Theme`, one of `display.Themes`, which `display.ParseTheme` reads, drawn in 256 colours, `Colors16` or `ColorsNone` by `Theme.In`; the zero `Theme` draws in no colour, as before (decisions 19 and 20); `cliprogress.Options.Theme`; `cliprogress.ThemeFor` picks the colours from what the program read of NO_COLOR, TERM and COLORTERM |
| Where a local web server listens, which user it serves, and how a refusal is written and logged | the address the program passes `localweb.Listen`, the uid it passes `localweb.PeerListener`, and the `Refuse` field, a `localweb.Refuser`, of each check's options; the program prints the address and opens the browser |
| The names of a local web server's session cookie and CSRF header, and the line a refusal adds to tell the user where the address is | `session.Options.CookiePrefix`, `.CSRFHeader` and `.Refuse`; the program makes the token with `session.NewToken` and prints it in the address |
| Which fragments a page streams, the topics each depends on, and how long changes gather | the program's store, which calls `sse.Notifier.Notify`, the render it passes `sse.Serve`, and `sse.Options` |
| What a page may load, and which other origins a development server adds | the policy the program passes `localweb.Headers`, `""` for `localweb.ContentSecurityPolicy`, and `localweb.OriginPolicy.Others` |
| Where the record of a running server lives, and whether a browser opens | the runtime directory the program passes `launch.Acquire`, and a directory of its own without one; the program calls `launch.Opener.Skip` and `Open`, prints the address in any case, and removes the redirect file when it stops |
| The files a page loads and the templates it renders, and working on them without a build | the `fs.FS` and prefix the program passes `assets.New`, the patterns and functions it passes `page.Parse`, and `assets.Options.Dev` and `page.Options.Dev` on a directory of its own |
| The checks of a doctor command, their texts, and the exit status of a failed one | the program, which writes its `doctor.Check` values with `doctor.Write` or as JSON, and picks its exit status from `doctor.Worst` |
| Where a program keeps its files, which of them it shares, and how long it waits for a lock | the paths the program passes `fsutil`, and the wait it passes `fsutil.Lock`; `fsutil.Update` waits `fsutil.DefaultLockWait` |

A Bus hands on what it was told: `Bus.Program`, `Bus.PanicLog` and
`Bus.Classify` answer for the Bus that `progress.BusFrom` finds in a
context, so that a library that runs work of its own names the program,
writes its panics and classes its errors as the Bus does. A nil Bus
answers "", nil and `progress.Classify` without a fallback; the defaults
are the caller's. `fanout.Map` takes them so: its own `Program`,
`PanicLog` and `Classify` override the Bus's, and without a Bus or an
option of its own it falls back to "the program", standard error and
`ClassTarget`. The
rule that tells an item canceled is the one that classes its target, so
the step and its targets agree (decision 12).

What stays in the program: the names of its flags and variables, and
reading them, its own, NO_COLOR, TERM, COLORTERM and the locale's among
them; whether a stream is a terminal; whether to draw in colour and in how
many colours (from NO_COLOR, TERM, COLORTERM and whether standard error is a
terminal); taking TRACEPARENT and TRACESTATE out of its environment; its
signals and its exit codes; which commands show progress, and the span each
runs in; and putting its own writers, and its logger's, through the
Terminal while a display is drawn, and back. `cliprogress` picks the
display, and whether the terminal can show one, from what the program tells
it, and makes and takes down the Bus, the display, the summary and the
event log; a program that picks by a rule of its own uses `display`
directly. The kit reads no variable, sets nothing process-wide and ends no
process, so two Buses in one process, one per call of a server, share no
state.

## The local web server

sind, brig and sdash serve a page to the user who started them, on the
user's own machine. Such a server acts as the user, so whoever can send it
a request acts as the user too: another user of the machine, who can reach
a loopback port; any page in the user's browser, which can send requests
there; and a page of another site that makes its own name resolve to
127.0.0.1. `localweb` keeps the three out, each by a check of its own
(decision 34):

- **The listener.** `Listen` binds a loopback address and nothing else, or
  a Unix socket that nobody but the user can reach or replace. On TCP,
  `PeerListener` closes, on Linux, a connection that a process of another
  user opened, before a request is read.
- **The Host.** `Hosts` refuses a request whose Host header is not one
  `Bound.Answers` names, which is how DNS rebinding shows: such a page
  names its own site. localhost is answered only while the server holds
  its port on both 127.0.0.1 and `::1`.
- **The site.** `SameOrigin` refuses what a page of another site caused, by
  `Sec-Fetch-Site` and `Origin`, which a browser sets and no page can
  change. Every port of 127.0.0.1 is the same site, so the SameSite
  attribute of a cookie keeps out none of the pages other programs serve
  there. A page lets in a navigation from anywhere (`OriginPolicy.Navigate`);
  what a page asks for itself, its fragments, its stream and its forms, is
  let in only from its own origin.
- **The headers.** `Headers` sets the policy that lets a page load and
  contact its own origin alone, and forbids framing, sniffing and the
  referrer to other sites.

Behind them, `localweb/session` lets in only the browser that opened the
address the program printed: the launch token in it is exchanged for a
session cookie, and a request that may change something carries the
session's CSRF token.

`localweb/sse` streams the live parts of a page. The program's store
notifies the topics it changes; a stream subscribes, sends every
fragment of the page first, since the page may be older than a change
the stream did not hear of, and then each fragment whose topic changed
and whose HTML differs from what it last sent. A subscriber that falls
behind keeps the set of topics it missed, so it renders no more than
changed. The stream is let in from the page's own origin alone.

A program chains them, outermost first: `Headers`, `Hosts`, `SameOrigin`,
`session.Manager.Middleware` and its handler; `Serve` serves the chain with every timeout set. Each
check answers a refused request through a `Refuser`, which the program may
give to write it in its own form and log the reason; the messages are an
interface of the kit.

Around the server, `localweb/launch` keeps one per user and opens the
browser on it (decision 35). A start takes the lock of
`Acquire`, and waits for it no longer than its context; when `Running` finds the record of a server that still runs,
whose listening socket is the user's, the start opens that one and ends;
otherwise it listens, `Publish`es its address and drops the lock. The
`Opener` writes the address, token and all, into a file only the user can
read, and hands that file to the desktop's opener, as a command line is
readable by every user. `localweb/assets` serves the page's files under a
hash of them all, so a browser keeps each for good; `localweb/page`
renders the page and its fragments from one template set, the same
blocks the stream of `sse` sends; and `localweb/htmx` holds the scripts
that swap them in, checked against their releases.

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
which keeps a CRLF line end as it is (decision 42), before it reaches a
terminal, and there is no other escaper: a program that
escapes everything with it has one place to fix when a character turns out
to need escaping too. Decision 7 calls the two by their earlier names,
`EscapeCell` and `EscapeText`. The forms of the escapes are stable, but the
runes escaped may grow in a minor release (decision 14). `termtext.Width`
counts the columns of text that is already escaped. Decision 8 names
`RuneWidth`, the width of one rune, which is no longer exported:
`Width(string(r))` gives it.
