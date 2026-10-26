---
title: "Tour: your first command"
linkTitle: Tour
description: Follow one command from the first span to its test. Each step adds one package of the kit, and each code block is an example that CI runs.
weight: 1
next: ["/concepts/spans", "/guides/add-progress", "/packages"]
---
<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

This tour builds the parts of a command that resets machines. It has seven
steps. You need Go 1.26 or later, and one command:

```console
$ go get github.com/GSI-HPC/go-clikit/progress
```

Each code block below is an Example function of the kit. CI runs each one
and compares its output with the output that the block shows. So the code
on this page compiles, and does what the page says.

{{< fig caption="The tour adds one package at each step. The arrows show the order." >}}
<svg viewBox="0 0 900 130" role="img" aria-label="Seven steps: report, no cost, many hosts, show, strict flags, result, test.">
<g font-size="13">
<line x1="60" y1="40" x2="840" y2="40" stroke="var(--line)" stroke-width="3"/>
<g fill="var(--run)"><circle cx="60" cy="40" r="17"/><circle cx="190" cy="40" r="17"/><circle cx="320" cy="40" r="17"/><circle cx="450" cy="40" r="17"/><circle cx="580" cy="40" r="17"/><circle cx="710" cy="40" r="17"/><circle cx="840" cy="40" r="17"/></g>
<g fill="var(--ink)" font-weight="700" text-anchor="middle"><text x="60" y="45">1</text><text x="190" y="45">2</text><text x="320" y="45">3</text><text x="450" y="45">4</text><text x="580" y="45">5</text><text x="710" y="45">6</text><text x="840" y="45">7</text></g>
<g fill="var(--fg)" text-anchor="middle" font-weight="600"><text x="60" y="84">Report</text><text x="190" y="84">No cost</text><text x="320" y="84">Many hosts</text><text x="450" y="84">Show</text><text x="580" y="84">Strict flags</text><text x="710" y="84">Result</text><text x="840" y="84">Test</text></g>
<g fill="var(--mute)" text-anchor="middle" font-size="12" class="mono"><text x="60" y="106">progress</text><text x="190" y="106">progress</text><text x="320" y="106">fanout</text><text x="450" y="106">cliprogress</text><text x="580" y="106">cobratree</text><text x="710" y="106">cliout</text><text x="840" y="106">progresstest</text></g>
</g>
</svg>
{{< /fig >}}

## 1. Report the work of a library

A library does not draw. It reports its work as spans. A span is one unit
of work: here, one step that resets the machines, and one target for each
machine. The span goes in the `context.Context`, so the library needs no
new parameter.

{{< example "progress" "reset" >}}

Three things in this code are important:

- The step says that it folds its targets (`FlagFold`) and how many it
  expects (`Total`). A display can then count the work from the start.
- Each target is started queued, with `Queued`, before the first one runs.
  `progresstest.Check` holds every library to this rule.
- `End` takes the error of the work. A nil error ends the span ok. Any
  other error ends it failed or canceled.

A command makes a Bus, puts it in the context, and starts its own span.
Here, the sink of the Bus prints each event:

{{< example "progress" "Example" >}}

Each event has a number, `Seq`. Every sink gets the events in this one
order. Read more in [Spans and the Bus](../concepts/spans/).

## 2. No listener, no cost

A library can report its work when no command listens. Without a Bus in the
context, `Start` gives back the same context and a nil span. The methods of
a nil span do nothing and allocate nothing.

{{< example "progress" "ExampleStart_withoutABus" >}}

## 3. Run the work on many hosts

The loop in step 1 resets one machine at a time. `fanout.Map` runs the work
on many items at once, at most `Limit` at a time. It reports each item as a
target, and keeps the rules of step 1 for you.

{{< example "fanout" "ExampleMap" >}}

`Map` gives the outcomes in the order of the items. The error of the step
names each item that failed. The last lines are the tree of spans that
`progresstest.Capture` kept, with the targets folded into node sets.

## 4. Show the work

The command decides how to show the work. `cliprogress` reads the words of
`--progress` and gives the command a `Session`: the Bus, the display, the
event log and the summary of one command.

{{< example "progress/cliprogress" "ExampleStart" >}}

This example asks for `plain` lines, which a test can compare. On a
terminal, `auto` draws the live tree:

{{< cast display="tree" theme="tide" themes="all" caption="The live tree, recorded from `progress/display` by a test of the kit. Pick a theme." >}}

{{% call note %}}
The kit reads no environment. The program reads `--progress`, the variable
that stands in for it, `NO_COLOR` and `TERM`, and gives what it read to
`cliprogress`. Read more in [Policy stays in the program](../concepts/policy/).
{{% /call %}}

## 5. Make the command line strict

cobra lets some mistakes pass. A word that names no command prints the help
and succeeds, so a script with a typing error goes on to its next step.
`cobratree.Strict` refuses these mistakes, and marks each error as a usage
error.

{{< example "cobratree" "ExampleStrict" >}}

The kit names no exit code. In this example the program gives a usage
error the code 2, which it finds with `IsUsage`.

## 6. Write the result

The result goes to standard output, as a table for a person or as JSON for a
program. `cliout` aligns each column by the columns that a terminal gives
the text, and escapes each cell.

{{< example "cliout" "ExampleTable" >}}

The reason of `exe0001` holds a newline. Without the escape, the terminal
would show a row that is not in the table.

## 7. Test what the command reports

`progresstest.Watch` gives a test a context with a Bus of its own. When the
test ends, `Check` holds the events to the rules of the kit, and `Finish`
gives the tree of spans, in an order that does not change from run to run.

{{< example "progress/progresstest" "ExampleWatch" >}}

## What to read next

You have used seven packages. The [guides](../guides/) each do one task in
full. The [concepts](../concepts/) tell why the kit works as it does.
[pkg.go.dev](https://pkg.go.dev/github.com/GSI-HPC/go-clikit) has every name.
