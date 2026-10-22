// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// usageError marks an error in the command line.
type usageError struct {
	err error
}

func (e *usageError) Error() string { return e.err.Error() }

func (e *usageError) Unwrap() error { return e.err }

// Usage marks err as an error in the command line: an unknown command or
// flag, a flag value or an argument that is not valid, or the wrong number
// of arguments, which the program refused before it acted on any. Its text
// is err's, and errors.Is and errors.As look through it. A nil err stays
// nil.
func Usage(err error) error {
	if err == nil {
		return nil
	}
	return &usageError{err: err}
}

// Usagef returns a usage error with a message formatted as by fmt.Errorf,
// whose %w verbs wrap their errors.
func Usagef(format string, args ...any) error {
	return Usage(fmt.Errorf(format, args...))
}

// IsUsage reports whether err is an error in the command line: one that
// Usage or Usagef made, an argument check of a tree Strict made strict
// among them, or one that pflag returned for a flag it could not parse,
// unknown, without its value, with a value it does not take or of a
// syntax it does not know. errors.As finds either anywhere in err's tree.
//
// pflag's errors count wherever cobra parsed the flags: cobra passes only
// those of the command that runs through its FlagErrorFunc, not those of
// the parents it traverses with TraverseChildren, and a command that sets
// DisableFlagParsing and parses its own flags returns pflag's errors as
// they are.
func IsUsage(err error) bool {
	return is[*usageError](err) ||
		is[*pflag.NotExistError](err) ||
		is[*pflag.ValueRequiredError](err) ||
		is[*pflag.InvalidValueError](err) ||
		is[*pflag.InvalidSyntaxError](err)
}

// is reports whether errors.As finds an error of type T in err's tree.
func is[T error](err error) bool {
	_, ok := errors.AsType[T](err)
	return ok
}

// InvalidValue returns the usage error for a value of the flag --name that
// the program refuses, in the one form FlagError gives pflag's: what the
// flag was given, quoted, and what it wants, such as "human or json":
//
//	invalid --output value "yaml": want human or json
//
// name is the flag's long name, without its dashes.
func InvalidValue(name, value, want string) error {
	return Usagef("invalid --%s value %q: want %s", name, value, want)
}

// flagValueError is pflag's error for a value it cannot parse, worded by
// InvalidValue.
type flagValueError struct {
	usage error
	err   *pflag.InvalidValueError
}

func (e *flagValueError) Error() string { return e.usage.Error() }

func (e *flagValueError) Unwrap() []error { return []error{e.usage, e.err} }

// flagWants says what a flag of each of pflag's types wants, for the values
// its parsers refuse with the text of Go's, such as strconv.ParseInt:
// parsing "x": invalid syntax.
var flagWants = map[string]string{
	"bool":     "true or false",
	"count":    "a whole number",
	"duration": "a duration with a unit, such as 90s, 5m or 1h30m",
	"float32":  "a number",
	"float64":  "a number",
	"int":      "a whole number",
	"int8":     "a whole number from -128 to 127",
	"int16":    "a whole number from -32768 to 32767",
	"int32":    "a whole number from -2147483648 to 2147483647",
	"int64":    "a whole number",
	"uint":     "a whole number, 0 or more",
	"uint8":    "a whole number from 0 to 255",
	"uint16":   "a whole number from 0 to 65535",
	"uint32":   "a whole number from 0 to 4294967295",
	"uint64":   "a whole number, 0 or more",
}

// FlagError is a cobra FlagErrorFunc, which a program sets on its root with
// SetFlagErrorFunc, that words a value pflag cannot parse as the flag's
// type as InvalidValue does, instead of with the text of Go's parser:
//
//	invalid --count value "x": want a whole number
//
// for a flag of a bool, a duration, a count, or a number of any of the
// sizes pflag has. Every other error is returned as it is: an unknown flag
// and a missing value say what they need already. Both are usage errors,
// and errors.As still finds pflag's InvalidValueError in the one worded.
//
// Cobra passes it the errors of the command that runs, and of no parent it
// traverses with TraverseChildren, as IsUsage says.
func FlagError(_ *cobra.Command, err error) error {
	invalid, ok := errors.AsType[*pflag.InvalidValueError](err)
	if !ok {
		return err
	}
	flag := invalid.GetFlag()
	want, ok := flagWants[flag.Value.Type()]
	if !ok {
		return err
	}
	return &flagValueError{usage: InvalidValue(flag.Name, invalid.GetValue(), want), err: invalid}
}
