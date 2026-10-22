// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/cobratree"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagTree is a command with a flag of each type FlagError words, and one
// of a type it does not.
func flagTree() *cobra.Command {
	root := &cobra.Command{Use: "prog", RunE: run, SilenceErrors: true, SilenceUsage: true}
	f := root.Flags()
	f.Bool("force", false, "")
	f.CountP("verbose", "v", "")
	f.Duration("wait", 0, "")
	f.Float32("f32", 0, "")
	f.Float64("f64", 0, "")
	f.IntP("count", "c", 0, "")
	f.Int8("i8", 0, "")
	f.Int16("i16", 0, "")
	f.Int32("i32", 0, "")
	f.Int64("i64", 0, "")
	f.Uint("u", 0, "")
	f.Uint8("u8", 0, "")
	f.Uint16("u16", 0, "")
	f.Uint32("u32", 0, "")
	f.Uint64("u64", 0, "")
	f.IP("address", nil, "")
	root.SetFlagErrorFunc(cobratree.FlagError)
	return root
}

func TestFlagError(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--force=maybe"}, `invalid --force value "maybe": want true or false`},
		{[]string{"-v=x"}, `invalid --verbose value "x": want a whole number`},
		{[]string{"--wait", "5"}, `invalid --wait value "5": want a duration with a unit, such as 90s, 5m or 1h30m`},
		{[]string{"--f32", "x"}, `invalid --f32 value "x": want a number`},
		{[]string{"--f64", "1,5"}, `invalid --f64 value "1,5": want a number`},
		{[]string{"-c", "many"}, `invalid --count value "many": want a whole number`},
		{[]string{"--i8", "128"}, `invalid --i8 value "128": want a whole number from -128 to 127`},
		{[]string{"--i16", "x"}, `invalid --i16 value "x": want a whole number from -32768 to 32767`},
		{[]string{"--i32", "x"}, `invalid --i32 value "x": want a whole number from -2147483648 to 2147483647`},
		{[]string{"--i64", "1.5"}, `invalid --i64 value "1.5": want a whole number`},
		{[]string{"--u", "-1"}, `invalid --u value "-1": want a whole number, 0 or more`},
		{[]string{"--u8", "256"}, `invalid --u8 value "256": want a whole number from 0 to 255`},
		{[]string{"--u16", "x"}, `invalid --u16 value "x": want a whole number from 0 to 65535`},
		{[]string{"--u32", "x"}, `invalid --u32 value "x": want a whole number from 0 to 4294967295`},
		{[]string{"--u64", "x"}, `invalid --u64 value "x": want a whole number, 0 or more`},
		{[]string{"--count", "\x1b[2J"}, `invalid --count value "\x1b[2J": want a whole number`},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			root := flagTree()
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
			if !cobratree.IsUsage(err) {
				t.Errorf("%v is no usage error", err)
			}
			if _, ok := errors.AsType[*pflag.InvalidValueError](err); !ok {
				t.Errorf("errors.As finds no InvalidValueError in %v", err)
			}
		})
	}
}

// What FlagError does not word keeps pflag's text, and is a usage error
// still.
func TestFlagErrorKeepsTheRest(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--address", "x"}, `invalid argument "x" for "--address" flag`},
		{[]string{"--bogus"}, "unknown flag: --bogus"},
		{[]string{"--count"}, "flag needs an argument: --count"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			root := flagTree()
			root.SetArgs(tc.args)
			err := root.Execute()
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) || !cobratree.IsUsage(err) {
				t.Errorf("error %v, want the usage error %q", err, tc.want)
			}
		})
	}
}
