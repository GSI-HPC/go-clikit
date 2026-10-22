// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree_test

import (
	"cmp"
	"errors"
	"fmt"

	"github.com/GSI-HPC/go-clikit/cobratree"
	"github.com/spf13/cobra"
)

// A program builds its tree, makes it strict, and exits with its own code
// for a usage error.
func ExampleStrict() {
	root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
	get := &cobra.Command{Use: "get", Short: "Show things"}
	get.AddCommand(&cobra.Command{
		Use:  "cluster [NAME]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(*cobra.Command, []string) error { return nil },
	})
	root.AddCommand(get)
	cobratree.Strict(root, map[string]string{"status": "get cluster"})

	for _, args := range [][]string{
		{"gte"},
		{"status"},
		{"help", "get", "clustr"},
		{"get", "cluster", "a", "b"},
		{"get", "cluster", "--bogus"},
	} {
		root.SetArgs(args)
		err := root.Execute()
		code := 1
		if cobratree.IsUsage(err) {
			code = 2
		}
		fmt.Printf("%d %v\n", code, err)
	}
	// Output:
	// 2 unknown command "gte" for "prog" (did you mean "get"?)
	// 2 unknown command "status" for "prog" (it is now "get cluster")
	// 2 unknown help topic "get clustr": unknown command "clustr" for "prog get" (did you mean "cluster"?)
	// 2 accepts at most 1 arg(s), received 2 (usage: prog get cluster [NAME])
	// 2 unknown flag: --bogus
}

// A command refuses what its flags cannot give it as a usage error.
func ExampleUsagef() {
	count := 0
	err := fmt.Errorf("power on: %w", cobratree.Usagef("--count must be at least 1, got %d", count))
	fmt.Println(cobratree.IsUsage(err), err)
	fmt.Println(cobratree.IsUsage(errors.New("node n1 did not answer")))
	// Output:
	// true power on: --count must be at least 1, got 0
	// false
}

// A program words the values its flags cannot parse, and those it refuses
// itself, in one form.
func ExampleFlagError() {
	root := &cobra.Command{
		Use:           "prog",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if output, _ := cmd.Flags().GetString("output"); output != "human" && output != "json" {
				return cobratree.InvalidValue("output", output, "human or json")
			}
			return nil
		},
	}
	root.Flags().IntP("count", "c", 1, "how many")
	root.Flags().StringP("output", "o", "human", "human or json")
	root.SetFlagErrorFunc(cobratree.FlagError)

	for _, args := range [][]string{{"-c", "many"}, {"-o", "yaml"}} {
		root.SetArgs(args)
		err := root.Execute()
		fmt.Println(cobratree.IsUsage(err), err)
	}
	// Output:
	// true invalid --count value "many": want a whole number
	// true invalid --output value "yaml": want human or json
}

// A program whose release builds inject their version with -ldflags falls
// back to what the toolchain recorded, and words a build that is no
// release as it likes.
func ExampleReadBuildInfo() {
	var version string // set with -ldflags "-X main.version=v1.2.3"
	bi := cobratree.ReadBuildInfo()
	line := "prog " + cmp.Or(version, bi.Version, "dev")
	if bi.Commit != "" {
		line += " (" + bi.Commit[:min(7, len(bi.Commit))]
		if bi.Dirty {
			line += "-dirty"
		}
		line += ")"
	}
	fmt.Println(line)
}
