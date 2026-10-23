// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"context"
	"slices"
	"sync"
)

// Keyed bounds the work under way on each key, such as a host: at most
// Limit at a time on any one key, however many keys a pool works on at
// once. The zero value bounds each key to one. A Keyed is safe for
// concurrent use; one Keyed shared by pools side by side bounds them
// together. It must not be copied after first use.
//
// A Keyed is no pool: it bounds what a pool's function takes, and the pool
// still bounds how many items run. With Each, the function for an index
// calls Acquire around its work on the key and releases when that is done,
// holding its place in the pool while it waits. With Map, MapOptions.Acquire
// is the place to call it: the item waits queued, and Map calls the release
// once the work has returned.
//
// The places of a key are kept for as long as the Keyed is, so a Keyed
// belongs to one command, not to a program that runs for ever over keys
// that are never the same twice.
type Keyed struct {
	// Limit is how many may be under way on one key; below one is one. It
	// is read when a key is first used, so it is set before the first
	// Acquire.
	Limit int

	mu     sync.Mutex
	places map[string]chan struct{}
}

// Acquire takes a place on every key it is given, waiting while one of
// them is full, and returns the function that gives them back, which may
// be called more than once and gives them back once. The places are taken
// in the order of the keys' names, whatever order they are given in, so two
// callers that need the same two keys cannot each hold one and wait for the
// other; a key given twice takes no second place, and an empty key takes
// none, so that work that goes to no key of its own is bounded by the pool
// alone. Once ctx has ended nothing is taken: what was taken is given back,
// and the context's error returned, with no release. That holds for no key
// and for an empty one too, so that work that goes to no key does not start
// once ctx has ended.
func (k *Keyed) Acquire(ctx context.Context, keys ...string) (release func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	names := slices.Compact(slices.Sorted(slices.Values(keys)))
	var held []chan struct{}
	var once sync.Once
	release = func() {
		once.Do(func() {
			for _, place := range held {
				<-place
			}
		})
	}
	for _, name := range names {
		if name == "" {
			continue
		}
		place := k.place(name)
		select {
		case place <- struct{}{}:
			held = append(held, place)
		case <-ctx.Done():
		}
		// When a place and the cancellation are both ready, select picks
		// either, so the context is asked again.
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
	}
	return release, nil
}

// place returns the places of one key.
func (k *Keyed) place(key string) chan struct{} {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.places == nil {
		k.places = map[string]chan struct{}{}
	}
	place, ok := k.places[key]
	if !ok {
		place = make(chan struct{}, max(1, k.Limit))
		k.places[key] = place
	}
	return place
}
