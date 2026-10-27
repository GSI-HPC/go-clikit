// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// topicsAnnotation marks a root whose help command shows topics, so that
// Strict tells when it is called again, which would lose them.
const topicsAnnotation = "cobratree.topics"

// Topics lets the help command of a strict tree show topics that are no
// command, such as the concepts of a program's manual: "prog help NAME"
// writes the text lookup returns for NAME to the help command's output,
// as it is, when NAME is one word that names no command and lookup knows
// it. A command of that name wins, and a word lookup does not know is
// refused as Strict refuses it. A program that lists its topics answers a
// word of its choice, such as "topics", with the list.
//
// names returns the names of the topics, which the completion of help
// offers beside the commands, in their order, but for those a command
// shadows or lookup does not know; nil offers none. No word is offered
// after a topic. lookup must not be nil, and Topics panics when it is.
//
// Strict shows the topics after its checks, whose check of help they
// extend, and panics for a tree without subcommands, which has no help
// command. The Short and Long of the help command, cobra's, speak of
// commands alone; the program words them on the command that
// root.Find([]string{"help"}) returns after Strict.
func Topics(lookup func(name string) (text string, ok bool), names func() []string) Option {
	if lookup == nil {
		panic("cobratree: Topics needs a lookup")
	}
	return func(o *options) { o.lookup, o.names = lookup, names }
}

// hasTopics reports whether the help command of root shows topics.
func hasTopics(root *cobra.Command) bool {
	_, ok := root.Annotations[topicsAnnotation]
	return ok
}

// helpCommand returns the help command of the tree under root, which it
// adds as cobra adds it, and panics for a tree that has none.
func helpCommand(root *cobra.Command) *cobra.Command {
	root.InitDefaultHelpCmd()
	help, _, _ := root.Find([]string{"help"})
	if help == root {
		// Cobra adds no help command to a root without subcommands, and
		// Find returns the root itself.
		panic(fmt.Sprintf("cobratree: help topics need a help command, and %q has no subcommands", root.Name()))
	}
	return help
}

// showTopics makes help, the help command of the tree under root, show
// the topics lookup knows, offer those names lists to completion and
// complete no word after a topic, and marks root.
func showTopics(root, help *cobra.Command, lookup func(string) (string, bool), names func() []string) {
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations[topicsAnnotation] = "true"
	// command reports whether name names a command.
	command := func(name string) bool {
		found, _, _ := root.Find([]string{name})
		return found != root
	}
	// text returns the topic args name, when they name one and no command.
	text := func(args []string) (string, bool) {
		if len(args) != 1 || command(args[0]) {
			return "", false
		}
		return lookup(args[0])
	}
	check, run, runE := help.Args, help.Run, help.RunE
	help.Args = func(cmd *cobra.Command, args []string) error {
		if _, ok := text(args); ok || check == nil {
			return nil
		}
		return check(cmd, args)
	}
	help.Run = nil
	help.RunE = func(cmd *cobra.Command, args []string) error {
		if t, ok := text(args); ok {
			_, err := io.WriteString(cmd.OutOrStdout(), t)
			return err
		}
		if runE != nil {
			return runE(cmd, args)
		}
		run(cmd, args)
		return nil
	}
	complete := help.ValidArgsFunction
	help.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			if _, ok := text(args[:1]); ok {
				// A topic takes no more words.
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}
		var comps []cobra.Completion
		directive := cobra.ShellCompDirectiveNoFileComp
		if complete != nil {
			comps, directive = complete(cmd, args, toComplete)
		}
		if len(args) > 0 || names == nil {
			return comps, directive
		}
		for _, name := range names() {
			if _, ok := text([]string{name}); ok && strings.HasPrefix(name, toComplete) {
				comps = append(comps, name)
			}
		}
		return comps, directive
	}
}
