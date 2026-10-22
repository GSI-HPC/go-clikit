// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliout_test

import (
	"fmt"
	"os"

	"github.com/GSI-HPC/go-clikit/cliout"
)

// A command writes its result as a table, the wide columns only for -o
// wide; a reason a node gave is escaped, so it cannot start a row of its
// own.
func ExampleTable() {
	table := &cliout.Table{
		Columns: []cliout.Column{{Name: "NODE"}, {Name: "CPUS", Right: true}, {Name: "STATE"}, {Name: "REASON", Wide: true}},
		Caption: "3 nodes, 1 drained",
	}
	table.Add("exe0001", "128", "drained", "fan failure\nexe0002 idle")
	table.Add("exe0002", "64", "idle")
	table.Add("exe10", "8", "idle")

	for _, wide := range []bool{false, true} {
		if err := table.Write(os.Stdout, wide); err != nil {
			fmt.Println(err)
		}
		fmt.Println("--")
	}
	// Output:
	// NODE     CPUS  STATE
	// exe0001   128  drained
	// exe0002    64  idle
	// exe10       8  idle
	//
	// 3 nodes, 1 drained
	// --
	// NODE     CPUS  STATE    REASON
	// exe0001   128  drained  fan failure\nexe0002 idle
	// exe0002    64  idle
	// exe10       8  idle
	//
	// 3 nodes, 1 drained
	// --
}

// The same result for -o json: a slice is written an element at a time,
// and a string cannot act on the terminal it is read in.
func ExampleWriteJSON() {
	type node struct {
		Name   string `json:"name"`
		State  string `json:"state"`
		Reason string `json:"reason,omitempty"`
	}
	nodes := []node{
		{Name: "exe0001", State: "drained", Reason: "fan <1> failure\u009b2J"},
		{Name: "exe0002", State: "idle"},
	}
	if err := cliout.WriteJSON(os.Stdout, nodes); err != nil {
		fmt.Println(err)
	}
	// Output:
	// [
	//   {
	//     "name": "exe0001",
	//     "state": "drained",
	//     "reason": "fan <1> failure\u009b2J"
	//   },
	//   {
	//     "name": "exe0002",
	//     "state": "idle"
	//   }
	// ]
}
