// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/cobratree"
	"github.com/spf13/cobra"
)

// run is a command that does nothing.
func run(*cobra.Command, []string) error { return nil }

// tree builds a program's command tree, made strict with moved.
func tree(moved map[string]string) *cobra.Command {
	root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("realm", "", "the realm")
	get := &cobra.Command{Use: "get", Short: "Show things"}
	get.AddCommand(
		&cobra.Command{Use: "cluster [NAME]", Args: cobra.MaximumNArgs(1), RunE: run},
		&cobra.Command{Use: "nodes", RunE: run},
	)
	create := &cobra.Command{Use: "create", Short: "Make things"}
	create.AddCommand(&cobra.Command{Use: "cluster", Args: cobra.NoArgs, RunE: run})
	power := &cobra.Command{Use: "power", Short: "Switch nodes"}
	on := &cobra.Command{Use: "on NODES", Args: cobra.MinimumNArgs(1), RunE: run}
	on.Flags().IntP("count", "c", 1, "how many")
	power.AddCommand(on)
	root.AddCommand(get, create, power,
		&cobra.Command{Use: "version", Args: cobra.NoArgs, RunE: run},
		&cobra.Command{Use: "logs", RunE: run}, // no check: any arguments
	)
	cobratree.Strict(root, moved)
	return root
}

// execute runs a strict tree with args.
func execute(moved map[string]string, args ...string) (string, error) {
	root := tree(moved)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

var moved = map[string]string{"status": "get cluster", "get machines": "get nodes"}

func TestStrictRefusesWhatNamesNoCommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"xyzzy"}, `unknown command "xyzzy" for "prog"`},
		{[]string{"gxx"}, `unknown command "gxx" for "prog" (did you mean "get"?)`},
		{[]string{"gte"}, `unknown command "gte" for "prog" (did you mean "get"?)`},
		{[]string{"--realm", "r", "gte"}, `unknown command "gte" for "prog" (did you mean "get"?)`},
		{[]string{"status", "default"}, `unknown command "status" for "prog" (it is now "get cluster")`},
		{[]string{"get", "clsuter"}, `unknown command "clsuter" for "prog get" (did you mean "cluster"?)`},
		{[]string{"get", "node"}, `unknown command "node" for "prog get" (did you mean "nodes"?)`},
		{[]string{"get", "machines"}, `unknown command "machines" for "prog get" (it is now "get nodes")`},
		{[]string{"create", "machines"}, `unknown command "machines" for "prog create"`},
		{[]string{"get", "bogus"}, `unknown command "bogus" for "prog get"`},
		{[]string{"create", "status"}, `unknown command "status" for "prog create"`},
		{[]string{"completion", "nushell"}, `unknown command "nushell" for "prog completion"`},
		{[]string{"completion", "zshh"}, `unknown command "zshh" for "prog completion" (did you mean "zsh"?)`},
		{[]string{"help", "xyzzy"}, `unknown help topic "xyzzy": unknown command "xyzzy" for "prog"`},
		{[]string{"help", "crate"}, `unknown help topic "crate": unknown command "crate" for "prog" (did you mean "create"?)`},
		{[]string{"help", "status"}, `unknown help topic "status": unknown command "status" for "prog" (it is now "get cluster")`},
		{[]string{"help", "get", "clustr"}, `unknown help topic "get clustr": unknown command "clustr" for "prog get" (did you mean "cluster"?)`},
		{[]string{"help", "get", "machines"}, `unknown help topic "get machines": unknown command "machines" for "prog get" (it is now "get nodes")`},
		{[]string{"help", "version", "extra"}, `unknown help topic "version extra": unknown command "extra" for "prog version"`},
		{[]string{"version", "extra"}, `unknown command "extra" for "prog version" (usage: prog version)`},
		{[]string{"get", "cluster", "a", "b"}, `accepts at most 1 arg(s), received 2 (usage: prog get cluster [NAME])`},
		{[]string{"power", "on"}, `requires at least 1 arg(s), only received 0 (usage: prog power on NODES)`},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := execute(moved, tc.args...)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
			if !cobratree.IsUsage(err) {
				t.Errorf("%v is no usage error", err)
			}
			if out != "" {
				t.Errorf("wrote %q, want nothing", out)
			}
		})
	}
}

// A command that moved is named only where no subcommand is close.
func TestStrictSuggestsBeforeItNamesAMove(t *testing.T) {
	_, err := execute(map[string]string{"gte": "get cluster"}, "gte")
	want := `unknown command "gte" for "prog" (did you mean "get"?)`
	if err == nil || err.Error() != want {
		t.Errorf("error %v, want %q", err, want)
	}
}

func TestStrictTakesNoMoves(t *testing.T) {
	_, err := execute(nil, "status")
	want := `unknown command "status" for "prog"`
	if err == nil || err.Error() != want {
		t.Errorf("error %v, want %q", err, want)
	}
}

// A command's own SuggestionsMinimumDistance holds.
func TestStrictKeepsTheDistanceOfSuggestions(t *testing.T) {
	root := tree(nil)
	root.SuggestionsMinimumDistance = 1
	root.SetArgs([]string{"gxx"})
	err := root.Execute()
	want := `unknown command "gxx" for "prog"`
	if err == nil || err.Error() != want {
		t.Errorf("error %v, want %q", err, want)
	}
}

func TestStrictLeavesWhatNamesACommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string // in what the command wrote
	}{
		{nil, "Available Commands:"},
		{[]string{"get"}, "Available Commands:"},
		{[]string{"power"}, "Available Commands:"},
		{[]string{"completion"}, "Available Commands:"},
		{[]string{"help"}, "Usage:"},
		{[]string{"help", "get"}, "Usage:"},
		{[]string{"help", "get", "cluster"}, "Usage:"},
		{[]string{"help", "completion", "bash"}, "Usage:"},
		{[]string{"help", "help"}, "Usage:"},
		{[]string{"get", "cluster", "dev"}, ""},
		{[]string{"power", "on", "n1", "n2"}, ""},
		{[]string{"logs", "a", "b", "c"}, ""},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := execute(moved, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("wrote %q, want %q in it", out, tc.want)
			}
		})
	}
}

// Strict adds the completion command now, so the root's output when Strict
// is called is where its scripts go.
func TestStrictWritesCompletionToTheOutputItWasGiven(t *testing.T) {
	root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "version", RunE: run})
	var out bytes.Buffer
	root.SetOut(&out)
	cobratree.Strict(root, nil)
	root.SetArgs([]string{"completion", "bash"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "bash completion") {
		t.Errorf("wrote %.80q, want a bash completion script", out.String())
	}
}

// A tree with no subcommands gets no help or completion command.
func TestStrictOnALoneCommand(t *testing.T) {
	root := &cobra.Command{Use: "prog NAME", Args: cobra.ExactArgs(1), RunE: run, SilenceErrors: true, SilenceUsage: true}
	cobratree.Strict(root, nil)
	if root.HasSubCommands() {
		t.Errorf("subcommands %v, want none", root.Commands())
	}
	root.SetArgs(nil)
	err := root.Execute()
	want := "accepts 1 arg(s), received 0 (usage: prog NAME)"
	if err == nil || err.Error() != want || !cobratree.IsUsage(err) {
		t.Errorf("error %v, want the usage error %q", err, want)
	}
}

func TestFlagErrorsAreUsageErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--bogus", "get", "nodes"}, "unknown flag: --bogus"},
		{[]string{"get", "nodes", "--bogus"}, "unknown flag: --bogus"},
		{[]string{"power", "on", "-x", "n1"}, "unknown shorthand flag: 'x' in -x"},
		{[]string{"power", "on", "n1", "--count"}, "flag needs an argument: --count"},
		{[]string{"power", "on", "n1", "--count", "many"}, `invalid argument "many" for "-c, --count" flag`},
		{[]string{"get", "nodes", "---realm"}, "bad flag syntax: ---realm"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			_, err := execute(nil, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q in it", err, tc.want)
			}
			if !cobratree.IsUsage(err) {
				t.Errorf("%v is no usage error", err)
			}
		})
	}
}
