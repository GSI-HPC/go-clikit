<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Proposal: the work of a span

Status: proposed, not implemented. Nothing here is API yet; once the
maintainer settles the open questions, the decision at the end goes into
[decisions.md](../decisions.md) as decision 21, and this file goes.

A span says what it is and how it ended, and a step says how many of its
targets are done. Nothing says how far one piece of work has got: a copy of
a 2 GiB image, a firmware upload, an index of 50,000 files. This proposal
lets a span report its **work**, an amount in a unit, bounded by a size or
not, and lets the displays draw it as an amount, a rate, a percentage, a bar
and the time left. The work of a span **rolls up** the tree, so that a step
over 480 targets that each copy an image shows how far the whole copy has
got.

## At a glance

A library declares the work and advances it; the rest is the kit's:

```go
ctx, call := progress.Start(ctx, progress.KindCall, "download",
	progress.HTTP("GET", "/images/rocky-9.4.qcow2"),
	progress.Work(progress.Bytes, resp.ContentLength)) // -1 is a size not known
_, err := io.Copy(progress.CountWriter(ctx, dst), resp.Body)
call.End(err)
```

What the live tree draws, on a terminal 120 columns wide, for a pool of 480
targets that each copy an image:

```text
deploy · 13:28.4
  copying the image  312/480 · 1 failed · 8 running · 160 queued · 629 GiB · 65% · 797 MiB/s · ~7m03s left
    ✗ exe0007  transport: dial tcp: i/o timeout
    ▸ exe0313  1.4/2.0 GiB · 70% · 101 MiB/s  14.2s  copy
    ▸ exe0314  0.5/2.0 GiB · 25% · 98.5 MiB/s  5.2s  copy
    … 6 more running
    ✓ exe[0001-0006,0008-0312]
```

And in the Tide theme, whose bar now fills with the share of the work
done, not only with the targets that ended:

```text
deploy ◦ 13:28.4
╎ copying the image  ◉◉◉◉◉◉◌◌◌◌ 312/480 ◦ 1 failed ◦ 8 running ◦ 160 queued ◦ 629 GiB ◦ 65% ◦ 797 MiB/s ◦ ~7m03s left
  ╎ ✕ exe0007  transport: dial tcp: i/o timeout
  ╎ ◡ exe0313  ◉◉◉◉◌◌ 1.4/2.0 GiB ◦ 70% ◦ 101 MiB/s  14.2s  copy
  ╎ ◡ exe0314  ◉◌◌◌◌◌ 0.5/2.0 GiB ◦ 25% ◦ 98.5 MiB/s  5.2s  copy
  ╎ ⋯ 6 more running
  ╎ ✓ exe[0001-0006,0008-0312]
```

## The model

### What a span says

A span may report work: how much is done, its **amount**, in a **unit**,
out of a **size** when one is known.

- A span with a size is **bounded**: a display draws the amount against it,
  a percentage, a bar where the theme draws one, the rate and the time
  left.
- A span without one is **unbounded**: a display draws the amount and the
  rate, and no percentage, bar or time left.
- The unit is one of a closed set, as a `Class` is: `Items`, things such as
  files, rows or packages, and `Bytes`. A unit may be added in a minor
  release. Amounts in different units are never added together.
- The amount may only be set or advanced while the span runs. It may go
  down, as a download that starts over does, and it may pass the size, which
  is then the program's estimate that was wrong: a display draws `2.1/2.0
  GiB · 100%`.
- The size may be given at the start, or late, once known, as an HTTP
  response's `Content-Length` is; it may change; and 0 or less is a size not
  known. The unit, once given, stays.

What a span says is its **own** work. What is below it is not added to it
in the event: the Bus sends what each span said, and a sink rolls it up.

### Rolling up the tree

`progress.Meter` keeps the work of a Bus's spans from its events, as
`Tally` keeps the count of their targets, and rolls it up the tree. Every
display reads it; a program's own sink, an MCP progress notifier later, can
too.

**The amount.** A span's rolled-up amount is its own, plus the rolled-up
amount of every span started under it in the same unit, those that ended
included: bytes copied stay copied, however the copy ended. A span that
reports no work of its own takes the unit of the first span below it that
does. Work in another unit does not add to the amount, though its share
done still counts towards the parent's share (rule 2). A hidden span's work
rolls up as any other's: the call doing the copy is plumbing, its bytes are
the target's.

**The share done**, a fraction from 0 to 1, comes from the first of three
rules that applies:

1. **Its own size.** A span with a size of its own: its rolled-up amount
   over that size. The program knows best: a step told it moves 960 GiB in
   all is measured in bytes, whatever its targets say.
2. **Its targets.** A span that counts targets, a step with `Fold` or a
   batch whose `Total` is known: the targets that ended, however they ended,
   plus the share done of each target running, over its `Total`. So 312
   ended and 8 running at 45% each make (312 + 3.6) / 480, 65%. A target
   counts the same however much work it holds, it needs no size of the
   targets still queued, and the share reaches 100% exactly when `Tally`'s
   count reaches its `Total`, after an interrupt too.
3. **The work below.** A span with neither: its rolled-up amount over the
   sum of the sizes of the spans below that report work in its unit, those
   ended included, as long as every one of them has a size. One without a
   size makes the span unbounded. The sum grows as work below starts, so
   this share can fall back, as a `docker pull` does when it finds another
   layer: a program that knows the whole up front says so with a size, and
   rule 1 applies.

Without any of them, the span is unbounded. A parent never guesses its
bound from the sizes of work that has not started.

**The rate** is how much the rolled-up amount grew over the last five
seconds, per second, or over the time the span has run when that is
shorter. It is read at the time of the frame, so it falls as the work slows
and reaches zero five seconds after the work stops; from then the work is
**stalled**, and a display says so in place of the rate.

**The time left** is an estimate: the share still to do, at the rate the
share grew over the last five seconds. A display draws it only once the
span has run for two seconds and the share is growing, rounded up to the
whole second, and with a `~` in front, so that it is not read as the
countdown a pause draws.

### How often the events go

A copy advances its span at every write, which may be a hundred thousand
times a second. Each `Advance` adds to an atomic counter of the span; the
Bus sends the amount in an event of a new type, `TypeAdvance`, at most once
a span every 100 ms, the newest amount never lost: what a span advanced
since its last event is sent once its next place comes, by a timer as the
lines of `Tee` are, and in any case with its `End`. A display draws at most
ten times a second, so it misses nothing it would draw. At a fan-out of 16
that is at most 160 events a second, the same as the budget for lines.

An `Advance` of a span with work costs an atomic add and a clock read, and
the lock once each 100 ms; without a Bus, on a nil span, it costs nothing,
and `CountWriter` returns the writer it was given.

## The API

### `progress`

```go
// Unit is what the work of a span is counted in. Values may be added in a
// minor release.
type Unit uint8

const (
	// Items counts things, such as files, rows or packages.
	Items Unit = iota + 1
	// Bytes counts bytes, which a display draws in KiB, MiB, GiB and TiB.
	Bytes
)

// String returns the name the event log writes for the unit, such as
// "bytes", or "unit(n)" for a value this package does not define.
func (u Unit) String() string

// In Fields:

	// Amount is how much of the span's own work is done, and Size how much
	// there is, in Unit; a Size of 0 is one not known. Neither counts the
	// work of the spans below, which Meter rolls up.
	Amount, Size int64
	Unit         Unit

// Work says that a span reports work in u, out of size; a size of 0 or
// less is one not known. Start and Update take it: Update to give the size
// once it is known, or to change it. A unit given before stays.
func Work(u Unit, size int64) Option

// TypeAdvance is a span whose Amount changed. The Bus sends one at most
// every 100 ms a span, with the newest Amount.
const TypeAdvance Type = …

// Advance adds n to the amount of a span's work. A span that was given no
// Work counts Items. It does nothing to a span that is not running, and
// costs no lock but once each 100 ms.
func (s *Span) Advance(n int64)

// SetAmount sets the amount of a span's work, for work that says how far
// it has got rather than how much more it did, such as a remote task that
// reports a percentage, or a download that starts over.
func (s *Span) SetAmount(n int64)

// CountWriter returns a writer that writes to w and advances the innermost
// span in ctx by the bytes w took. It returns w itself when ctx carries no
// span.
func CountWriter(ctx context.Context, w io.Writer) io.Writer

// CountReader returns a reader that reads from r and advances the innermost
// span in ctx by the bytes it read. It returns r itself when ctx carries no
// span.
func CountReader(ctx context.Context, r io.Reader) io.Reader
```

```go
// Meter keeps the work of a Bus's spans from its events, rolled up the
// tree, the way a display shows how far the work has got. It is given every
// event in order, as a sink is, and is not safe for concurrent use.
type Meter struct{ … }

// Add counts e in. When e ends a span with work, own or below, Add returns
// what its work came to.
func (m *Meter) Add(e Event) (ended Reading, ok bool)

// Read returns the work of an open span as it stands at now, when it has
// any.
func (m *Meter) Read(span SpanID, now time.Time) (Reading, bool)

// Reading is the work of a span, rolled up. Fields may be added in a minor
// release.
type Reading struct {
	Unit Unit
	// Amount is the span's own amount and that of the spans below in Unit.
	Amount int64
	// Size is the bound of Amount, under BySize and ByBelow; 0 otherwise.
	Size int64
	// Bound says where Fraction comes from; Unbounded has none.
	Bound    Bound
	Fraction float64
	// Rate is how much Amount grew a second over the last five seconds.
	Rate float64
	// Left is the time the rest of the work should take; 0 is not known.
	Left time.Duration
	// Stalled is how long Amount has not grown, once that is five seconds.
	Stalled time.Duration
}

// Bound says which rule gives a span its share done.
type Bound uint8

const (
	Unbounded Bound = iota
	BySize            // rule 1: the span's own Size
	ByTargets         // rule 2: the targets it counts
	ByBelow           // rule 3: the sizes of the work below it
)

// String returns "unbounded", "size", "targets" or "below".
func (b Bound) String() string
```

### `fanout`

`fanout.Item` gains the work of an item's target, so that a pool whose
items have sizes known up front has every target bounded from its start:

```go
type Item struct {
	Node, Host, Role string
	// Unit and Size are the work of the item's target, as progress.Work
	// gives them; a zero Unit is none.
	Unit progress.Unit
	Size int64
}
```

### `progresstest`

- `Capture.Tree` writes a span's work after its fields: `amount=2147483648
  size=2147483648 unit=bytes`.
- `Check` holds every source to new rules, which only events and fields
  that did not exist before can break: a `TypeAdvance` names a span that is
  running and changes its `Amount`; `Amount` and `Size` are never negative;
  and a span's `Unit` never changes once given.

## Using it

### One download, bounded

```go
func fetch(ctx context.Context, url string, dst io.Writer) (err error) {
	ctx, call := progress.Start(ctx, progress.KindCall, "download", progress.HTTP("GET", path(url)))
	defer func() { call.End(err) }()
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	// The size is known once the headers are; -1 leaves it unknown.
	call.Update(progress.Work(progress.Bytes, resp.ContentLength))
	_, err = io.Copy(progress.CountWriter(ctx, dst), resp.Body)
	return err
}
```

The command runs it under a step, and the step's row shows the call's work,
rolled up by rule 3:

```text
fetch · 0:14.4
  fetching the base image  1.1/2.0 GiB · 56% · 80.0 MiB/s · ~12s left  14.4s  GET /images/rocky-9.4.qcow2
```

The line the step leaves once it has ended says what it moved, and how
fast on average:

```text
✓ fetching the base image  25s  2.0 GiB at 80.0 MiB/s
```

The counter, which has no counted step to show here, shows the work of the
newest named step in its place:

```text
fetching the base image · 1.1/2.0 GiB · 56% · 80.0 MiB/s · ~12s left · 0:14.4
```

Plain lines say how far a step with work has got every ten seconds, as they
do for a counted step:

```text
[0:00] fetch › fetching the base image: start
[0:10] fetch › fetching the base image: 0.8/2.0 GiB, 39%, 80.0 MiB/s, ~16s left
[0:25] fetch › fetching the base image: ok in 25s: 2.0 GiB at 80.0 MiB/s
```

### Work of unknown size

A walk counts the files it indexes, and a stream without a
`Content-Length` its bytes. Neither has a size, so they draw an amount and
a rate:

```go
ctx, step := progress.Start(ctx, progress.KindStep, "indexing the archive",
	progress.Work(progress.Items, 0))
err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}
	step.Advance(1)
	return index.Add(path, d)
})
step.End(err)
```

```text
index · 0:07.3
  indexing the archive  48.2k · 6.6k/s  7.3s
  receiving the dump  312 MiB · 42.2 MiB/s  7.4s  GET /export
```

Once the dump has not grown for five seconds, its rate gives way:

```text
  receiving the dump  312 MiB · stalled 6.1s  13.5s  GET /export
```

### Work that says how far it has got

A Redfish firmware update reports a percentage on its task, which the call
sets as it polls:

```go
_, call := progress.Start(ctx, progress.KindCall, "firmware update",
	progress.HTTP("GET", task), progress.Work(progress.Items, 100))
for st := range poll(ctx, task) {
	call.SetAmount(int64(st.PercentComplete))
}
```

```text
    ▸ exe0313  40/100 · 40% · 1.9/s · ~32s left  21.0s  GET /redfish/v1/TaskService/Tasks/7
```

### A pool, rolled up

The pool's step counts its targets; each target's copy advances the call it
runs under, which rolls up into the target, and the targets roll up into
the step by rule 2:

```go
outcomes, err := fanout.Map(ctx, nodes, fanout.MapOptions[node]{
	Step:  "copying the image",
	Limit: 8,
	Describe: func(n node) fanout.Item {
		return fanout.Item{Node: n.Name, Unit: progress.Bytes, Size: image.Size}
	},
}, func(ctx context.Context, n node) (struct{}, error) {
	return struct{}{}, copyImage(ctx, n, image) // writes through progress.CountWriter(ctx, …)
})
```

As it runs, on 120 columns, as at the top:

```text
deploy · 13:28.4
  copying the image  312/480 · 1 failed · 8 running · 160 queued · 629 GiB · 65% · 797 MiB/s · ~7m03s left
    ✗ exe0007  transport: dial tcp: i/o timeout
    ▸ exe0313  1.4/2.0 GiB · 70% · 101 MiB/s  14.2s  copy
    ▸ exe0314  0.5/2.0 GiB · 25% · 98.5 MiB/s  5.2s  copy
    … 6 more running
    ✓ exe[0001-0006,0008-0312]
```

On 80 columns the step's row gives up the time left, the rate and the
amount, in that order, to keep its percentage; a target's row gives up its
request first, as it does now:

```text
deploy · 13:28.4
  copying the image  312/480 · 1 failed · 8 running · 160 queued · 65%
    ✗ exe0007  transport: dial tcp: i/o timeout
    ▸ exe0313  1.4/2.0 GiB · 70% · 101 MiB/s  14.2s  copy
```

The counter, and plain lines with the line the step leaves at its end:

```text
copying the image · 312/480 · 1 failed · 8 running · 160 queued · 629 GiB · 65% · 797 MiB/s · ~7m03s left · 13:28.4
```

```text
[0:00] deploy › copying the image: start, 480 nodes, 8 at a time
[0:03] deploy › copying the image › exe0007 failed (transport): dial tcp: i/o timeout
[13:20] deploy › copying the image: 310/480 done, 1 failed, 8 running, 162 queued, 625 GiB, 65%, 797 MiB/s, ~7m08s left
[20:33] deploy › copying the image: failed in 20m33s: 479 ok, 1 failed, 958 GiB at 796 MiB/s
```

The summary:

```text
deploy: failed in 20m33s: 479 ok, 1 failed, 958 GiB at 796 MiB/s
```

### A step that knows its whole size

A step told the bytes of all its targets is measured in bytes by rule 1,
so a large target weighs more than a small one, and the step's row draws
the amount against the size:

```go
ctx, step := progress.Start(ctx, progress.KindStep, "syncing the home directories",
	progress.WithFlags(progress.Fold), progress.Total(len(users)),
	progress.Work(progress.Bytes, quota.Used()))
```

```text
  syncing the home directories  41/120 · 6 running · 73 queued · 1.4/3.9 TiB · 35% · 1.1 GiB/s · ~38m47s left
```

## How it draws

The text drawn with the zero `Theme` is an interface (decision 5), so it is
fixed here. A span that reports no work, which is every span today, draws
exactly what it draws now: no consumer's frames change.

| What | How it reads |
| --- | --- |
| Bytes | IEC units, one decimal below 100 and none from 100, rounded: `512 B`, `85.3 MiB`, `312 MiB`, `1.2 GiB` |
| Items | Whole numbers below 10,000, then SI prefixes as for bytes: `9312`, `48.2k`, `1.2M` |
| An amount out of a size | Both in the size's unit: `0.5/2.0 GiB`, `40/100`, `48.2k/50.0k` |
| A rate | The amount's form with `/s`: `85.3 MiB/s`, `6.6k/s`, `0.4/s` |
| The share done | A whole percentage, cut, not rounded, so that `100%` means done: `65%` |
| The time left | `~` and the time rounded up to the second: `~12s left`, `~7m03s left`, `~1h02m left` |
| Stalled | `stalled` and how long, as a live clock: `stalled 6.1s` |
| On a row of the tree or the counter | After the counts of a step, or after the name of a target, before how long it has run, split by the separator: `1.1/2.0 GiB · 56% · 80.0 MiB/s · ~12s left` |
| In a plain line or the summary | The same parts split by commas; at the end, the amount and the average rate: `2.0 GiB at 80.0 MiB/s` |
| On a row that does not fit | The request goes first, as now; then the time left, the rate, the amount and the bar |

In a theme:

- the bar of a counted step fills with its share done, so that it moves
  while its targets run, not only as they end; the cells of the targets
  that failed stay at its end, in the colour of a failure, as now;
- a running target, call or step with a bounded share draws a bar of six
  cells before its amount, which goes before the rest of the work when the
  row is short;
- the amount and rate take the muted colour, the percentage the colour of
  the bar, and `stalled` the colour of a cancellation. The words stay those
  of no theme (decision 20).

With `ASCII` the bar is `[####..]`, as now.

## The event log

Under version 1, keys and values may be added, so this is no new version.
The log gains three keys and one value of `type`:

| Key | Holds |
| --- | --- |
| `type` | `advance`, a span whose `amount` changed |
| `amount`, `size` | How much of the span's own work is done, and how much there is, in its unit; never what is below it |
| `unit` | `items` or `bytes` |

The amount is on every event of the span from the first that sets it, as
every field is, so that the `end` of a span says what its work came to.
A reader that sums the amounts below a span gets the rolled-up amount; the
log never carries what a display derives.

```json
{"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":3,"time":"2026-10-10T12:00:00.000000000Z","type":"start","span":"b7e3a1c09d2f4e12","parent":"b7e3a1c09d2f4e11","kind":"call","name":"download","state":"running","method":"GET","path":"/images/rocky-9.4.qcow2"}
{"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":4,"time":"2026-10-10T12:00:00.020000000Z","type":"update","span":"b7e3a1c09d2f4e12","parent":"b7e3a1c09d2f4e11","kind":"call","name":"download","state":"running","method":"GET","path":"/images/rocky-9.4.qcow2","size":2147483648,"unit":"bytes"}
{"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":5,"time":"2026-10-10T12:00:00.100000000Z","type":"advance","span":"b7e3a1c09d2f4e12","parent":"b7e3a1c09d2f4e11","kind":"call","name":"download","state":"running","method":"GET","path":"/images/rocky-9.4.qcow2","amount":8388608,"size":2147483648,"unit":"bytes"}
{"v":1,"run":"0123456789abcdef","trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":261,"time":"2026-10-10T12:00:25.600000000Z","type":"end","span":"b7e3a1c09d2f4e12","parent":"b7e3a1c09d2f4e11","kind":"call","name":"download","state":"ended","method":"GET","path":"/images/rocky-9.4.qcow2","amount":2147483648,"size":2147483648,"unit":"bytes","status":"ok"}
```

The golden log grows by a third command, `fetch`, after `status`, with an
`advance` and each unit; its first lines do not change.

## Examples, as they would be committed

These are the `Example` functions the change would add, with the output the
tests would hold them to. `newExampleClock` and `startTargets` are those of
`progress/display/example_test.go`.

```go
// A span reports its work as it goes: Work gives its unit and size, and
// Advance adds to the amount done. The Tree draws the work on the row of
// the step the call is made for, which it rolls up into.
func ExampleNewTree_work() {
	clock := newExampleClock()
	screen := &progresstest.Screen{Width: 120}
	term := display.NewTerminal(screen, display.TerminalOptions{Size: func() (int, int, error) { return 120, 24, nil }})
	tree := display.NewTree(term, display.TreeOptions{Now: clock.Now})
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{tree}, Now: clock.Now})
	ctx, command := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "fetch")

	ctx, step := progress.Start(ctx, progress.KindStep, "fetching the base image")
	_, call := progress.Start(ctx, progress.KindCall, "download",
		progress.HTTP("GET", "/images/rocky-9.4.qcow2"), progress.Work(progress.Bytes, 2<<30))
	for range 144 {
		clock.Add(100 * time.Millisecond)
		call.Advance(8 << 20)
	}
	tree.Draw()
	fmt.Print(screen.String())
	fmt.Println("---")

	for range 112 {
		clock.Add(100 * time.Millisecond)
		call.Advance(8 << 20)
	}
	call.End(nil)
	step.End(nil)
	command.End(nil)
	bus.Close()
	tree.Close()
	fmt.Print(screen.String())
	// Output:
	// fetch · 0:14.4
	//   fetching the base image  1.1/2.0 GiB · 56% · 80.0 MiB/s · ~12s left  14.4s  GET /images/rocky-9.4.qcow2
	// ---
	// ✓ fetching the base image  25s  2.0 GiB at 80.0 MiB/s
}

// A Meter rolls the work of a Bus's spans up the tree. A step that counts
// its targets is as far as the targets that ended, and the share done of
// each one running: here two of four ended and one is half done.
func ExampleMeter() {
	clock := newExampleClock()
	capture := &progresstest.Capture{}
	bus := progress.NewBus(progress.BusOptions{Sinks: []progress.Sink{capture}, Now: clock.Now})
	ctx, _ := progress.Start(progress.WithBus(context.Background(), bus), progress.KindCommand, "deploy")

	ctx, _ = progress.Start(ctx, progress.KindStep, "copying the image",
		progress.WithFlags(progress.Fold), progress.Total(4))
	targets := make([]*progress.Span, 4)
	for i, node := range []string{"exe01", "exe02", "exe03", "exe04"} {
		_, targets[i] = progress.Start(ctx, progress.KindTarget, node, progress.Queued(),
			progress.Node(node), progress.Work(progress.Bytes, 1<<30))
	}
	for _, target := range targets[:2] {
		target.Run()
		target.Advance(1 << 30)
		target.End(nil)
	}
	targets[2].Run()
	clock.Add(5 * time.Second)
	targets[2].Advance(1 << 29)

	var meter progress.Meter
	var counted progress.SpanID
	for _, e := range capture.Events() {
		meter.Add(e)
		if e.Kind == progress.KindStep && e.Type == progress.TypeStart {
			counted = e.Span
		}
	}
	r, _ := meter.Read(counted, clock.Now())
	fmt.Println(r.Bound, r.Fraction, r.Amount, r.Size)
	bus.Close()
	// Output:
	// targets 0.625 2684354560 0
}
```

`ExampleCountWriter` in `progress` shows a copy through `CountWriter` in a
`Capture`'s tree, and `ExampleMapOptions_work` in `fanout` a pool whose
`Item`s carry their sizes.

## Alternatives considered

- **Sinks read the amounts from the spans when they draw.** It costs the
  least, but the events would no longer say everything that happened, in
  one order: the log could not record the work, a test could not replay
  it, and a sink would hold a span, which is not plain data.
- **An event for every `Advance`.** A copy would send tens of thousands a
  second to every sink, under the Bus's lock, and into the log.
- **The amount in a `TypeUpdate`.** An update says that `Total` or
  `Message` changed, which a display acts on; a sink that does not draw
  work would be woken ten times a second a span for nothing it uses, and a
  reader of the log could not tell the two apart without comparing fields.
- **The Bus rolls up, and the events carry the totals.** Every `Advance`
  would send an event for each span above it, and the log would carry what
  can be derived from it. The roll-up is a sink's, as the count of targets
  is `Tally`'s.
- **A parent bounded by an estimate of the sizes of its targets still
  queued**, from the mean of those that started. It is a guess drawn as a
  measure; rule 2 is exact in its own terms, a share of the targets, and
  needs no size of a target that has not started.
- **A unit of free text**, such as "files". The vocabulary is closed: a
  unit is a value of the log, and a program's noun would need a plural too.
  The step's name says what is counted.

## Open questions

1. Rule 3 lets a share fall back as work below starts, which is honest but
   jumps. Should it apply only to targets, whose work is usually one call,
   and leave other spans without a size unbounded?
2. Should there be a `Percent` unit, so that work that reports a percentage
   draws `40%` rather than `40/100 · 40%`, and never adds to an amount
   above it?
3. Should a counted step whose targets report no work of a known size draw
   a percentage at all? Rule 2 then gives the share of its targets that
   ended, which `312/480` already says.
4. Is 100 ms between the advances of one span right for the log, which a
   pool of 16 copies would fill with 160 lines a second?
5. Bytes in IEC units, `MiB`, everywhere, or decimal units, `MB`, as a
   display option?

## Plan

One logical change per commit, each with its tests at 100% coverage:

1. `feat(progress)`: `Unit`, the work in `Fields`, `Work`, `Advance`,
   `SetAmount` and `TypeAdvance`, sampled every 100 ms; the event log's
   keys, `event-log.md` and the golden log's third command. A test holds
   `Advance` on a nil span to no allocation, and a benchmark its cost with
   a Bus.
2. `feat(progress)`: `CountWriter` and `CountReader`.
3. `feat(progress)`: `Meter` and `Reading`, with a fuzz target that holds
   every rolled-up amount to the sum of the amounts below it, and every
   share to 0 to 1.
4. `feat(progresstest)`: the work in `Capture.Tree`, and the rules of
   `Check`.
5. `feat(display)`: the work on the rows of the tree and the counter, in
   plain lines and the summary, and the bars in the themes; the contract
   tests for widths, and a cost test of 10,000 targets that advance.
6. `feat(fanout)`: `Item.Unit` and `Item.Size`.
7. `docs`: decision 21 below, and `architecture.md`.

## The decision, as it would be recorded

### 21. A span reports its work, sampled, and a sink rolls it up

#### Context

The displays said how many targets of a step were done, and nothing of how
far one piece of work had got: a copy, an upload or a walk of many files
looked the same at its first second and its last. A pool of targets that
each copy something showed its share done only as targets ended.

#### Decision

- A span reports its work: an amount, in a unit of a closed set, `Items`
  or `Bytes`, out of a size when one is known. `Work` gives the unit and
  size; `Advance`, `SetAmount`, `CountWriter` and `CountReader` move the
  amount.
- The Bus sends the amount as a `TypeAdvance` at most every 100 ms a span,
  the newest never lost, and with the span's `End`.
- An event carries only what its span said. `progress.Meter` rolls the
  work up the tree: amounts add within a unit; a span's share done comes
  from its own size, else from the targets it counts, else from the sizes
  of the work below, and it is unbounded without any of them.
- The displays draw a span's work after its counts, or after its name;
  what they draw for a span without work does not change.
- The log gains `advance`, `amount`, `size` and `unit` under version 1.
- `Check`'s new rules apply only to the new event and fields, which no
  source before this one sends, and so are no breaking change in the sense
  of decision 15.

#### Costs

- A span with work keeps an atomic counter and a timer; an `Advance`
  reads the clock.
- A pool of copies adds up to ten log lines a second a target running.
- Rule 3 lets a share fall back when work below starts that was not known.
- The time left is an estimate, and reads wrong while the rate changes.
