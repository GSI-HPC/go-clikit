// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package cobratreetest runs a cobra command tree in a test, and checks the
// command lines a program's manual and help show against the tree.
//
// Run executes the tree with the words of a command line and returns what
// it wrote to standard output and to standard error apart, since a program
// writes its result to the one and its diagnostics to the other, and a test
// of either has to tell them apart.
//
// CheckCommandLines reads text as a shell would read a script, a manual's
// examples or a help text's, finds the command lines that run the program,
// and checks that each names a command the tree has and flags that command
// has, so that a command renamed or a flag dropped breaks the test of the
// manual that still shows it:
//
//	$ prog get cluster --realm dev
//	eval "$(prog env dev)"
//	prog power on n[1-4] \
//	    --count 2 2>/dev/null
package cobratreetest

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Run executes the tree under root with args, the words of a command line
// after the program's name, under t's context, and returns what the tree
// wrote to its standard output and to its standard error, and the error
// Execute returned. No args runs the program with none, not with the test
// binary's.
//
// It sets the root's output, error output and arguments, which every
// command of the tree inherits unless it set its own. The completion
// scripts go to the output the root had when Strict was called, not to
// stdout. Cobra keeps the values flags were given from one Execute to the
// next, so a test runs each command line on a tree built afresh.
func Run(t testing.TB, root *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	if args == nil {
		args = []string{}
	}
	root.SetArgs(args)
	err = root.ExecuteContext(t.Context())
	return out.String(), errOut.String(), err
}

// CheckCommandLines checks every command line in text that runs the
// program root is, by root's name, with ValidateCommandLine, reports each
// that fails through t.Errorf, with its line, and returns how many it
// checked, for a test to require as many as the manual holds.
//
// text is read as lines of a shell script: a line that ends with a
// backslash goes on in the next, a prompt of "$ " in front is left out, a
// word that starts with # begins a comment, and quotes and backslashes are
// taken out of the words as a shell takes them out. A command line runs the
// program when the program's name is its first word: at the start of a
// line, after ;, &&, ||, | or &, or inside $( ). It ends at the next of
// those, or at a redirection such as > out.json or 2>/dev/null.
func CheckCommandLines(t testing.TB, root *cobra.Command, text string) (checked int) {
	t.Helper()
	for _, line := range scriptLines(text) {
		for _, args := range commandLines(root.Name(), line.text) {
			checked++
			if err := ValidateCommandLine(root, args); err != nil {
				t.Errorf("line %d: %q: %v", line.n, line.text, err)
			}
		}
	}
	return checked
}

// placeholder matches a word that stands for the words a reader puts in
// its place, such as NAME, <name>, [flags], (one|two) or ..., and so names
// no command.
var placeholder = regexp.MustCompile(`^[A-Z<\[(.]`)

// ValidateCommandLine reports whether args, the words of a command line
// after the program's name, name a command of the tree under root and
// flags that command has, its own or inherited, and returns the error that
// says which does not. It runs nothing, and fails no test: CheckCommandLines
// does that.
//
// The words that name commands come first: a command that holds
// subcommands takes no other word, as Strict has it, unless it runs and
// its own check of its arguments takes the words that follow, as Execute
// would; a word that is a placeholder, such as NAME, <name>, [flags] or
// ..., ends them, as does a command that holds none. Every flag is looked
// up, after them too, and takes the next word as its value unless it is a
// bool, a count or written with =. What follows -- is the arguments' and
// is not looked at, but by the check of a command that holds subcommands.
func ValidateCommandLine(root *cobra.Command, args []string) error {
	cmd := root
	commands := true
	// group is a runnable command that holds subcommands, and word the
	// first of its arguments, which names none of them; positional holds
	// its arguments, for its own check.
	var group *cobra.Command
	var word string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		cmd.InitDefaultHelpFlag()
		cmd.InitDefaultVersionFlag()
		switch {
		case arg == "--":
			if group == nil {
				return nil
			}
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(arg, "--"):
			name, _, hasValue := strings.Cut(arg[2:], "=")
			flag := lookupFlag(cmd, func(fs *pflag.FlagSet) *pflag.Flag { return fs.Lookup(name) })
			if flag == nil {
				return fmt.Errorf("%s has no flag --%s", cmd.CommandPath(), name)
			}
			if !hasValue && takesValue(flag) {
				i++
			}
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			for j := 1; j < len(arg); j++ {
				short := arg[j : j+1]
				flag := lookupFlag(cmd, func(fs *pflag.FlagSet) *pflag.Flag { return fs.ShorthandLookup(short) })
				if flag == nil {
					return fmt.Errorf("%s has no flag -%s", cmd.CommandPath(), short)
				}
				if takesValue(flag) {
					// The rest of the word is the value, or the next word.
					if j == len(arg)-1 {
						i++
					}
					break
				}
			}
		case commands && placeholder.MatchString(arg):
			commands = false
		case commands:
			sub := subcommand(cmd, arg)
			switch {
			case sub != nil:
				cmd = sub
			case cmd.HasSubCommands() && cmd.Runnable():
				// The command's own check decides, once its arguments
				// are known.
				group, word, positional = cmd, arg, []string{arg}
				commands = false
			case cmd.HasSubCommands():
				return fmt.Errorf("%s has no command %s", cmd.CommandPath(), arg)
			default:
				commands = false
			}
		case group != nil:
			positional = append(positional, arg)
		}
	}
	if group != nil && !takesArgs(group, positional) {
		return fmt.Errorf("%s has no command %s", group.CommandPath(), word)
	}
	return nil
}

// takesArgs reports whether cmd, a runnable command that holds
// subcommands, takes args, as Execute decides: by its own check, or, with
// none, by cobra's, which refuses any argument of the root alone.
func takesArgs(cmd *cobra.Command, args []string) bool {
	if cmd.Args == nil && !cmd.HasParent() {
		return false
	}
	return cmd.ValidateArgs(args) == nil
}

// subcommand returns the subcommand of cmd that name names, or one of its
// aliases; nil for none.
func subcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name || sub.HasAlias(name) {
			return sub
		}
	}
	return nil
}

// lookupFlag finds a flag of cmd, its own or inherited, with find.
func lookupFlag(cmd *cobra.Command, find func(*pflag.FlagSet) *pflag.Flag) *pflag.Flag {
	if flag := find(cmd.Flags()); flag != nil {
		return flag
	}
	return find(cmd.InheritedFlags())
}

// takesValue reports whether flag takes the next word as its value when it
// is not written with =: every flag but a bool and one, such as a count,
// that has a value of its own when it is given none.
func takesValue(flag *pflag.Flag) bool {
	return flag.Value.Type() != "bool" && flag.NoOptDefVal == ""
}

// scriptLine is a line of a script, its continued lines joined, and the
// number of the line it starts on.
type scriptLine struct {
	n    int
	text string
}

// scriptLines joins the continued lines of text and takes the prompts off.
func scriptLines(text string) []scriptLine {
	var out []scriptLine
	var cur strings.Builder
	start := 0
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if cur.Len() == 0 {
			start = i + 1
			line = strings.TrimPrefix(line, "$ ")
		}
		if cont, ok := strings.CutSuffix(line, `\`); ok {
			cur.WriteString(strings.TrimRight(cont, " \t") + " ")
			continue
		}
		cur.WriteString(line)
		out = append(out, scriptLine{n: start, text: cur.String()})
		cur.Reset()
	}
	return out
}

// subshell matches $( ) around a command line that runs program, which
// commandLines replaces with the name.
func subshell(program string) *regexp.Regexp {
	return regexp.MustCompile(`\$\((` + regexp.QuoteMeta(program) + ` [^)]*)\)`)
}

// commandLines returns the words after program of each command line in
// line that runs it, those inside $( ) first.
func commandLines(program, line string) [][]string {
	var out [][]string
	inner := subshell(program)
	for _, m := range inner.FindAllStringSubmatch(line, -1) {
		out = append(out, commandLines(program, m[1])...)
	}
	line = inner.ReplaceAllString(line, "x")
	words := shellWords(line)
	start := true
	for i, word := range words {
		if ends(word) {
			start = true
			continue
		}
		if start && word == program {
			end := i + 1
			for end < len(words) && !ends(words[end]) {
				end++
			}
			out = append(out, words[i+1:end])
		}
		start = false
	}
	return out
}

// redirection matches a word that redirects a stream, such as >out or
// 2>/dev/null.
var redirection = regexp.MustCompile(`^[0-9]*[<>]`)

// ends reports whether word ends a command line: an operator between
// commands, or a redirection.
func ends(word string) bool {
	switch word {
	case ";", "&&", "||", "|", "&":
		return true
	}
	return redirection.MatchString(word)
}

// shellWords splits a line as a shell splits it into words, with the
// quotes and backslashes taken out, the operators between commands words
// of their own, and a comment cut off.
func shellWords(line string) []string {
	var words []string
	var cur strings.Builder
	in := false
	var quote rune
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case quote == '"':
			switch {
			case r == '"':
				quote = 0
			case r == '\\' && i+1 < len(rs):
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, in = r, true
		case r == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			in = true
		case r == '#' && !in:
			return words
		case r == ' ' || r == '\t' || r == ';' || r == '|' || r == '&':
			if in {
				words = append(words, cur.String())
				cur.Reset()
				in = false
			}
			if r == ' ' || r == '\t' {
				continue
			}
			// An operator is a word of its own, with no space around it
			// too: ;, |, ||, & and &&.
			op := string(r)
			if r != ';' && i+1 < len(rs) && rs[i+1] == r {
				i++
				op += op
			}
			words = append(words, op)
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if in {
		words = append(words, cur.String())
	}
	return words
}
