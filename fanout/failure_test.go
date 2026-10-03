// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/progress"
)

// Failure counts and names the items that failed, with the noun given, as
// a node set when every name is one host name, and as the list it was
// given otherwise; it keeps the errors underneath, and its class is the
// targets' or, when the interrupt ended them, canceled.
func TestFailure(t *testing.T) {
	t.Parallel()

	refused := errors.New("exe2: connection refused")
	for _, tc := range []struct {
		name     string
		noun     string
		names    []string
		errs     []error
		canceled bool
		want     string
		class    progress.Class
	}{
		{"none failed", "hosts", nil, nil, false, "", progress.ClassNone},
		{"hosts", "hosts", []string{"exe3", "exe1", "exe2"}, []error{nil, nil, refused}, false,
			"3 of 5 hosts failed: exe[1-3]", progress.ClassTarget},
		{"no noun", "", []string{"exe2"}, []error{refused}, false, "1 of 5 failed: exe2", progress.ClassTarget},
		{"names that are no host names", "volumes", []string{"config volume", "exe1"}, []error{refused, nil}, false,
			"2 of 5 volumes failed: config volume,exe1", progress.ClassTarget},
		{"a name no node set holds", "", []string{"exe1", "exe99999999999999999999999"}, []error{refused, nil}, false,
			"2 of 5 failed: exe1,exe99999999999999999999999", progress.ClassTarget},
		// A node set holds a name once, so the list would name fewer
		// than the count.
		{"names that repeat", "hosts", []string{"exe1", "exe1", "exe2"}, []error{refused, nil, nil}, false,
			"3 of 5 hosts failed: exe1,exe1,exe2", progress.ClassTarget},
		{"a name that is a group", "", []string{"@compute"}, []error{refused}, false, "1 of 5 failed: @compute", progress.ClassTarget},
		{"canceled", "hosts", []string{"exe1", "exe2"}, []error{context.Canceled, refused}, true,
			"2 of 5 hosts failed: exe[1-2]", progress.ClassCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := fanout.Failure(tc.noun, summary(5, tc.names, tc.errs, tc.canceled))
			if tc.want == "" {
				if err != nil {
					t.Errorf("Failure = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Fatalf("Failure = %v, want %q", err, tc.want)
			}
			if got := progress.Classify(err, nil); got != tc.class {
				t.Errorf("class %s, want %s", got, tc.class)
			}
			for _, e := range tc.errs {
				if e != nil && !errors.Is(err, e) {
					t.Errorf("%v is not kept underneath", e)
				}
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
		f := fanout.Failed{Name: name}
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

// Failure names the items of a wide fan-out that an interrupt ended, all
// of them failed, in time linear in their number: a command stopped with
// Ctrl-C over a large cluster exits at once rather than seconds later.
func TestFailureOfManyItemsIsQuick(t *testing.T) {
	t.Parallel()
	names := manyNames(16000)
	start := time.Now()
	err := fanout.Failure("hosts", summary(len(names), names, nil, true))
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("Failure of %d names took %s", len(names), took)
	}
	if want := "16000 of 16000 hosts failed: exe[00000-15999]"; err.Error() != want {
		t.Errorf("Failure = %q, want %q", err, want)
	}
}

func BenchmarkFailure(b *testing.B) {
	for _, n := range []int{1000, 16000} {
		s := summary(n, manyNames(n), nil, false)
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			for b.Loop() {
				_ = fanout.Failure("hosts", s)
			}
		})
	}
}
