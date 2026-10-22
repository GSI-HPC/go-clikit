// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/eventlog"
)

// draw draws a run's spans, one a line, indented by depth, with what each
// says that a view shows.
func draw(r eventlog.Run) string {
	var b strings.Builder
	fmt.Fprintf(&b, "run %s %s %s last %s", r.ID, r.Program, r.Version, r.Last.Format("15:04:05.0"))
	if r.Suspended {
		b.WriteString(" suspended")
	}
	b.WriteString("\n")
	var span func(s eventlog.Span, depth int)
	span = func(s eventlog.Span, depth int) {
		fmt.Fprintf(&b, "%s%s %q %s", strings.Repeat("  ", depth), s.Kind, s.Name, s.State)
		for _, f := range []struct {
			key   string
			value any
			set   bool
		}{
			{"flags", s.Flags, s.Flags != 0},
			{"node", s.Node, s.Node != ""},
			{"total", s.Total, s.Total != 0},
			{"limit", s.Limit, s.Limit != 0},
			{"batch", s.Batch, s.Batch != ""},
			{"message", s.Message, s.Message != ""},
			{"work", fmt.Sprintf("%d/%d %s", s.Amount, s.Size, s.Unit), s.Unit != 0},
			{"status", s.Status, s.Status != 0},
			{"class", s.Class, s.Class != progress.ClassNone},
			{"err", s.Err, s.Err != ""},
			{"lines", fmt.Sprintf("%d-%d", s.Lines, s.Dropped), s.Lines+s.Dropped != 0},
			{"queued", s.Queued.Format("05.0"), true},
			{"started", s.Started.Format("05.0"), !s.Started.IsZero()},
			{"ended", s.Ended.Format("05.0"), !s.Ended.IsZero()},
		} {
			if f.set {
				fmt.Fprintf(&b, " %s=%v", f.key, f.value)
			}
		}
		if s.Exit != nil {
			fmt.Fprintf(&b, " exit=%d", *s.Exit)
		}
		b.WriteString("\n")
		for _, c := range s.Children {
			span(c, depth+1)
		}
	}
	span(r.Root, 0)
	return b.String()
}

// reduce folds lines into a Reducer with o.
func reduce(o eventlog.Options, lines ...string) *eventlog.Reducer {
	r := eventlog.NewReducer(o)
	for _, l := range lines {
		r.Line([]byte(l))
	}
	return r
}

// ev returns a line of an event of the run, at second seq, with the span
// and its parent as small numbers, and the keys of rest.
func ev(run string, seq int, typ string, span, parent int, rest string) string {
	s := fmt.Sprintf(`{"v":1,"run":%q,"trace":"4bf92f3577b34da6a3ce929d0e0e4736","seq":%d,"time":"2026-09-25T12:00:%02d.000000000Z","type":%q`,
		run, seq, seq, typ)
	if span != 0 {
		s += fmt.Sprintf(`,"span":"%016x"`, span)
	}
	if parent != 0 {
		s += fmt.Sprintf(`,"parent":"%016x"`, parent)
	}
	if rest != "" {
		s += "," + rest
	}
	return s + "}"
}

// command returns the lines of a run of one command that ends as status
// says, or has not ended for "".
func command(run, name, status string) []string {
	lines := []string{
		`{"v":1,"type":"trace","run":"` + run + `","trace":"4bf92f3577b34da6a3ce929d0e0e4736","program":"prog"}`,
		ev(run, 1, "start", 1, 0, `"kind":"command","name":"`+name+`","state":"running"`),
	}
	if status != "" {
		lines = append(lines, ev(run, 2, "end", 1, 0, `"kind":"command","name":"`+name+`","state":"ended","status":"`+status+`"`))
	}
	return lines
}

// names returns the names of the runs' commands, the newest first.
func names(r *eventlog.Reducer) []string {
	var out []string
	for _, ru := range r.Runs() {
		out = append(out, ru.Root.Name)
	}
	return out
}

// The golden log folds into the tree of its command, every span with all
// its events said of it. The commands the Bus ran after the first have no
// parent the log holds, and are the first's.
func TestReducerGoldenLog(t *testing.T) {
	r := reduce(eventlog.Options{}, golden(t)...)
	runs := r.Runs()
	if len(runs) != 1 {
		t.Fatalf("%d runs, want 1", len(runs))
	}
	ru := runs[0]
	if ru.Trace.Parent.String() != "00f067aa0ba902b7" || ru.Trace.State != "rojo=00f067aa0ba902b7" || !ru.Done() {
		t.Errorf("the run = %+v", ru)
	}
	got := draw(ru)
	want := goldenTree
	if got != want {
		t.Errorf("the golden log folds into\n%s\nwant\n%s", got, want)
	}
}

const goldenTree = `run 0123456789abcdef  v1.2.3 last 12:00:03.5
command "exec" ended flags=dry-run status=ok queued=00.0 started=00.0 ended=02.0
  call "credential bmc" ended flags=hidden status=ok queued=00.0 started=00.0 ended=00.0
  step "run" ended flags=fold,show-lines total=2 limit=1 message=1 of 2 failed status=failed class=target err=1 of 2 hosts failed: exe0002 queued=00.0 started=00.0 ended=02.0
    target "exe0001" ended flags=show-lines node=exe0001 status=ok queued=00.0 started=01.0 ended=02.0
      call "ssh" ended flags=show-lines status=ok lines=2-0 queued=01.0 started=01.0 ended=02.0 exit=0
    target "exe0002" ended flags=show-lines node=exe0002 status=failed class=transport err=exe0002.mgmt: no answer queued=00.0 started=02.0 ended=02.0
      call "redfish" ended flags=show-lines status=failed class=transport err=exe0002.mgmt: no answer queued=02.0 started=02.0 ended=02.0
  step "power on" ended flags=fold total=6 limit=5 status=failed class=target err=5 of 6 hosts failed queued=02.0 started=02.0 ended=02.0
    batch "batch 1/2" ended node=exe[0003-0007] total=5 limit=5 batch=1/2 status=failed class=target err=5 of 5 hosts failed queued=02.0 started=02.0 ended=02.0
      target "exe0003" ended node=exe0003 status=failed class=timeout err=exe0003: timeout queued=02.0 started=02.0 ended=02.0
      target "exe0004" ended node=exe0004 status=failed class=auth err=exe0004: auth queued=02.0 started=02.0 ended=02.0
      target "exe0005" ended node=exe0005 status=failed class=pin err=exe0005: pin queued=02.0 started=02.0 ended=02.0
      target "exe0006" ended node=exe0006 status=failed class=usage err=exe0006: usage queued=02.0 started=02.0 ended=02.0
      target "exe0007" ended node=exe0007 status=canceled class=canceled err=exe0007: canceled queued=02.0 started=02.0 ended=02.0
    batch "batch 2/2" ended node=exe0008 total=1 limit=5 batch=2/2 status=skipped err=not tried: an earlier batch failed queued=02.0 ended=02.0
    wait "stagger" ended status=canceled class=canceled err=context canceled queued=02.0 started=02.0 ended=02.0
  wait "confirm" ended message=reset 2 hosts status=skipped err=dry run: nothing was done queued=02.0 started=02.0 ended=02.0
  command "status" ended status=ok queued=02.0 started=02.0 ended=02.0
    call "inventory" ended flags=hidden status=ok queued=02.0 started=02.0 ended=02.0
    call "inventory" ended flags=hidden status=ok queued=02.0 started=02.0 ended=02.0
    call "inventory" ended flags=hidden status=ok queued=02.0 started=02.0 ended=02.0
    call "inventory" ended flags=hidden status=ok queued=02.0 started=02.0 ended=02.0
    call "sol" ended flags=show-lines status=ok lines=21-1 queued=02.0 started=02.0 ended=02.0
  command "fetch" ended status=ok queued=02.0 started=02.0 ended=03.5
    step "fetching the base image" ended status=ok queued=02.0 started=02.0 ended=03.2
      call "download" ended work=109051904/2147483648 bytes status=ok queued=02.0 started=02.0 ended=03.2
    step "indexing the archive" ended work=1200/0 items status=ok queued=03.2 started=03.2 ended=03.3
    call "firmware update" ended work=100/100 percent status=ok queued=03.3 started=03.3 ended=03.5
`

// The runs of a log, mixed, are told apart, and listed the newest first;
// a run without a command is not listed.
func TestReducerRuns(t *testing.T) {
	lines := []string{
		command("aaaa", "one", "")[0],
		command("bbbb", "two", "")[0],
		command("aaaa", "one", "")[1],
		`{"v":1,"type":"trace","run":"cccc"}`,
		command("bbbb", "two", "")[1],
		ev("dddd", 1, "start", 7, 0, `"kind":"step","name":"lone","state":"running"`),
		"not a line",
	}
	r := reduce(eventlog.Options{}, lines...)
	if got := names(r); !slices.Equal(got, []string{"two", "one"}) {
		t.Errorf("the runs = %q", got)
	}
	if got := r.Changed(); !slices.Equal(got, []string{"aaaa", "bbbb", "cccc", "dddd"}) {
		t.Errorf("Changed() = %q", got)
	}
	if got := r.Changed(); got != nil {
		t.Errorf("Changed() again = %q", got)
	}
	// A run whose first line is lost takes its trace from its events.
	ru := reduce(eventlog.Options{}, ev("eeee", 1, "start", 1, 0, `"kind":"command","name":"x","state":"running"`)).Runs()[0]
	if ru.Trace.Trace.String() != "4bf92f3577b34da6a3ce929d0e0e4736" || ru.Program != "" {
		t.Errorf("the run without its first line = %+v", ru)
	}
}

// Beyond Keep, the oldest run that has ended goes, else the oldest, and a
// late line of a run that went does not bring it back.
func TestReducerKeep(t *testing.T) {
	var lines []string
	lines = append(lines, command("aaaa", "a", "")...)
	lines = append(lines, command("bbbb", "b", "ok")...)
	lines = append(lines, command("cccc", "c", "")...)
	r := reduce(eventlog.Options{Keep: 2}, lines...)
	if got := names(r); !slices.Equal(got, []string{"c", "a"}) {
		t.Errorf("the runs = %q, want the ended one gone", got)
	}
	r.Changed()
	r.Line([]byte(command("dddd", "d", "")[0]))
	if got := r.Changed(); !slices.Equal(got, []string{"aaaa", "dddd"}) {
		t.Errorf("Changed() = %q, want the one that went too", got)
	}
	r.Line([]byte(command("dddd", "d", "")[1]))
	r.Line([]byte(command("aaaa", "a", "ok")[2]))
	if got := names(r); !slices.Equal(got, []string{"d", "c"}) {
		t.Errorf("the runs = %q, want none that came back", got)
	}

	// Of many runs that went, the Reducer forgets which once there are
	// 16 times Keep, so that a late line may make one again.
	r = reduce(eventlog.Options{Keep: 1})
	for i := range 20 {
		r.Line([]byte(command(fmt.Sprintf("r%02d", i), "x", "ok")[0]))
	}
	r.Line([]byte(command("r00", "back", "")[1]))
	if got := names(r); !slices.Equal(got, []string{"back"}) {
		t.Errorf("the runs = %q, want the run back", got)
	}
}

func TestReducerDefaults(t *testing.T) {
	for _, o := range []eventlog.Options{{}, {Keep: -1, MaxSpans: -1}} {
		r := eventlog.NewReducer(o)
		for i := range 101 {
			r.Line([]byte(command(fmt.Sprintf("r%03d", i), "x", "")[1]))
		}
		if got := len(r.Runs()); got != 100 {
			t.Errorf("%+v keeps %d runs, want 100", o, got)
		}
	}
}

// Beyond MaxSpans, the spans that start are left out, with their events.
func TestReducerMaxSpans(t *testing.T) {
	r := reduce(eventlog.Options{MaxSpans: 2},
		ev("r", 1, "start", 1, 0, `"kind":"command","name":"c","state":"running"`),
		ev("r", 2, "start", 2, 1, `"kind":"target","name":"t2","state":"running"`),
		ev("r", 3, "start", 3, 1, `"kind":"target","name":"t3","state":"running"`),
		ev("r", 4, "end", 3, 1, `"kind":"target","name":"t3","state":"ended","status":"ok"`),
		ev("r", 5, "start", 4, 3, `"kind":"call","name":"c4","state":"running"`),
	)
	got := r.Runs()[0].Root
	if len(got.Children) != 1 || got.Children[0].Name != "t2" {
		t.Errorf("the spans = %+v, want t2 alone", got.Children)
	}
}

// A span's life: queued, run, its lines, and its end, after which its
// events are left out; a span whose parent the log does not hold, or that
// is its own, is the root's.
func TestReducerSpans(t *testing.T) {
	r := reduce(eventlog.Options{},
		ev("r", 1, "start", 1, 0, `"kind":"command","name":"c","state":"running"`),
		ev("r", 2, "start", 2, 1, `"kind":"step","name":"s","flags":["fold"],"state":"running","total":2`),
		ev("r", 3, "start", 3, 2, `"kind":"target","name":"t","state":"queued","node":"n1"`),
		ev("r", 4, "run", 3, 2, `"kind":"target","name":"t","state":"running","node":"n1"`),
		ev("r", 5, "run", 3, 2, `"kind":"target","name":"t","state":"running","node":"n1"`),
		ev("r", 6, "line", 3, 2, `"kind":"target","name":"t","state":"running","node":"n1","stream":"stdout"`),
		ev("r", 7, "line", 3, 2, `"kind":"target","name":"t","state":"running","node":"n1","stream":"stderr","dropped":3`),
		ev("r", 8, "update", 3, 2, `"kind":"target","name":"t","state":"running","node":"n1","message":"halfway","unit":"percent","size":100`),
		ev("r", 9, "advance", 3, 2, `"kind":"target","name":"t","state":"running","node":"n1","message":"halfway","unit":"percent","size":100,"amount":50`),
		ev("r", 10, "end", 3, 2, `"kind":"target","name":"t","state":"ended","node":"n1","unit":"percent","size":100,"amount":50,"status":"failed","class":"timeout","err":"timed out","dropped":5,"exit":0`),
		ev("r", 11, "update", 3, 2, `"kind":"target","name":"late","state":"running"`),
		ev("r", 12, "start", 5, 9, `"kind":"call","name":"orphan","state":"running"`),
		ev("r", 13, "start", 6, 6, `"kind":"call","name":"self","state":"running"`),
		ev("r", 14, "suspend", 0, 0, ``),
	)
	got := draw(r.Runs()[0])
	want := `run r   last 12:00:14.0 suspended
command "c" running queued=01.0 started=01.0
  step "s" running flags=fold total=2 queued=02.0 started=02.0
    target "t" ended node=n1 work=50/100 percent status=failed class=timeout err=timed out lines=2-5 queued=03.0 started=04.0 ended=10.0 exit=0
  call "orphan" running queued=12.0 started=12.0
  call "self" running queued=13.0 started=13.0
`
	if got != want {
		t.Errorf("the run is\n%s\nwant\n%s", got, want)
	}
	r.Line([]byte(ev("r", 15, "resume", 0, 0, ``)))
	if r.Runs()[0].Suspended {
		t.Error("the run is suspended after a resume")
	}
	// A run that ended is no longer suspended, whatever its log says.
	r.Line([]byte(ev("r", 16, "suspend", 0, 0, ``)))
	r.Line([]byte(ev("r", 17, "end", 1, 0, `"kind":"command","name":"c","state":"ended","status":"ok"`)))
	if ru := r.Runs()[0]; ru.Suspended || !ru.Done() || ru.Last != time.Date(2026, 9, 25, 12, 0, 17, 0, time.UTC) {
		t.Errorf("the ended run = %+v", ru)
	}
}

// A Run, once returned, is never changed: a run is built anew when it
// changes.
func TestReducerRunsStand(t *testing.T) {
	r := reduce(eventlog.Options{}, command("r", "c", "")...)
	r.Line([]byte(ev("r", 2, "start", 2, 1, `"kind":"step","name":"s","state":"running"`)))
	first := r.Runs()
	r.Line([]byte(ev("r", 3, "end", 2, 1, `"kind":"step","name":"s","state":"ended","status":"ok"`)))
	if first[0].Root.Children[0].Done() || !r.Runs()[0].Root.Children[0].Done() {
		t.Error("a Run returned changed with the Reducer, or the Reducer did not")
	}
}

// A Reset forgets every run, and Changed reports them.
func TestReducerReset(t *testing.T) {
	r := reduce(eventlog.Options{Keep: 1}, append(command("aaaa", "a", "ok"), command("bbbb", "b", "")...)...)
	r.Changed()
	r.Reset()
	if got := r.Changed(); !slices.Equal(got, []string{"bbbb"}) {
		t.Errorf("Changed() = %q", got)
	}
	if got := r.Runs(); got != nil {
		t.Errorf("Runs() = %+v, want none", got)
	}
	// A run that had gone may come back once the log is read again.
	r.Line([]byte(command("aaaa", "a", "ok")[1]))
	if got := names(r); !slices.Equal(got, []string{"a"}) {
		t.Errorf("the runs = %q", got)
	}
}
