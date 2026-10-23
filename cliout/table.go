// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package cliout writes the result of a command for a person and for a
// program: a Table in columns aligned by the columns a terminal gives its
// text, and JSON, every string in either made safe for a terminal.
//
// A cell or a string is often text from elsewhere, the reason a node gave
// for being down or the output of a program on it, which may hold a newline
// that would start a row that seems to be another, or a control character
// that would rewrite what the terminal shows. Table escapes every cell with
// termtext before it measures it, and WriteJSON writes the runes termtext
// escapes, and encoding/json does not, as JSON escapes.
//
//	NODE     STATE    REASON
//	exe0001  drained  fan failure\nexe0002 idle
//	exe0003  idle
//
// Which words a flag such as -o takes, and which of them the command's
// result is written in, are the program's: the package parses no flag and
// knows no format but these two.
package cliout

import (
	"io"
	"slices"
	"strings"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// Column is a column of a Table.
type Column struct {
	// Name is the column's heading, upper case by convention, such as
	// "NODE".
	Name string
	// Wide keeps the column out of the table unless a wide one is asked
	// for, as with -o wide.
	Wide bool
	// Right aligns the column's cells to the right, for counts.
	Right bool
}

// Table is a command's result in rows and columns. No cell is cut short: a
// long reason or a wide node set is worth a line that wraps, and a cell cut
// has hidden the half of a message that mattered.
type Table struct {
	Columns []Column
	// Rows are the cells of each row, in the order of Columns. A row with
	// fewer cells leaves the last columns empty, and one with more has
	// them left out, so a column added later needs no row changed.
	Rows [][]string
	// Caption is written under the table, after an empty line, for a total
	// or a note; empty writes none.
	Caption string
}

// Add appends a row of cells. It copies them, so a caller may reuse the
// slice it passed for the next row (decision 42).
func (t *Table) Add(cells ...string) {
	t.Rows = append(t.Rows, slices.Clone(cells))
}

// Write writes t to w: the headings, then a line for each row, the cells of
// a column aligned by the columns Width gives them and two spaces between
// columns, and the caption, if any, after an empty line. The columns marked
// Wide are written only when wide is set. Every cell and heading is
// escaped with termtext.Escape, and the caption with termtext.EscapeLines,
// before it is measured. No line ends in a space: the last column is not
// padded, so that a line copied takes no spaces with it.
//
// A table with no column to write writes nothing.
func (t *Table) Write(w io.Writer, wide bool) error {
	var shown []int
	for i, c := range t.Columns {
		if !c.Wide || wide {
			shown = append(shown, i)
		}
	}
	if len(shown) == 0 {
		return nil
	}
	lines := make([][]string, 0, len(t.Rows)+1)
	headings := make([]string, len(shown))
	for j, i := range shown {
		headings[j] = termtext.Escape(t.Columns[i].Name)
	}
	lines = append(lines, headings)
	for _, row := range t.Rows {
		cells := make([]string, len(shown))
		for j, i := range shown {
			if i < len(row) {
				cells[j] = termtext.Escape(row[i])
			}
		}
		lines = append(lines, cells)
	}
	widths := make([]int, len(shown))
	for _, cells := range lines {
		for j, cell := range cells {
			widths[j] = max(widths[j], termtext.Width(cell))
		}
	}

	var b, line strings.Builder
	for _, cells := range lines {
		line.Reset()
		for j, cell := range cells {
			if j > 0 {
				line.WriteString("  ")
			}
			pad := strings.Repeat(" ", widths[j]-termtext.Width(cell))
			if t.Columns[shown[j]].Right {
				line.WriteString(pad + cell)
			} else {
				line.WriteString(cell + pad)
			}
		}
		// The padding of the last column is left out, and so is that of
		// the columns before it when its cells are empty.
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	if t.Caption != "" {
		b.WriteString("\n" + termtext.EscapeLines(t.Caption) + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
