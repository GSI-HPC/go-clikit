// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree

import (
	"io"

	"github.com/spf13/cobra"
)

// HelpTopics lets the help command of the tree under root show topics that
// are no command, such as the concepts of a program's manual: "prog help
// NAME" writes the text topic returns for NAME to the help command's
// output, as it is, when NAME is one word that names no command and topic
// knows it. A command of that name wins, and a word topic does not know is
// refused as Strict refuses it. A program that lists its topics answers a
// word of its choice, such as "topics", with the list.
//
// The program calls HelpTopics after Strict, whose help command it extends;
// on a tree that is not strict, a word that is neither a command nor a
// topic shows the root's help, as cobra's help does. It returns the help
// command, whose Short and Long, cobra's, speak of commands alone, for the
// program to word.
func HelpTopics(root *cobra.Command, topic func(name string) (text string, ok bool)) *cobra.Command {
	root.InitDefaultHelpCmd()
	help, _, _ := root.Find([]string{"help"})
	// text returns the topic args name, when they name one and no command.
	text := func(args []string) (string, bool) {
		if len(args) != 1 {
			return "", false
		}
		if found, _, _ := root.Find(args); found != root {
			return "", false
		}
		return topic(args[0])
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
	return help
}
