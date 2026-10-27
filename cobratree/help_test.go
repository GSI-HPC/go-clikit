// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree_test

import (
	"bytes"
	"errors"
	"io"
	"slices"
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

// topicNames lists the topics, "get" among them, and "gone", which topics
// does not know.
func topicNames() []string { return []string{"topics", "realm", "get", "gone"} }

// executeTopics runs a strict tree with help topics with args.
func executeTopics(args ...string) (string, error) {
	root := tree(cobratree.Moved(moved), cobratree.Topics(topics, topicNames))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestTopicsShowsATopic(t *testing.T) {
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
func TestTopicsLeavesTheCommands(t *testing.T) {
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
func TestTopicsRefusesTheRest(t *testing.T) {
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

// A help command of the program's own, with a RunE, still runs it, and a
// topic that cannot be written fails the command.
func TestTopicsWithAHelpCommandOfItsOwn(t *testing.T) {
	root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "version", RunE: run})
	ran := false
	root.SetHelpCommand(&cobra.Command{Use: "help", RunE: func(*cobra.Command, []string) error {
		ran = true
		return nil
	}})
	cobratree.Strict(root, cobratree.Topics(topics, topicNames))
	root.SetArgs([]string{"help", "version"})
	if err := root.Execute(); err != nil || !ran {
		t.Errorf("help version: ran %v, error %v; want the program's help to run", ran, err)
	}

	root.SetOut(failingWriter{})
	root.SetArgs([]string{"help", "realm"})
	if err := root.Execute(); !errors.Is(err, errWrite) {
		t.Errorf("help realm on a failing output: error %v, want %v", err, errWrite)
	}

	// The program's help completes nothing itself; the topics are offered.
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"__complete", "help", "r"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := completions(out.String()); !slices.Equal(got, []string{"realm"}) {
		t.Errorf("completed %q, want realm", got)
	}
}

// completions returns the completions cobra's __complete wrote in out,
// without the line of the directive.
func completions(out string) []string {
	var got []string
	for line := range strings.Lines(out) {
		if strings.HasPrefix(line, ":") {
			break
		}
		got = append(got, strings.TrimSuffix(line, "\n"))
	}
	return got
}

// help completes the topics after the commands, but for those a command
// shadows and those the lookup does not know, and only as its first word,
// and nothing after a topic.
func TestTopicsAreCompleted(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"help", "re"}, []string{"realm"}},
		{[]string{"help", "t"}, []string{"topics"}},
		{[]string{"help", "g"}, []string{"get\tShow things"}},
		{[]string{"help", "get", ""}, []string{"cluster", "nodes"}},
		{[]string{"help", "realm", ""}, nil},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := executeTopics(append([]string{"__complete"}, tc.args...)...)
			if err != nil {
				t.Fatal(err)
			}
			if got := completions(out); !slices.Equal(got, tc.want) {
				t.Errorf("completed %q, want %q", got, tc.want)
			}
		})
	}
	out, err := executeTopics("__complete", "help", "")
	if err != nil {
		t.Fatal(err)
	}
	got := completions(out)
	if n := len(got); n < 3 || !slices.Equal(got[n-2:], []string{"topics", "realm"}) || !slices.Contains(got, "version") {
		t.Errorf("completed %q, want the commands, then topics and realm", got)
	}
}

// Without a list of names, help completes the commands alone.
func TestTopicsWithoutNames(t *testing.T) {
	root := tree(cobratree.Topics(topics, nil))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"__complete", "help", "re"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := completions(out.String()); got != nil {
		t.Errorf("completed %q, want nothing", got)
	}
	out.Reset()
	root.SetArgs([]string{"__complete", "help", "get", "n"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := completions(out.String()); !slices.Equal(got, []string{"nodes"}) {
		t.Errorf("completed %q, want nodes", got)
	}
	out.Reset()
	root.SetArgs([]string{"help", "realm"})
	if err := root.Execute(); err != nil || out.String() != "A realm keeps clusters apart.\n" {
		t.Errorf("help realm: %q, %v; want the topic", out.String(), err)
	}
}

// Topics refuses a nil lookup, and a tree without subcommands, which has
// no help command, before Strict changes it.
func TestTopicsRefuses(t *testing.T) {
	wantPanic(t, "cobratree: Topics needs a lookup", func() { cobratree.Topics(nil, topicNames) })

	ran := false
	root := &cobra.Command{Use: "prog NAME", Args: cobra.ExactArgs(1), SilenceErrors: true, SilenceUsage: true, RunE: func(*cobra.Command, []string) error {
		ran = true
		return nil
	}}
	wantPanic(t, `cobratree: help topics need a help command, and "prog" has no subcommands`, func() {
		cobratree.Strict(root, cobratree.Topics(topics, topicNames))
	})
	root.SetArgs([]string{"realm"})
	if err := root.Execute(); err != nil || !ran {
		t.Errorf("prog realm: ran %v, error %v; want the root to run", ran, err)
	}
}

// Strict after a Strict with Topics would drop the topics, and panics.
func TestTopicsInTheWrongOrder(t *testing.T) {
	again := `cobratree: Strict called on "prog", whose help shows topics already, which it would drop: pass Topics to one call of Strict instead`
	wantPanic(t, again, func() { cobratree.Strict(tree(cobratree.Topics(topics, nil))) })
}

// wantPanic runs f and fails t unless f panics with want.
func wantPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		if r := recover(); r != want {
			t.Errorf("panic %v, want %q", r, want)
		}
	}()
	f()
}

var errWrite = errors.New("write failed")

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWrite }
