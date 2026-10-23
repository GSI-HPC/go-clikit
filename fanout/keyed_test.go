// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GSI-HPC/go-clikit/fanout"
	"github.com/GSI-HPC/go-clikit/fanout/fanouttest"
	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

// Each key is bounded on its own: work on one key waits for a place there,
// and work on another is not held up by it. Each holder waits a second of
// the fake clock of testing/synctest, which passes only once every holder
// that can have started has, so the most under way on a key is its bound,
// exactly, and both keys run that many together. A limit below one is one.
func TestKeyedBoundsEachKeyOnItsOwn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"the zero value", 0, 1},
		{"a negative limit", -3, 1},
		{"one", 1, 1},
		{"three", 3, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				keys := &fanout.Keyed{Limit: tc.limit}
				on := map[string]*fanouttest.InFlight{"gw1.example.org": {}, "gw2.example.org": {}}
				var both fanouttest.InFlight
				var wg sync.WaitGroup
				for key, calls := range on {
					for range 10 {
						wg.Go(func() {
							release, err := keys.Acquire(context.Background(), key)
							if err != nil {
								t.Error(err)
								return
							}
							defer release()
							defer calls.Enter()()
							defer both.Enter()()
							time.Sleep(time.Second)
						})
					}
				}
				wg.Wait()
				for key, calls := range on {
					if got := calls.Peak(); got != tc.want {
						t.Errorf("%s: %d were under way at once, want %d", key, got, tc.want)
					}
				}
				if got := both.Peak(); got != 2*tc.want {
					t.Errorf("%d were under way on both keys at once, want %d: one key held up the other", got, 2*tc.want)
				}
			})
		})
	}
}

// Work that goes through two keys takes a place on both, in the same
// order whatever order it names them in, so two holders of one place each
// never wait for each other's. Callers that did would be asleep for ever,
// which the fake clock of testing/synctest turns into the minute that ends
// the test.
func TestKeyedTakesItsPlacesInOneOrder(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		keys := &fanout.Keyed{Limit: 1}
		done := make(chan struct{})
		go func() {
			defer close(done)
			var wg sync.WaitGroup
			for i := range 50 {
				wg.Go(func() {
					pair := []string{"a.example.org", "b.example.org"}
					if i%2 == 1 {
						pair[0], pair[1] = pair[1], pair[0]
					}
					release, err := keys.Acquire(context.Background(), pair...)
					if err != nil {
						t.Error(err)
						return
					}
					time.Sleep(time.Millisecond)
					release()
				})
			}
			wg.Wait()
		}()
		select {
		case <-done:
		case <-time.After(time.Minute):
			t.Fatal("work through the same two keys waited for itself")
		}
	})
}

// Once the context ends, a holder waiting for a place stops waiting and
// gives back the places it took, and nothing is taken any more, though a
// place is free.
func TestKeyedGivesBackWhatItTookWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		keys := &fanout.Keyed{Limit: 1}
		holding, err := keys.Acquire(context.Background(), "b.example.org")
		if err != nil {
			t.Fatal(err)
		}
		defer holding()
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)
		if release, err := keys.Acquire(ctx, "a.example.org", "b.example.org"); !errors.Is(err, context.Canceled) || release != nil {
			t.Fatalf("Acquire = %p, %v, want no release and the context's error", release, err)
		}
		quick, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		release, err := keys.Acquire(quick, "a.example.org")
		if err != nil {
			t.Fatalf("the place on a.example.org was not given back: %v", err)
		}
		release()
		if _, err := keys.Acquire(ctx, "c.example.org"); !errors.Is(err, context.Canceled) {
			t.Fatalf("Acquire after the context ended = %v, want the context's error", err)
		}
		release, err = keys.Acquire(quick, "c.example.org")
		if err != nil {
			t.Fatalf("a place on c.example.org was taken once the context had ended: %v", err)
		}
		release()
	})
}

// Once the context has ended, Acquire takes nothing and returns the
// context's error, with no release, whatever keys it is given: also none,
// and an empty one, which take no place, so that work that goes to no key
// does not start either.
func TestKeyedRefusesAnEndedContextForAnyKeys(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		keys []string
	}{
		{"no keys", nil},
		{"an empty key", []string{""}},
		{"a free key", []string{"gw.example.org"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			keys := &fanout.Keyed{Limit: 1}
			if release, err := keys.Acquire(ctx, tc.keys...); !errors.Is(err, context.Canceled) || release != nil {
				t.Fatalf("Acquire(%q) = %p, %v, want no release and the context's error", tc.keys, release, err)
			}
			// Nothing was taken: the key is free.
			quick, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			release, err := keys.Acquire(quick, "gw.example.org")
			if err != nil {
				t.Fatalf("a place on gw.example.org was taken: %v", err)
			}
			release()
		})
	}
}

// A key with no name takes no place, and nor does one given twice: work
// that goes to no key of its own is bounded by the pool alone, and a key
// listed again is no reason to wait for oneself.
func TestKeyedTakesNoPlaceForNoKeyAndNoSecondForOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []string
	}{
		{"no keys", nil},
		{"an empty key", []string{""}},
		{"an empty key beside another", []string{"", "gw.example.org"}},
		{"the same key twice", []string{"gw.example.org", "gw.example.org"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				keys := &fanout.Keyed{Limit: 1}
				// A second place on a key that is full would wait for ever,
				// so the context ends it.
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				release, err := keys.Acquire(ctx, tc.keys...)
				if err != nil {
					t.Fatalf("Acquire(%q) = %v", tc.keys, err)
				}
				release()
			})
		})
	}

	t.Run("the empty key holds nothing up", func(t *testing.T) {
		t.Parallel()
		keys := &fanout.Keyed{Limit: 1}
		release, err := keys.Acquire(context.Background(), "", "gw.example.org")
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		second, err := keys.Acquire(context.Background(), "")
		if err != nil {
			t.Fatalf("a second work with no key had to wait: %v", err)
		}
		second()
	})
}

// A release that is called again gives nothing back a second time, so a
// place another holder has taken since stays that holder's.
func TestKeyedReleasesOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		keys := &fanout.Keyed{Limit: 1}
		first, err := keys.Acquire(context.Background(), "gw.example.org")
		if err != nil {
			t.Fatal(err)
		}
		first()
		second, err := keys.Acquire(context.Background(), "gw.example.org")
		if err != nil {
			t.Fatal(err)
		}
		defer second()
		first()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := keys.Acquire(ctx, "gw.example.org"); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Acquire = %v, want it to wait for the place of the second holder", err)
		}
	})
}

// A Keyed shared by two Each loops bounds them together, with the work of
// each taking its place inside the function Each calls.
func TestKeyedBoundsPoolsSideBySide(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		keys := &fanout.Keyed{Limit: 2}
		var calls fanouttest.InFlight
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() {
				fanout.Each(context.Background(), 6, 6, func(int) {
					release, err := keys.Acquire(context.Background(), "gw.example.org")
					if err != nil {
						t.Error(err)
						return
					}
					defer release()
					defer calls.Enter()()
					time.Sleep(time.Second)
				})
			})
		}
		wg.Wait()
		if got := calls.Peak(); got != 2 {
			t.Errorf("%d were under way at once through one key, want 2", got)
		}
	})
}

// An item waits for what Acquire takes with its target queued, since its
// work has not started; one Acquire refuses ends as never started, with
// the reason, and the rest go on. A pool of eight through one key keeps to
// that key's bound.
func TestKeyedAsWhatMapAcquires(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, w := progresstest.Watch(context.Background(), t)
		keys := &fanout.Keyed{Limit: 4}
		var calls fanouttest.InFlight
		refused := errors.New("no place for exe2")
		outcomes, _ := fanout.Map(ctx, nodes(8), fanout.MapOptions[string]{
			Step:  "check",
			Limit: 8,
			Acquire: func(ctx context.Context, node string) (func(), error) {
				for _, e := range w.Events() {
					if e.Type == progress.TypeRun && e.Name == node {
						t.Errorf("%s runs before it has its place", node)
					}
				}
				if node == "exe2" {
					return nil, refused
				}
				return keys.Acquire(ctx, "gw.example.org")
			},
		}, func(context.Context, string) (struct{}, error) {
			defer calls.Enter()()
			time.Sleep(time.Second)
			return struct{}{}, nil
		})
		if o := outcomes[1]; o.Started || !errors.Is(o.Err, refused) {
			t.Errorf("the outcome of exe2 is %+v, want it never started, with the refusal", o)
		}
		if got := calls.Peak(); got != 4 {
			t.Errorf("%d items were under way at once through one key, want 4", got)
		}
		want := `step check total=8 limit=8 [fold]: failed (target): 1 of 8 failed: exe2
  target exe2: failed (target): no place for {}
  target exe[1,3-8]: ok
`
		if got := w.Finish(); got != want {
			t.Errorf("tree:\n%s\nwant:\n%s", got, want)
		}
	})
}
