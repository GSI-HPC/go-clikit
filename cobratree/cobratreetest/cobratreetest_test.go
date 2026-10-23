// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratreetest

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/cobratree"
	"github.com/spf13/cobra"
)

// fakeTB is a testing.TB that keeps what a helper reports, for a test of
// the helper itself.
type fakeTB struct {
	testing.TB
	errors []string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}

// tree builds a program's command tree, made strict.
func tree() *cobra.Command {
	run := func(*cobra.Command, []string) error { return nil }
	root := &cobra.Command{Use: "prog", Version: "v1.2.3", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("realm", "", "the realm")
	root.PersistentFlags().CountP("verbose", "v", "more")
	get := &cobra.Command{Use: "get", Aliases: []string{"show"}}
	cluster := &cobra.Command{Use: "cluster [NAME]", RunE: run}
	cluster.Flags().StringP("output", "o", "human", "human or json")
	cluster.Flags().BoolP("all", "A", false, "every cluster")
	get.AddCommand(cluster)
	env := &cobra.Command{Use: "env NAME", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "export REALM=%s\n", args[0])
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "prog: note")
		return nil
	}}
	root.AddCommand(get, env)
	cobratree.Strict(root)
	return root
}

func TestRun(t *testing.T) {
	stdout, stderr, err := Run(t, tree(), "env", "dev")
	if err != nil || stdout != "export REALM=dev\n" || stderr != "prog: note\n" {
		t.Errorf("Run = %q, %q, %v; want the export, the note and no error", stdout, stderr, err)
	}
	stdout, stderr, err = Run(t, tree(), "env")
	if !cobratree.IsUsage(err) || stdout != "" || stderr != "" {
		t.Errorf("Run = %q, %q, %v; want a usage error alone", stdout, stderr, err)
	}
}

// No args runs the program with none, whatever the test binary was given.
func TestRunWithNoArgs(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	os.Args = []string{"prog.test", "bogus"}
	stdout, _, err := Run(t, tree())
	if err != nil || !strings.Contains(stdout, "Available Commands:") {
		t.Errorf("Run = %q, %v; want the root's help", stdout, err)
	}
}

func TestCheckCommandLine(t *testing.T) {
	for _, line := range []string{
		"",
		"get cluster",
		"show cluster",
		"get cluster dev -o json",
		"get cluster dev --output=json -A",
		"get cluster -Ao json",
		"-vvv --realm dev get cluster",
		"get cluster NAME --all",
		"get [flags]",
		"get cluster <name> extra words",
		"env dev",
		"help get cluster",
		"get --help",
		"--version",
		"completion bash",
		"get cluster -- --no-such-flag",
		"get cluster -",
	} {
		if err := CheckCommandLine(tree(), strings.Fields(line)); err != nil {
			t.Errorf("prog %s: %v", line, err)
		}
	}
	for line, want := range map[string]string{
		"getx":                   "prog has no command getx",
		"get clusters":           "prog get has no command clusters",
		"get cluster --bogus":    "prog get cluster has no flag --bogus",
		"get cluster -Z":         "prog get cluster has no flag -Z",
		"get cluster -AZ":        "prog get cluster has no flag -Z",
		"env --output json":      "prog env has no flag --output",
		"NAME get cluster --x=1": "prog has no flag --x",
	} {
		err := CheckCommandLine(tree(), strings.Fields(line))
		if err == nil || err.Error() != want {
			t.Errorf("prog %s: error %v, want %q", line, err, want)
		}
	}
}

// A word under a runnable command with subcommands is refused only when
// the command's own check refuses its arguments, as Execute refuses them.
func TestCheckCommandLineUnderARunnableGroup(t *testing.T) {
	run := func(*cobra.Command, []string) error { return nil }
	build := func(rootRuns bool) *cobra.Command {
		root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
		if rootRuns {
			root.RunE = run
		}
		get := &cobra.Command{Use: "get [NODE]", Args: cobra.MaximumNArgs(1), RunE: run}
		get.Flags().StringP("output", "o", "human", "human or json")
		if err := get.MarkFlagRequired("output"); err != nil {
			t.Fatal(err)
		}
		get.AddCommand(&cobra.Command{Use: "cluster", RunE: run})
		logs := &cobra.Command{Use: "logs", RunE: run} // no check: any arguments
		logs.AddCommand(&cobra.Command{Use: "clear", RunE: run})
		power := &cobra.Command{Use: "power"} // runs nothing, unless Strict makes it
		power.AddCommand(&cobra.Command{Use: "on", RunE: run})
		root.AddCommand(get, logs, power)
		return root
	}
	for _, strict := range []bool{false, true} {
		newTree := func(rootRuns bool) *cobra.Command {
			root := build(rootRuns)
			if strict {
				cobratree.Strict(root)
			}
			return root
		}
		for line, want := range map[string]string{
			"get mynode":         "",
			"get mynode -o json": "",
			"get -o json mynode": "",
			"get a b":            "prog get has no command a",
			"get a -o json b":    "prog get has no command a",
			"get -- a":           "",
			"get a -- b":         "prog get has no command a",
			"get cluster":        "",
			"logs a b c":         "",
			"get mynode --bogus": "prog get has no flag --bogus",
			"power bogus":        "prog power has no command bogus",
		} {
			err := CheckCommandLine(newTree(false), strings.Fields(line))
			if got := fmt.Sprint(err); got != cmp.Or(want, "<nil>") {
				t.Errorf("strict %v: prog %s: error %v, want %q", strict, line, err, want)
			}
		}
		// Execute agrees.
		for line, ok := range map[string]bool{"get mynode -o json": true, "get a -o json b": false} {
			root := newTree(false)
			root.SetArgs(strings.Fields(line))
			if err := root.Execute(); (err == nil) != ok {
				t.Errorf("strict %v: prog %s: Execute error %v", strict, line, err)
			}
		}
		// A runnable root takes no argument, by cobra's check or by Strict's.
		err := CheckCommandLine(newTree(true), []string{"gte"})
		if got := fmt.Sprint(err); got != "prog has no command gte" {
			t.Errorf("strict %v: prog gte: error %v, want prog has no command gte", strict, err)
		}
	}
}

func TestCheckCommandLines(t *testing.T) {
	text := `# Show what there is.
$ prog get cluster --realm dev
prog show cluster -o json > clusters.json # to keep
eval "$(prog env dev)"
echo prog is not run here
prog get cluster && prog get cluster -A | grep dev; prog get \
    clusters 2>/dev/null
other prog get cluster & prog getx
$ prog env 'a b' "c \"d\"" e\ f
`
	tb := &fakeTB{}
	if got := CheckCommandLines(tb, tree(), text); got != 8 {
		t.Errorf("checked %d command lines, want 8", got)
	}
	want := []string{
		`line 6: "prog get cluster && prog get cluster -A | grep dev; prog get clusters 2>/dev/null": prog get has no command clusters`,
		`line 8: "other prog get cluster & prog getx": prog has no command getx`,
	}
	if !slices.Equal(tb.errors, want) {
		t.Errorf("reported %q, want %q", tb.errors, want)
	}
}

func TestCommandLines(t *testing.T) {
	for line, want := range map[string][]string{
		`prog use dev && prog get cluster`:        {"use dev", "get cluster"},
		`eval "$(prog env dev)"`:                  {"env dev"},
		`x $(prog env a) $(prog env b)`:           {"env a", "env b"},
		`echo hi | prog get cluster -- cat`:       {"get cluster -- cat"},
		`prog get cluster -o json > out # note`:   {"get cluster -o json"},
		`prog get cluster 2>&1 | less`:            {"get cluster"},
		`prog get cluster &>log`:                  {"get cluster"},
		`prog get cluster <in`:                    {"get cluster"},
		`echo prog is nice`:                       nil,
		`prog get cluster;prog env x||prog env y`: {"get cluster", "env x", "env y"},
		`a|prog get cluster&`:                     {"get cluster"},
		`prog 'get' "cluster" a\ b '#' "x y" # c`: {"get cluster a b # x y"},
		`prog "unterminated`:                      {"unterminated"},
		"prog\tget\t\tcluster \\":                 {"get cluster \\"},
	} {
		var got []string
		for _, words := range commandLines("prog", line) {
			got = append(got, strings.Join(words, " "))
		}
		if !slices.Equal(got, want) {
			t.Errorf("commandLines(%q) = %q, want %q", line, got, want)
		}
	}
}

// Text that runs no command line checks none.
func TestCheckCommandLinesWithoutAny(t *testing.T) {
	tb := &fakeTB{}
	if got := CheckCommandLines(tb, tree(), "no command here\n"); got != 0 || len(tb.errors) != 0 {
		t.Errorf("checked %d, reported %q; want nothing", got, tb.errors)
	}
}
