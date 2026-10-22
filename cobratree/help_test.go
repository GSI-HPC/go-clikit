// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/cobratree"
	"github.com/spf13/cobra"
)

// topics are a program's help topics; "topics" lists them, and "get" is a
// topic that a command shadows.
func topics(name string) (string, bool) {
	text, ok := map[string]string{
		"realm":  "A realm keeps clusters apart.\n",
		"topics": "realm\n",
		"get":    "the topic, not the command\n",
	}[name]
	return text, ok
}

// executeTopics runs a strict tree with help topics with args.
func executeTopics(args ...string) (string, error) {
	root := tree(moved)
	cobratree.HelpTopics(root, topics)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestHelpTopicsShowsATopic(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"help", "realm"}, "A realm keeps clusters apart.\n"},
		{[]string{"help", "topics"}, "realm\n"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := executeTopics(tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if out != tc.want {
				t.Errorf("wrote %q, want %q", out, tc.want)
			}
		})
	}
}

// A command wins over a topic of its name, and help still shows the
// commands.
func TestHelpTopicsLeavesTheCommands(t *testing.T) {
	for _, args := range [][]string{
		{"help"},
		{"help", "get"},
		{"help", "get", "cluster"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := executeTopics(args...)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "Usage:") || strings.Contains(out, "the topic") {
				t.Errorf("wrote %q, want the command's help", out)
			}
		})
	}
}

// What is neither a command nor a topic is refused as Strict refuses it; a
// topic takes no more words.
func TestHelpTopicsRefusesTheRest(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"help", "xyzzy"}, `unknown help topic "xyzzy": unknown command "xyzzy" for "prog"`},
		{[]string{"help", "crate"}, `unknown help topic "crate": unknown command "crate" for "prog" (did you mean "create"?)`},
		{[]string{"help", "realm", "x"}, `unknown help topic "realm x": unknown command "realm" for "prog"`},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := executeTopics(tc.args...)
			if err == nil || err.Error() != tc.want || !cobratree.IsUsage(err) {
				t.Fatalf("error %v, want the usage error %q", err, tc.want)
			}
			if out != "" {
				t.Errorf("wrote %q, want nothing", out)
			}
		})
	}
}

// On a tree that is not strict, help shows the root's help for a word that
// is neither, as cobra's does.
func TestHelpTopicsOnALenientTree(t *testing.T) {
	root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "version", RunE: run})
	help := cobratree.HelpTopics(root, topics)
	if help.Name() != "help" {
		t.Fatalf("HelpTopics returned %q, want the help command", help.Name())
	}
	for _, tc := range []struct {
		arg, want string
	}{
		{"realm", "A realm keeps clusters apart.\n"},
		{"xyzzy", "Available Commands:"},
	} {
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs([]string{"help", tc.arg})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("help %s wrote %q, want %q in it", tc.arg, out.String(), tc.want)
		}
	}
}

// A help command of the program's own, with a RunE, still runs it, and a
// topic that cannot be written fails the command.
func TestHelpTopicsWithAHelpCommandOfItsOwn(t *testing.T) {
	root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "version", RunE: run})
	ran := false
	root.SetHelpCommand(&cobra.Command{Use: "help", RunE: func(*cobra.Command, []string) error {
		ran = true
		return nil
	}})
	cobratree.HelpTopics(root, topics)
	root.SetArgs([]string{"help", "version"})
	if err := root.Execute(); err != nil || !ran {
		t.Errorf("help version: ran %v, error %v; want the program's help to run", ran, err)
	}

	root.SetOut(failingWriter{})
	root.SetArgs([]string{"help", "realm"})
	if err := root.Execute(); !errors.Is(err, errWrite) {
		t.Errorf("help realm on a failing output: error %v, want %v", err, errWrite)
	}
}

var errWrite = errors.New("write failed")

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }
