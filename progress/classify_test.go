// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress"
)

// classified is an error that says its own class, as a pin mismatch or a
// refused account does.
type classified struct{ class progress.Class }

func (c classified) Error() string                 { return "classified" }
func (c classified) ProgressClass() progress.Class { return c.class }

// timeout is an error that reports a timeout the way net.Error does.
type timeout struct{}

func (timeout) Error() string { return "i/o timeout" }
func (timeout) Timeout() bool { return true }

// errUnreachable is an error a program's own rule, byCode below, says is
// the transport's, as a program may say of one of its exit codes.
var errUnreachable = errors.New("exe0001: no route to host")

// byCode is a program's fallback: the transport's for errUnreachable,
// the target's for the rest.
func byCode(err error) progress.Class {
	if errors.Is(err, errUnreachable) {
		return progress.ClassTransport
	}
	return progress.ClassTarget
}

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want progress.Class
	}{
		{"no error", nil, progress.ClassNone},
		{"an error that says nothing is the target's", errors.New("command exited 1"), progress.ClassTarget},
		{"the fallback's class", fmt.Errorf("ssh: %w", errUnreachable), progress.ClassTransport},
		{"an interrupt however wrapped", fmt.Errorf("%w: %w", errUnreachable, context.Canceled), progress.ClassCanceled},
		{"a deadline", fmt.Errorf("exe0001: %w", context.DeadlineExceeded), progress.ClassTimeout},
		{"a network timeout", &url.Error{Op: "Get", URL: "https://bmc", Err: timeout{}}, progress.ClassTimeout},
		{"a dial that timed out", fmt.Errorf("%w: %w", errUnreachable, &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}), progress.ClassTimeout},
		{"a network error that is no timeout", fmt.Errorf("%w: %w", errUnreachable, &net.OpError{Op: "dial", Err: errors.New("connection refused")}), progress.ClassTransport},
		{"an error that says its class", fmt.Errorf("bmc: %w", classified{progress.ClassPin}), progress.ClassPin},
		{"its class before the fallback", fmt.Errorf("%w: %w", errUnreachable, classified{progress.ClassAuth}), progress.ClassAuth},
		{"its class before an interrupt", fmt.Errorf("%w: %w", classified{progress.ClassTimeout}, context.Canceled), progress.ClassTimeout},
		{"a class of none leaves the rules", fmt.Errorf("%w: %w", errUnreachable, classified{progress.ClassNone}), progress.ClassTransport},
	}
	for _, tc := range tests {
		if got := progress.Classify(tc.err, byCode); got != tc.want {
			t.Errorf("%s: Classify(%v) = %s, want %s", tc.name, tc.err, got, tc.want)
		}
	}
}

// Without a fallback, an error the first rules do not class is the
// target's, and the fallback is never asked about a nil error.
func TestClassifyWithoutAFallback(t *testing.T) {
	t.Parallel()
	if got := progress.Classify(errUnreachable, nil); got != progress.ClassTarget {
		t.Errorf("Classify(%v, nil) = %s, want %s", errUnreachable, got, progress.ClassTarget)
	}
	if got := progress.Classify(context.Canceled, nil); got != progress.ClassCanceled {
		t.Errorf("Classify(%v, nil) = %s, want %s", context.Canceled, got, progress.ClassCanceled)
	}
	asked := func(error) progress.Class {
		t.Error("the fallback was asked about a nil error")
		return progress.ClassTarget
	}
	if got := progress.Classify(nil, asked); got != progress.ClassNone {
		t.Errorf("Classify(nil) = %s, want %s", got, progress.ClassNone)
	}
}

// A fallback that answers ClassNone for an error leaves it the target's,
// for an error is a failure, and ClassNone says that nothing failed.
func TestClassifyFallbackOfNone(t *testing.T) {
	t.Parallel()
	none := func(error) progress.Class { return progress.ClassNone }
	if got := progress.Classify(errUnreachable, none); got != progress.ClassTarget {
		t.Errorf("Classify(%v) = %s, want %s", errUnreachable, got, progress.ClassTarget)
	}
}
