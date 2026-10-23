// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cobratree

import "errors"

// codedError gives an error the exit status a program exits with for it.
type codedError struct {
	status int
	err    error
}

func (e *codedError) Error() string { return e.err.Error() }

func (e *codedError) Unwrap() error { return e.err }

// Coded gives err the exit status status, which Status returns for it and
// for any error that wraps it. Its text is err's, and errors.Is and
// errors.As look through it, IsUsage among them. An error that carries a
// status already keeps it, since the code nearer its cause gave it; a nil
// err stays nil.
//
// A status of 0 or less is no status: Coded returns err as it is, so that
// Status gives the default for it, and a command that failed does not exit
// with success (decision 39).
//
// The package names no status: a program gives its own, such as 3 for a
// host it could not reach, where the cause is known, and maps the errors
// that carry none at the top, in main.
func Coded(status int, err error) error {
	if err == nil || status <= 0 {
		return err
	}
	if _, ok := errors.AsType[*codedError](err); ok {
		return err
	}
	return &codedError{status: status, err: err}
}

// Status returns the exit status Coded gave err, the first errors.As finds
// in its tree; def for an error that carries none, and 0 for nil.
func Status(err error, def int) int {
	if err == nil {
		return 0
	}
	if coded, ok := errors.AsType[*codedError](err); ok {
		return coded.status
	}
	return def
}
