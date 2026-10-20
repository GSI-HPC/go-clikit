// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree

import (
	"errors"
	"fmt"

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
