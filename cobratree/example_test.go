// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree_test

import (
	"cmp"
	"errors"
	"fmt"
	"os"

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

// A command gives an error the exit status that says what failed, where it
// knows; main maps the rest.
func ExampleStatus() {
	const exitTransport = 3
	errs := []error{
		nil,
		fmt.Errorf("n1: %w", cobratree.Coded(exitTransport, errors.New("connection refused"))),
		cobratree.Usagef("--count must be at least 1"),
		errors.New("n2 did not drain"),
	}
	for _, err := range errs {
		status := cobratree.Status(err, 1)
		if cobratree.IsUsage(err) {
			status = 2
		}
		fmt.Println(status, err)
	}
	// Output:
	// 0 <nil>
	// 3 n1: connection refused
	// 2 --count must be at least 1
	// 1 n2 did not drain
}

// A program shows the concepts of its manual with help, beside its
// commands, and lists them under a word of its own.
func ExampleHelpTopics() {
	root := &cobra.Command{Use: "prog", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "version", RunE: func(*cobra.Command, []string) error { return nil }})
	cobratree.Strict(root, nil)
	topics := map[string]string{
		"realm":  "A realm keeps the clusters of one user apart from another's.\n",
		"topics": "realm   what a realm is\n",
	}
	help := cobratree.HelpTopics(root, func(name string) (string, bool) {
		text, ok := topics[name]
		return text, ok
	})
	help.Short = "Help about any command or topic"

	root.SetOut(os.Stdout)
	for _, args := range [][]string{{"help", "topics"}, {"help", "realm"}, {"help", "relam"}} {
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			fmt.Println(err)
		}
	}
	// Output:
	// realm   what a realm is
	// A realm keeps the clusters of one user apart from another's.
	// unknown help topic "relam": unknown command "relam" for "prog"
}

// A program whose release builds inject their version and commit with
// -ldflags falls back to what the toolchain recorded, and words a build
// that is no release as it likes.
func ExampleBuildInfo_Resolve() {
	var version, commit string // set with -ldflags "-X main.version=v1.2.3 -X main.commit=…"
	bi := cobratree.ReadBuildInfo().Resolve(version, commit)
	line := "prog " + cmp.Or(bi.Version, "dev")
	if rev := bi.Short(7); rev != "" {
		line += " (" + rev + ")"
	}
	fmt.Println(line)
}

// A test applies the rules of Resolve to the build info of a checkout it
// builds: a release build's version wins, and a commit that is not the
// checkout's drops what was recorded about the checkout.
func ExampleBuildInfo_Resolve_checkout() {
	checkout := cobratree.BuildInfo{
		Commit: "ebed4872343d0123456789abcdef0123456789ab",
		Time:   "2026-09-23T18:09:18Z",
		Dirty:  true,
	}
	fmt.Printf("%+v\n", checkout.Resolve("v1.2.3", "ebed487"))
	fmt.Printf("%+v\n", checkout.Resolve("", "0123456"))
	// Output:
	// {Version:v1.2.3 Commit:ebed487 Time:2026-09-23T18:09:18Z Dirty:true}
	// {Version: Commit:0123456 Time: Dirty:false}
}

func ExampleBuildInfo_Short() {
	bi := cobratree.BuildInfo{Commit: "ebed4872343d0123456789abcdef0123456789ab", Dirty: true}
	fmt.Println(bi.Short(7))
	fmt.Println(bi.Short(12))
	// Output:
	// ebed487-dirty
	// ebed4872343d-dirty
}
