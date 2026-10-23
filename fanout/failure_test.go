// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
)

// hosts counts hosts, as a program's noun does.
func hosts(n int) string {
	if n == 1 {
		return "1 host"
	}
	return fmt.Sprintf("%d hosts", n)
}

// A FailureError counts and names the items that failed, with the noun
// given, as a node set when every name is one host name, and as the list it
// was given otherwise; it keeps the errors underneath, and its class is the
// targets' or, when the interrupt ended them, canceled.
func TestFailureError(t *testing.T) {
	t.Parallel()

	refused := errors.New("exe2: connection refused")
	volumes := func(n int) string { return fmt.Sprintf("%d volumes", n) }
	for _, tc := range []struct {
		name     string
		noun     func(int) string
		names    []string
		errs     []error
		canceled bool
		want     string
		class    progress.Class
	}{
		{"hosts", hosts, []string{"exe3", "exe1", "exe2"}, []error{nil, nil, refused}, false,
			"3 of 5 hosts failed: exe[1-3]", progress.ClassTarget},
		{"no noun", nil, []string{"exe2"}, []error{refused}, false, "1 of 5 failed: exe2", progress.ClassTarget},
		{"names that are no host names", volumes, []string{"config volume", "exe1"}, []error{refused, nil}, false,
			"2 of 5 volumes failed: config volume,exe1", progress.ClassTarget},
		{"a name no node set holds", nil, []string{"exe1", "exe99999999999999999999999"}, []error{refused, nil}, false,
			"2 of 5 failed: exe1,exe99999999999999999999999", progress.ClassTarget},
		// A node set holds a name once, so the list would name fewer
		// than the count.
		{"names that repeat", hosts, []string{"exe1", "exe1", "exe2"}, []error{refused, nil, nil}, false,
			"3 of 5 hosts failed: exe1,exe1,exe2", progress.ClassTarget},
		{"a name that is a group", nil, []string{"@compute"}, []error{refused}, false, "1 of 5 failed: @compute", progress.ClassTarget},
		{"canceled", hosts, []string{"exe1", "exe2"}, []error{context.Canceled, refused}, true,
			"2 of 5 hosts failed: exe[1-2]", progress.ClassCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := summary(5, tc.names, tc.errs, tc.canceled)
			var err error = &fanout.FailureError{Summary: s, Noun: tc.noun}
			if err.Error() != tc.want {
				t.Fatalf("FailureError = %q, want %q", err, tc.want)
			}
			if got := progress.Classify(err, nil); got != tc.class {
				t.Errorf("class %s, want %s", got, tc.class)
			}
			var kept []error
			for _, e := range tc.errs {
				if e != nil {
					kept = append(kept, e)
					if !errors.Is(err, e) {
						t.Errorf("%v is not kept underneath", e)
					}
				}
			}
			if got := err.(interface{ Unwrap() []error }).Unwrap(); !slices.Equal(got, kept) {
				t.Errorf("Unwrap = %v, want %v", got, kept)
			}
			var fe *fanout.FailureError
			if wrapped := fmt.Errorf("exit 2: %w", err); !errors.As(wrapped, &fe) || fe.Summary.Total != 5 ||
				len(fe.Summary.Failed) != len(tc.names) || fe.Summary.Canceled != tc.canceled {
				t.Errorf("errors.As found %+v, want the Summary %+v", fe, s)
			}
		})
	}
}

// Failure, the deprecated constructor, makes a *FailureError with the noun
// it is given after the count, and nil when none failed.
func TestFailure(t *testing.T) {
	t.Parallel()

	refused := errors.New("exe2: connection refused")
	for _, tc := range []struct {
		name string
		noun string
		s    fanout.Summary
		want string
	}{
		{"none failed", "hosts", summary(5, nil, nil, false), ""},
		{"a noun", "hosts", summary(5, []string{"exe2"}, []error{refused}, false), "1 of 5 hosts failed: exe2"},
		{"no noun", "", summary(5, []string{"exe2"}, []error{refused}, true), "1 of 5 failed: exe2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := fanout.Failure(tc.noun, tc.s)
			if tc.want == "" {
				if err != nil {
					t.Errorf("Failure = %v, want nil", err)
				}
				return
			}
			var fe *fanout.FailureError
			if !errors.As(err, &fe) || err.Error() != tc.want || !errors.Is(err, refused) {
				t.Errorf("Failure = %#v (%v), want a *FailureError %q over %v", err, err, tc.want, refused)
			}
		})
	}
}

// summary makes the Summary of total items of which those named names
// failed, each with the error of the same place in errs, or none past its
// end.
func summary(total int, names []string, errs []error, canceled bool) fanout.Summary {
	s := fanout.Summary{Total: total, Canceled: canceled}
	for i, name := range names {
		f := fanout.FailedItem{Name: name}
		if i < len(errs) {
			f.Err = errs[i]
		}
		s.Failed = append(s.Failed, f)
	}
	return s
}

// manyNames returns n host names, exe00000 up.
func manyNames(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("exe%05d", i)
	}
	return names
}

// A FailureError names the items of a wide fan-out that an interrupt ended, all
// of them failed, in time linear in their number: a command stopped with
// Ctrl-C over a large cluster exits at once rather than seconds later.
func TestFailureErrorOfManyItemsIsQuick(t *testing.T) {
	t.Parallel()
	names := manyNames(16000)
	start := time.Now()
	err := &fanout.FailureError{Summary: summary(len(names), names, nil, true), Noun: hosts}
	text := err.Error()
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("the text of a FailureError of %d names took %s", len(names), took)
	}
	if want := "16000 of 16000 hosts failed: exe[00000-15999]"; text != want {
		t.Errorf("FailureError = %q, want %q", text, want)
	}
}

func BenchmarkFailureError(b *testing.B) {
	for _, n := range []int{1000, 16000} {
		err := &fanout.FailureError{Summary: summary(n, manyNames(n), nil, false), Noun: hosts}
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for b.Loop() {
				_ = err.Error()
			}
		})
	}
}
