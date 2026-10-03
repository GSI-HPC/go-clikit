// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"context"
	"errors"
)

// Classify tells why work failed from its error. The first rule that
// applies wins:
//
//  1. the first error in the chain that is a Classifier, unless it
//     answers ClassNone;
//  2. context.Canceled is ClassCanceled;
//  3. context.DeadlineExceeded, or an error that reports Timeout, such as
//     a network timeout, is ClassTimeout;
//  4. what fallback says of the error, the program's own rule, such as
//     one that reads the exit code the error asks for, unless fallback
//     is nil or answers ClassNone;
//  5. ClassTarget.
//
// A nil error is ClassNone, and fallback is not asked.
//
// An error that sums up the failures of many targets is best a Classifier
// of its own, since the first of its targets' errors that says a class is
// no more the whole's than any other.
func Classify(err error, fallback func(error) Class) Class {
	if err == nil {
		return ClassNone
	}
	var c Classifier
	if errors.As(err, &c) {
		if class := c.ProgressClass(); class != ClassNone {
			return class
		}
	}
	if errors.Is(err, context.Canceled) {
		return ClassCanceled
	}
	var t interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &t) && t.Timeout() {
		return ClassTimeout
	}
	if fallback != nil {
		if class := fallback(err); class != ClassNone {
			return class
		}
	}
	return ClassTarget
}

// ErrSkipped says that work was left out on purpose, such as a call a dry
// run only records, or the work for a node a command no longer tries
// once it found the node unreachable: not a failure. Span.End ends a span
// whose error is ErrSkipped, as errors.Is tells, skipped, and the pools of
// fanout do not count such an item among those that failed. Test for it
// with errors.Is, since the error Skip returns, and one that wraps it,
// say why instead.
var ErrSkipped = errors.New("skipped")

// Skip returns an error that says work was left out on purpose, for the
// reason given: its text is reason, and errors.Is finds ErrSkipped in it.
// Span.End ends a span with it skipped, with reason as its Err, as
// Span.Skip does.
func Skip(reason string) error { return &skipped{reason: reason} }

// skipped is the error of work left out on purpose, as Skip makes it.
type skipped struct{ reason string }

func (s *skipped) Error() string { return s.reason }

// Is reports that the error is ErrSkipped.
func (s *skipped) Is(target error) bool { return target == ErrSkipped }
