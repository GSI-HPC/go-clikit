<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# The event log

`progress.Log` writes the events of a Bus as JSON, one object per line: an
event log, for a bug report, a CI job's artifacts, a test, or a converter to
another tracing system. The displays are for people and may change their text
from one release to the next; the log is for programs, and its format is an
interface of the kit, versioned apart from the module. This document is its
reference. `progress/testdata/log-v1.jsonl` is version 1, line for line, and
the tests fail when the log writes anything else.

How a program lets its users ask for a log, by a flag or a variable, and
where it writes it, is the program's to say in its manual.

## Lines

The first line of a run says what the run is; every line after it is one
event:

```json
{"v":1,"type":"trace","run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","parent":"00f067aa0ba902b7","traceFlags":"01","traceState":"rojo=00f067aa0ba902b7","program":"prog","version":"v1.2.3"}
{"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":5,"time":"2026-09-25T12:00:00.000000000Z","type":"start","span":"b7e3a1c09d2f4e10","parent":"b7e3a1c09d2f4e0f","kind":"target","name":"exe0001","state":"queued","node":"exe0001","host":"exe0001.hpc.example.org","role":"compute"}
```

Several runs may append to one file, the commands of one CI job for
instance, and may share one trace; `run` tells their lines apart, and the
lines of runs written at once do not cut into each other, since each run
writes whole lines.

## The first line

| Key | Holds |
| --- | --- |
| `v` | The version of the format, `1` |
| `type` | `trace` |
| `run` | The run, 16 hexadecimal digits drawn for each `Log` unless `LogOptions.Run` names it, the same on every line of the run |
| `trace` | The trace the run's spans belong to, 32 hexadecimal digits |
| `parent`, `traceFlags`, `traceState` | When the run continues a trace another program began, as `ParseTraceContext` reads it from a W3C `traceparent` and `tracestate`: the span of that program's the run works under, 16 hexadecimal digits; the trace flags, two; and the tracestate as it was given |
| `program`, `version` | The program that wrote the log and its version, as `LogOptions` name them |

## An event

| Key | Holds |
| --- | --- |
| `v` | The version of the format, `1` |
| `run`, `trace` | As on the first line |
| `seq` | The event's number, from 1 with no gaps within a run: the one order every sink saw |
| `time` | When the event happened, in UTC, as RFC 3339 with nine digits after the second, so that the times of one run sort as they happened |
| `type` | `start`; `run`, a queued span that starts to run; `update`; `line`, a line of output; `end`; `suspend` and `resume`, the displays taken off the terminal for a question and put back |
| `span`, `parent` | The span the event is about and the one it was started under, 16 hexadecimal digits each; the root has no `parent`. A `suspend` or `resume` has only a `span`, the one it was asked for under, if any |
| `kind` | `command`, `step`, `batch`, `target`, `call` or `wait` |
| `name` | What the span is, in few words |
| `flags` | A list of `hidden`, `fold`, `show-lines` and `dry-run` |
| `state` | `queued`, `running` or `ended`, once the event has happened |
| `node`, `host`, `role` | What a target or a call is for, a node or whatever else the program's targets are, and the address and role the work goes to |
| `total`, `limit`, `batch` | How many targets a step or a batch expects, how many it works on at once, and a batch's place among its step's, `2/5` |
| `message` | One short line for a display, such as the question a confirmation asks |
| `method`, `path`, `httpStatus` | The request of an HTTP call, and the status of its answer |
| `cache`, `source` | Where a lookup was answered from, `hit`, `miss`, `memory` or `disk`; and the kind of source a secret or a credential was read from, never the value |
| `timeout` | The bound of a call, or the length of a wait, in seconds |
| `exit` | The exit code of a remote command; `0` is written, and a command that gave none has no `exit` |
| `status` | How the span ended: `ok`, `failed`, `canceled` or `skipped` |
| `class` | Why a span that failed did: `target`, `transport`, `timeout`, `auth`, `pin`, `usage` or `canceled` |
| `err` | The error the span ended with, on one line |
| `stream` | The stream a line of output came from, `stdout` or `stderr` |
| `dropped` | The lines of output the sinks were not sent: on a `line`, those left out since the span's previous line, and on an `end`, all of the span's |

A key whose value is zero or empty is left out, but for `exit`. `name`,
`message` and `err` are text for people, not names to match: a program may
change them in any release. Every other value is one this document lists.

## Versions

`v` is `progress.LogVersion`. It changes only when what a key or a value
means changes, and such a change is a breaking change of the kit, named in
its release notes. Under version 1:

- a key or a value may be added, in a minor release, and a reader leaves out
  the keys and the values it does not know rather than failing on them;
- a key is never renamed or removed, and a value never changes its meaning;
- the golden log is changed only by adding to it.

The version of the log is not the version of the module: a release of the
kit that adds nothing to the log writes the same version, and a program that
reads logs written by several releases reads each line by its `v`.

## What never goes in

A span can say only what `progress.Fields` holds, so the log has no place for
an argument vector, a script, standard input, the environment, a header or
the value of a secret or a credential. A `line` event says which stream the
line came from, and nothing of what it said: the text of a line never leaves
the process. The log asks for no lines of output itself, so there are `line`
events only while a display asks for them.

The text that does go in, a name, a message or an error, has been through
`progress.Sanitize`, as every text in an event has: nothing in it acts on a
terminal, and it keeps to its bound. An error may still name a host and say
why it failed, so a program that writes the log to a file creates it
readable by its owner alone.

## Writing

The log writes from a goroutine of its own, never under the Bus's lock, in
whole lines: once 64 KiB wait, as the command, a step or a batch ends, and a
second after a line came at the latest, so that a log followed as it grows,
or that of a run killed, falls little behind. A write that fails stops the
log, and so does a writer that falls 8 MiB behind, a pipe whose reader has
stopped: the log never holds the work up. A log that falls behind still
writes the lines it had taken, should its writer take them after all, and
leaves out those that come after, so that what is written is the start of
the run without a gap. `Close` writes what is left, waiting five seconds at
most, and returns the error that stopped the log short, if one did.

## The golden log

`progress/testdata/log-v1.jsonl` is a run that says every value of every key
this document lists, but `program`, which came after it and which a test of
its own checks on the first line. The span ids, which each Bus draws at
random, are written as `#1`, `#2` and so on in the order they first appear. The test
that writes it compares what the log writes with it, line for line, and
fails on any difference; another test sets every part of an `Event` and fails
when one is neither logged nor left out on purpose, and a third fails when a
value has no name in the log. `go test ./progress -run TestTheEventLog
-update` rewrites the file, and the difference is then a change to the
format, reviewed as one: it keeps version 1 only when it adds.
