// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliout_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/cliout"
)

// write returns what t writes.
func write(t *testing.T, table *cliout.Table, wide bool) string {
	t.Helper()
	var b strings.Builder
	if err := table.Write(&b, wide); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func nodes() *cliout.Table {
	table := &cliout.Table{Columns: []cliout.Column{
		{Name: "NODE"}, {Name: "CPUS", Right: true}, {Name: "STATE"}, {Name: "REASON", Wide: true},
	}}
	table.Add("exe0001", "128", "drained", "firmware update, ticket 4711")
	table.Add("exe0002", "64", "idle")
	table.Add("exe10", "8", "down", "no answer", "a cell beyond the columns")
	return table
}

func TestTableWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		wide bool
		want string
	}{
		{"narrow", false, "" +
			"NODE     CPUS  STATE\n" +
			"exe0001   128  drained\n" +
			"exe0002    64  idle\n" +
			"exe10       8  down\n"},
		{"wide", true, "" +
			"NODE     CPUS  STATE    REASON\n" +
			"exe0001   128  drained  firmware update, ticket 4711\n" +
			"exe0002    64  idle\n" +
			"exe10       8  down     no answer\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := write(t, nodes(), tc.wide); got != tc.want {
				t.Errorf("wrote\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestTableWriteCaption(t *testing.T) {
	table := nodes()
	table.Caption = "3 nodes\n1 drained\x1b[2J"
	got := write(t, table, false)
	if want := "exe10       8  down\n\n3 nodes\n1 drained\\x1b[2J\n"; !strings.HasSuffix(got, want) {
		t.Errorf("wrote\n%s\nwant it to end with\n%s", got, want)
	}
}

func TestTableWriteNothing(t *testing.T) {
	for name, table := range map[string]*cliout.Table{
		"no columns":        {Rows: [][]string{{"a"}}},
		"only wide columns": {Columns: []cliout.Column{{Name: "A", Wide: true}}, Rows: [][]string{{"a"}}},
	} {
		if got := write(t, table, false); got != "" {
			t.Errorf("%s: wrote %q, want nothing", name, got)
		}
	}
	empty := &cliout.Table{Columns: []cliout.Column{{Name: "NODE"}, {Name: "STATE"}}}
	if got := write(t, empty, false); got != "NODE  STATE\n" {
		t.Errorf("a table without rows wrote %q, want its headings", got)
	}
}

// hostile is what a node that was taken over might answer: a carriage
// return and cursor-up to write over the line of another node, and OSC 52
// to write the clipboard.
const hostile = "ok\r\x1b[1Aexe0001: \x1b]52;c;ZXZpbA==\x07evil"

// A cell cannot start a row that seems to be another, or act on the
// terminal.
func TestTableEscapesEveryCell(t *testing.T) {
	table := &cliout.Table{Columns: []cliout.Column{{Name: "NODE"}, {Name: "STATE"}, {Name: "REASON\x1b"}}}
	table.Add("exe0001", "drained", "bad\nexe0002  idle     fine")
	table.Add("exe0003", "idle", hostile)
	got := write(t, table, false)
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasPrefix(line, "exe0002") {
			t.Errorf("wrote a forged row:\n%s", got)
		}
	}
	if strings.ContainsAny(got, "\r\x1b\x07") {
		t.Errorf("wrote control characters: %q", got)
	}
	if !strings.Contains(got, `bad\nexe0002`) || !strings.Contains(got, `REASON\x1b`) {
		t.Errorf("does not show the escapes:\n%s", got)
	}
}

// The columns are measured once the cells are escaped, and by the columns
// a terminal gives wide characters.
func TestTableAligns(t *testing.T) {
	table := &cliout.Table{Columns: []cliout.Column{{Name: "NODE"}, {Name: "REASON"}, {Name: "STATE"}}}
	table.Add("exe1", "失败", "down")
	table.Add("exe2", "✅ ok", "idle")
	table.Add("exe3", "x\ty", "idle")
	got := write(t, table, false)
	want := "" +
		"NODE  REASON  STATE\n" +
		"exe1  失败    down\n" +
		"exe2  ✅ ok   idle\n" +
		`exe3  x\ty    idle` + "\n"
	if got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}

// No cell is cut, and no line ends in a space.
func TestTableCutsNothing(t *testing.T) {
	long := "a reason long enough that a column of a fixed width would have cut it off"
	table := &cliout.Table{Columns: []cliout.Column{{Name: "NODE"}, {Name: "REASON"}, {Name: "NOTE"}}}
	table.Add("exe1", long, "")
	table.Add("exe2", "", "")
	table.Add("exe3", "ok", "trailing ")
	got := write(t, table, false)
	if !strings.Contains(got, long) {
		t.Errorf("cut the cell:\n%s", got)
	}
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasSuffix(line, " ") {
			t.Errorf("the line %q ends in a space", line)
		}
	}
}

var errWrite = errors.New("write failed")

// failing fails every write.
type failing struct{}

func (failing) Write([]byte) (int, error) { return 0, errWrite }

func TestTableWriteFails(t *testing.T) {
	if err := nodes().Write(failing{}, false); !errors.Is(err, errWrite) {
		t.Errorf("error %v, want %v", err, errWrite)
	}
}
