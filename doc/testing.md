<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Testing

`make test` runs every test under the race detector, `make floor` vets and
tests with the Go release `go.mod` names, `make cover` holds every file to
100% of its statements, `make fuzz` runs the fuzz targets for a minute
each, and `make costs` measures what the live tree costs on a large step.
CI runs all of them, the tests on Linux and on macOS as well, since the
displays draw on terminals and the programs that use them run on both.

Tests use the standard library alone, table-driven, with `t.Errorf` and
`t.Fatalf`. A test's name says the property it checks, not the function it
calls, and a comment above it says why the property matters.

## The contract of the events

`progress/progresstest` is how the kit tests itself, and how a program tests
what it reports. `Capture` is a sink that keeps the events of a Bus, and
`Check` holds them to every promise the progress package makes to a display:

- `Seq` runs from 1 with no gaps, and every span starts once and ends once,
  under a parent that has started and not ended;
- the targets of a Fold step or a batch, and the batches of a step, all
  start queued before the first of them runs, and add up to the step's
  Total however it ended, an interrupt included;
- no more targets run at once below a span than its Limit;
- every Suspend is followed by one Resume;
- and no text holds anything `termtext.Escape` would escape, or is
  longer than its bound.

Each rule is a promise the progress package makes to every sink, and
programs hold their own tests to it, so the rules are an interface beyond
the Go API (decision 15). A new or stricter rule is a breaking change,
named in the release notes; a rule that catches what an existing promise
already forbade is a fix. No option selects which rules run.

`Watch` gives a test a Bus of its own and a `Watcher`, whose `Finish`
checks the events, closes the Bus and draws them as a tree. It checks
before the Bus is closed, which would end a span left open and hide it,
and a test that never calls `Finish` is checked the same way when it ends.
The Bus asks for lines of output, as a live display does, so they are
checked too; `Events` and `Tree` read the events so far and check nothing.
A test that sets up its own Bus with a `Capture` calls `Check` before it
closes the Bus. `Watch`'s options are opaque: `Classify` gives the Bus the
program's rule for the class of an error, and `Sinks` puts more sinks on
it, such as a display drawing on a `Screen`, ahead of the `Capture`, which
no option can take off. `Capture.Tree` draws the spans as an indented tree
that does not depend on how concurrent work was scheduled: targets that
read the same are
folded into one line naming them as a node set, or listing them when one is
no node or a node is named twice, alone or in another name's node set, as
for a target that ran twice under one name or for exe1 next to exe[1-2],
and siblings are sorted, so tests compare trees whole. `Check`'s own
reporting is tested with a fake `testing.TB`, and a test that walks
`progress.Event` and `progress.Fields` fails when a text field is added
that `Check` does not check or `Tree` does not draw.

## Clocks

Nothing in the tests waits for real time to pass. The Bus, the displays and
the plain lines each take a `Now`; the tests give them a clock they move by
hand, and call `Draw` for a frame where a display would draw from its ticker.
So a frame shows the same counts and times on every run, and no test waits
for a display's first second, a heartbeat's ten or a tick. The few tests that
start a display's own goroutine run in a `testing/synctest` bubble, whose
fake clock passes at once, and so do those of the event log's second and of
the five seconds its `Close` waits, which are not options. The pause between
two batches is a timer, so the tests of `Batches` run in a bubble too; a
display's test draws its frame during the pause from a goroutine that
`synctest.Wait` lets go once `Batches` waits for the timer.

## The terminal

`progresstest.Screen` is a terminal for tests: it applies text, newlines,
carriage returns and the sequences the displays write, moving up rows,
erasing a row, the rest of it or the rest of the screen, and shows every
other control character as text, so that a test sees it, as it does a
sequence cut short and an escape, a sequence or a rune that the output ends
in the middle of. It keeps what has scrolled off the top, and with `Width`
set wraps a row the way a terminal does, so a row drawn too wide shows as
the two it would be: a row written to its last column leaves the cursor on
that column until the next rune, where an erase to the end of the row takes
the last rune. A wide rune takes two columns, and writing over half of one
blanks the other, as a terminal does. A test compares what a person would
see, frame by frame. With `Styles` set, a Screen applies the colours a theme
draws, which take no column, and `Styled` shows each run of text in the
attributes it was written in, `«1;31»✗«»`, so that a test sees which part
is in which colour; any other attribute, a background or italic, is still
shown as text. `Styled` shows the attributes of what is on the screen, not
the sequences that set them, so a row that leaves its colour on reads as
one that sets it back: a colour left on shows only in the text written
after it. The tests that check that none is left on write text after the
output, or a mark after every row and every write, and look at its
colour.

The tests of each display cover a wide fan-out, failures that group, a hidden
span that turns slow, a step that fails at once, a power-on in batches, two
steps side by side, a terminal too small for the tree, the ASCII marks, an
interrupt, a question and a write in the middle of a frame. Every display is
also drawn in themes, in each number of colours and in ASCII, on a Screen
with `Styles`: a themed row never wraps at any width from 40 columns to 120,
no row or line ends in a colour, and plain lines and the summary say the
same words with the colours and their mark taken out. With the zero `Theme`
every display writes what it wrote before, byte for byte, which the tests
written before themes hold it to. Every glyph of every theme is held to one
column, to an East Asian Width that is not ambiguous, and to no emoji, nor
any rune Unicode keeps for emoji to come. The palettes are held to decision
19: those of 256 colours to a middle lightness, an L* of 45 to 60, with the
failed end of a bar a ΔE of 20 or more from the rest of it, as readers with
deuteranopia or protanopia see it too; those of 16 to the terminal's own
colours, never bright and never bold with a colour. The contract tests of
`progress/display` put them together, as a program sees them: a
pool, the command's output and questions, and a Screen, on which the counter
and the tree are taken off before every write and never drawn over a
question, and leave the command's output and their summary behind. The
Terminal's `Lines` is tested around a question, an open line, a frame and the
display's end, and against its bound; a stress test has four goroutines write
200 lines each through it while the command writes its own lines, asks
questions and the counter draws, and checks that every line arrived whole, on
a row of its own, in its writer's order, and none inside a question.

## The command line

`progress/cliprogress` is tested from outside the package, as a program uses
it. `Choose` has a table of what the flag and the variable ask for on a
terminal, on a dumb one and in a pipe, with every error and note it returns.
`Start` runs with `Options.Manual` on a `progresstest.Screen` that the
Options call a terminal of 80 columns, on a clock the test moves, and
`Run.Draw` draws each frame; one test starts the tree's own goroutine in a
`testing/synctest` bubble, and sees it say the command was interrupted.

The private file of the event log is tested inside the package, through
`appendPrivate(path, me)`, whose `me` is the user the file has to be. A test
run as another user than root passes its own uid plus one to stand for
someone else; one run as root gives the file, the link or the pipe to nobody,
uid 65534, with `Chown`, or `Lchown` for a link. So every refusal runs on a
CI runner, as root or not, and a test can run in parallel with the others.
A link or a named pipe that another user would put at the log's name after
the walk of its path is put there between `trustedLinks` and `openPrivate`,
the walk and the open `appendPrivate` makes.

A pseudo-terminal opened through `/dev/ptmx`, with the standard library's
`syscall`, holds `TerminalSize` and `InForeground` to a real terminal on
Linux. A terminal that is not the process's controlling terminal cannot say
who its foreground is, and is taken to be the process's.

## The pools

A fake that answers at once rarely has two calls under way together, so a
pool's bound is tested on the fake clock of `testing/synctest`: every call
waits a second, which passes only once every call that can start has, and the
test counts the most that were under way at once. The same clock lets the
pause between batches pass at once. Each pool is also checked with
`progresstest`, so that what it reports, after a failure, a panic and an
interrupt too, keeps the contract above.

## Fuzzing

Five fuzz targets check what must hold for any text a remote host sends,
for the rows a display draws in colour, and for the work a span reports:

- `termtext.FuzzEscape`: neither escaper leaves a rune its policy names, and
  escaping twice changes nothing;
- `termtext.FuzzTruncate`: a cut row is a prefix of the text, cut on a rune,
  that fits its columns and is the longest that does, text that fits
  already is left as it is, and `Width` keeps to the bounds the widths of
  the runes set;
- `progress.FuzzSanitize`: what comes out is UTF-8, holds nothing a terminal
  would act on, keeps to its bound and is the same when sanitised again;
- `progress.FuzzMeter`: whatever events of spans that start, run, report
  work and end it is given, in whatever order, a `Meter` reads a span's
  amount as its own and that of every span below it in its unit, those
  that ended included, gives a span with work of its own a reading in its
  unit, and holds every share to 0 to 1;
- `display.FuzzCut`: a row cut to its columns shows what `termtext.Truncate`
  keeps of its text, with every colour sequence whole and none after the
  cut, and a row holding an escape that was shortened ends by setting the
  colours back.

`make fuzz` runs each for `FUZZTIME`, 60 s unless told otherwise, and
`FUZZ=progress:FuzzSanitize` runs one. CI runs each for a minute on every
change and uploads the failing input when one fails; it is committed under
the package's `testdata/fuzz/`, where every later `go test` replays it.

## The path that allocates nothing

A library reports its spans whether a command listens or not, so without a
Bus that has to cost nothing. Two tests hold it to that with
`testing.AllocsPerRun`: a target's lifecycle, queued, run, one call and both
ended, and the rest of what a library calls, updating and skipping a span,
suspending the displays and passing output through `Tee`. Each asks for no
allocation at all. `BenchmarkTargetLifecycle` measures the lifecycle with a
Bus and without one.

## The cost of a large step

The tree keeps every event under its lock while the Bus waits, and every
worker that reports an event waits for the Bus, so what the tree spends on
an event or a frame slows the work itself. Programs draw it over steps of
tens of thousands of targets. `TestTheCostOfALargeStep`, in
`progress/display`'s `scale_test.go`, holds its cost to the number of
targets rather than to a number of milliseconds, which would depend on the
machine: each of its parts runs the same work at two sizes, takes the best
of five runs of each, and fails when the larger costs several times more
than the sizes alone explain. Work quadratic in the targets misses that by
an order of magnitude at thirty times as many. Each part logs how many
times as long the larger size took, the two times and its limit, so that
the log of a run with `-v` shows how much room each has.

The race detector slows some code more than other code, so the test skips
under it, and `make test` and CI's test jobs, which run every test under
the race detector, leave it out. `make costs` runs it without the race
detector, and so does CI, in a job of its own, Costs, alone on its runner,
so that no other package's tests compete for the CPU while it measures
([decision 17](decisions.md#17-the-cost-tests-run-in-ci-alone)).
`make floor` runs it too.

## The event log

The log is compared line for line with `progress/testdata/log-v1.jsonl`, with
the span ids replaced by their order and a clock the test moves;
[event-log.md](event-log.md) says what the file pins. The file grows only
at its end, by a later command on the same Bus, and a test holds its first
lines, those version 1 was first released with, to a checksum. A test sets
every part of an event and fails when one is neither logged nor left out on
purpose, so a part added to `Event` or `Fields` is not logged, or kept out,
without a decision; another fails when a member of `Fields` takes the name
of one of `Event`, which would shadow it, or two parts share a key of the
log. Other tests cover a run that appends to a file another run
appends to, a writer that fails, one that falls behind, and `Close`.

## Examples

Every package has `Example` functions for its main entry points, which
pkg.go.dev shows next to them. Those with an `// Output:` comment run with the
tests and fail when their output changes, so the documentation cannot drift
from what the code does; the one that shows a test, `progresstest`'s `Watch`,
is compiled but not run.

## Coverage

Every file stays at 100% of its statements, which `.testcoverage.yml` holds
it to in CI. A branch no test can reach is deleted, not excluded: it is code
nobody can show works. The files built for systems that are not Unix,
`*_other.go`, are not in the profile, which is Linux's: `make vet-other`
compiles them, and nothing runs them.
