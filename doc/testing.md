<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Testing

`make test` runs every test under the race detector, `make floor` vets and
tests with the Go release `go.mod` names, `make cover` holds every file to
100% of its statements, and `make fuzz` runs the fuzz targets for a minute
each. CI runs all of them, the tests on Linux and on macOS as well, since the
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

`Checked` gives a test a Bus whose events are checked when the test ends,
and `Watch` one whose events are checked, and drawn as a tree, when the test
asks. Both check before the Bus is closed, which would end a span left open
and hide it. `Capture.Tree` draws the spans as an indented tree that does not
depend on how concurrent work was scheduled: targets that read the same are
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
fake clock passes at once. The pause between two batches is `After`, which a
test replaces.

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
see, frame by frame.

The tests of each display cover a wide fan-out, failures that group, a hidden
span that turns slow, a step that fails at once, a power-on in batches, two
steps side by side, a terminal too small for the tree, the ASCII marks, an
interrupt, a question and a write in the middle of a frame. The contract
tests of `progress/display` put them together, as a program sees them: a
pool, the command's output and questions, and a Screen, on which the counter
and the tree are taken off before every write and never drawn over a
question, and leave the command's output and their summary behind. The
Terminal's `Lines` is tested around a question, an open line, a frame and the
display's end, and against its bound; a stress test has four goroutines write
200 lines each through it while the command writes its own lines, asks
questions and the counter draws, and checks that every line arrived whole, on
a row of its own, in its writer's order, and none inside a question.

## The pools

A fake that answers at once rarely has two calls under way together, so a
pool's bound is tested on the fake clock of `testing/synctest`: every call
waits a second, which passes only once every call that can start has, and the
test counts the most that were under way at once. The same clock lets the
pause between batches pass at once. Each pool is also checked with
`progresstest`, so that what it reports, after a failure, a panic and an
interrupt too, keeps the contract above.

## Fuzzing

Three fuzz targets check what must hold for any text a remote host sends:

- `termtext.FuzzEscape`: neither escaper leaves a rune its policy names, and
  escaping twice changes nothing;
- `termtext.FuzzTruncate`: a cut row is a prefix of the text, cut on a rune,
  that fits its columns and is the longest that does, text that fits
  already is left as it is, and `Width` keeps to the bounds the widths of
  the runes set;
- `progress.FuzzSanitize`: what comes out is UTF-8, holds nothing a terminal
  would act on, keeps to its bound and is the same when sanitised again.

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

## The event log

The log is compared line for line with `progress/testdata/log-v1.jsonl`, with
the span ids replaced by their order and a clock the test moves;
[event-log.md](event-log.md) says what the file pins. The file grows only
at its end, by a later command on the same Bus, and a test holds its first
lines, those version 1 was first released with, to a checksum. A test sets
every part of an event and fails when one is neither logged nor left out on
purpose, so a part added to `Event` or `Fields` is not logged, or kept out,
without a decision. Other tests cover a run that appends to a file another run
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
nobody can show works.
