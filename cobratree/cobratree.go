// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package cobratree makes a cobra command tree strict about its command
// line, marks the errors of a command line as usage errors, and reads what
// the Go toolchain recorded about the binary, for its version command.
//
// Cobra is lenient where a script needs it strict: a command that only
// holds subcommands prints its help and succeeds for a word that names
// none of them, so a script that misspelt one goes on to its next step;
// help prints the root's help, and succeeds, for a topic that names no
// command; and its argument checks, such as cobra.ExactArgs, return plain
// errors that name neither the arguments wanted nor the command. Strict
// fixes all three once the tree is built:
//
//	prog gte          unknown command "gte" for "prog" (did you mean "get"?)
//	prog status       unknown command "status" for "prog" (it is now "get cluster")
//	prog help crate   unknown help topic "crate": unknown command "crate" for "prog" (did you mean "create"?)
//	prog power on     requires at least 1 arg(s), only received 0 (usage: prog power on NODES)
//
// Every such error is a usage error, as Usage makes one, and so are
// cobra's errors for a required flag not given and for the flags of a
// group, those marked with MarkFlagsRequiredTogether, MarkFlagsOneRequired
// or MarkFlagsMutuallyExclusive, and an error pflag returns for a flag it
// cannot parse, wherever cobra parsed it: IsUsage tells them from the
// errors of a command that ran and failed. The package names no exit
// code: a program maps a usage error to its own, commonly 2. FlagError,
// which a program sets as the tree's FlagErrorFunc, words a flag's value
// that pflag cannot parse as InvalidValue words one the program refuses
// itself, invalid --count value "x": want a whole number.
// Coded gives an error the exit status the code nearer its cause knows,
// such as that of a host that could not be reached, and Status returns it.
//
// HelpTopics lets help show topics that are no command, such as the
// concepts of a program's manual, beside the commands.
//
// ReadBuildInfo returns the module version, the revision and the time of
// the commit, and whether the checkout was modified, as the toolchain
// recorded them, and BuildInfo.Resolve puts the version and commit a
// release build set with -ldflags in their place. How a program shows
// them, the word it uses for a build that is no release and how much of
// the revision it prints, BuildInfo.Short, is the program's.
//
// The package knows no program: the names in its errors are those of the
// commands in the tree, and the commands a program renamed are its data.
package cobratree

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// Strict makes the command tree under root strict about its command line.
// The program calls it once the tree is built, after its last AddCommand,
// and before Execute:
//
//   - It adds cobra's help and completion commands now, rather than when
//     the tree runs, so that the checks below cover them. The completion
//     scripts are written to the output root has when Strict is called.
//   - A command that holds subcommands and runs nothing, completion among
//     them, prints its help when it is given no argument, and refuses any
//     argument: unknown command "word" for "prog group". The closest
//     subcommand is suggested, within cobra's SuggestionsMinimumDistance,
//     2 unless the command sets its own; with none, a command that moved
//     names its new place.
//   - help refuses a topic that names no command, all of it, and says
//     which word is not known.
//   - Every argument check in the tree returns a usage error, and that of
//     a command that holds no subcommands ends with the command's usage
//     line, since a program that silences cobra's usage block shows none.
//   - Every runnable command, one without an argument check included,
//     checks its required flags and its flag groups with its arguments,
//     before its PersistentPreRun and PreRun hooks rather than after them
//     as cobra does, and returns cobra's error for them as a usage error:
//     required flag(s) "realm" not set. A hook that sets a required flag
//     is too late for this check.
//
// moved maps a command line a program no longer takes, its words after the
// program's name, such as "status" or "get node", to the command line that
// replaced it, such as "get cluster", for the error that a script calling
// the old one gets. It may be nil.
//
// Strict expects the program to print the error Execute returns itself,
// with SilenceErrors and SilenceUsage set on root.
func Strict(root *cobra.Command, moved map[string]string) {
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	for _, sub := range root.Commands() {
		if sub.Name() == "help" {
			sub.Args = func(cmd *cobra.Command, args []string) error {
				return helpTopic(cmd, args, moved)
			}
		}
	}
	requireKnown(root, moved)
	usageArgs(root)
}

// requireKnown makes every command that holds subcommands and runs nothing
// print its help when invoked bare and refuse any argument. Cobra prints
// such a command's help and succeeds whatever its arguments.
func requireKnown(cmd *cobra.Command, moved map[string]string) {
	for _, sub := range cmd.Commands() {
		requireKnown(sub, moved)
	}
	if !cmd.HasSubCommands() || cmd.Runnable() {
		return
	}
	if cmd.SuggestionsMinimumDistance <= 0 {
		// Cobra's default, which it applies only on its own error path.
		cmd.SuggestionsMinimumDistance = 2
	}
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		return unknown(cmd, args, moved)
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}
}

// helpTopic is the argument check of the help command. The topic has to
// name a command, all of it: cobra's help prints the root's help for a
// topic it cannot find, and the help of the nearest command for one with
// words left over, and succeeds either way.
func helpTopic(cmd *cobra.Command, args []string, moved map[string]string) error {
	// Find reports a word the root has no subcommand for as an error, but
	// still returns the command it reached and the words it could not place.
	found, rest, _ := cmd.Root().Find(args)
	if len(rest) == 0 {
		return nil
	}
	return fmt.Errorf("unknown help topic %q: %w", strings.Join(args, " "), unknown(found, rest, moved))
}

// unknown refuses the arguments of a command that holds subcommands,
// suggesting the closest of them, or else the command that replaced the
// one asked for.
func unknown(cmd *cobra.Command, args []string, moved map[string]string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if s := cmd.SuggestionsFor(args[0]); len(s) > 0 {
		msg += fmt.Sprintf(" (did you mean %q?)", s[0])
	} else if to, ok := moved[below(cmd, args[0])]; ok {
		msg += fmt.Sprintf(" (it is now %q)", to)
	}
	return errors.New(msg)
}

// below returns the words of a command line that names word under cmd,
// without the program's name: "get node" for the word "node" under get.
func below(cmd *cobra.Command, word string) string {
	line := word
	for c := cmd; c.HasParent(); c = c.Parent() {
		line = c.Name() + " " + line
	}
	return line
}

// usageArgs makes every argument check in the tree return a usage error,
// and gives every runnable command one, that marks the errors of its
// required flags and flag groups as usage errors too. Cobra's own checks,
// such as cobra.ExactArgs, return plain errors, which name neither the
// arguments wanted nor the command, and cobra checks the flags after Args
// with plain errors of its own.
func usageArgs(cmd *cobra.Command) {
	if check := cmd.Args; check != nil || cmd.Runnable() {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := argsError(c, check, args); err != nil {
				return Usage(err)
			}
			return Usage(flagsError(c))
		}
	}
	for _, sub := range cmd.Commands() {
		usageArgs(sub)
	}
}

// argsError returns the error of check for args, with the usage line of a
// command that holds no subcommands after it. A nil check takes any
// arguments, as cobra's does for a command that is not the root.
func argsError(c *cobra.Command, check cobra.PositionalArgs, args []string) error {
	if check == nil {
		return nil
	}
	err := check(c, args)
	if err != nil && !c.HasSubCommands() && c.Name() != "help" {
		err = fmt.Errorf("%w (usage: %s)", err, strings.TrimSuffix(c.UseLine(), " [flags]"))
	}
	return err
}

// flagsError returns cobra's error for a required flag that was not given,
// or for the flags of a group that were not given together, or given
// together where they exclude each other. It checks only flags that were
// parsed: cobra parses them before it calls Args.
func flagsError(c *cobra.Command) error {
	if !c.Flags().Parsed() {
		return nil
	}
	if err := c.ValidateRequiredFlags(); err != nil {
		return err
	}
	return c.ValidateFlagGroups()
}
