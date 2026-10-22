// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratreetest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/cobratree"
	"github.com/GSI-HPC/go-clikit/cobratree/cobratreetest"
	"github.com/spf13/cobra"
)

// newTree builds the program's command tree, as its main does.
func newTree() *cobra.Command {
	root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
	get := &cobra.Command{Use: "get"}
	cluster := &cobra.Command{Use: "cluster [NAME]", RunE: func(cmd *cobra.Command, _ []string) error {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "NAME  NODES")
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "prog: no realm given, using default")
		return nil
	}}
	cluster.Flags().StringP("output", "o", "human", "human or json")
	get.AddCommand(cluster)
	root.AddCommand(get)
	cobratree.Strict(root, nil)
	return root
}

// A test runs a command line and checks the result and the diagnostics
// apart.
func ExampleRun() {
	_ = func(t *testing.T) {
		stdout, stderr, err := cobratreetest.Run(t, newTree(), "get", "cluster")
		if err != nil || stdout != "NAME  NODES\n" || stderr != "prog: no realm given, using default\n" {
			t.Errorf("prog get cluster: %q, %q, %v", stdout, stderr, err)
		}
	}
}

// A test of the manual checks every command line its examples show, and
// that it found them.
func ExampleCheckCommandLines() {
	_ = func(t *testing.T) {
		manual := "$ prog get cluster dev -o json\nprog get cluster | wc -l\n"
		if n := cobratreetest.CheckCommandLines(t, newTree(), manual); n < 2 {
			t.Errorf("checked %d command lines of the manual, want 2", n)
		}
	}
}

// Each command line names a command of the tree and its flags, or is
// refused.
func ExampleCheckCommandLine() {
	for _, line := range []string{"get cluster dev -o json", "get clusters", "get cluster --format json"} {
		fmt.Println(cobratreetest.CheckCommandLine(newTree(), strings.Fields(line)))
	}
	// Output:
	// <nil>
	// prog get has no command clusters
	// prog get cluster has no flag --format
}
