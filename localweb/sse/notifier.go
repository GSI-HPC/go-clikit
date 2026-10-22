// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package sse

import "sync"

// Notifier tells its subscribers which topics changed: the parts of what
// a program shows that change as a whole, such as "runs" or the name of
// one run. Notify never blocks, and a subscriber that falls behind loses
// no topic: it learns the exact topics that changed since it last took
// them, however many changes it missed, so that a slow stream renders the
// fragments of those topics only.
//
// The zero Notifier has no subscribers and is ready to use. A Notifier
// must not be copied after first use.
type Notifier[T comparable] struct {
	mu   sync.Mutex
	subs map[*Subscription[T]]struct{}
}

// Subscribe returns a subscription to the topics that change from now on.
// The caller ends it with Close. A stream subscribes before it renders
// what it sends first, so that no change falls between the two.
func (n *Notifier[T]) Subscribe() *Subscription[T] {
	s := &Subscription[T]{n: n, wake: make(chan struct{}, 1), seen: map[T]struct{}{}}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.subs == nil {
		n.subs = map[*Subscription[T]]struct{}{}
	}
	n.subs[s] = struct{}{}
	return s
}

// Notify tells every subscriber that topics changed.
func (n *Notifier[T]) Notify(topics ...T) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for s := range n.subs {
		s.add(topics)
	}
}

// Subscription is one subscriber's view of a Notifier: the set of topics
// that changed since it last took them, and a channel that wakes it.
type Subscription[T comparable] struct {
	n *Notifier[T]
	// wake holds one value while pending is not empty.
	wake chan struct{}

	mu      sync.Mutex
	pending []T
	seen    map[T]struct{}
}

// Changed returns a channel that receives when topics have changed since
// the last Take.
func (s *Subscription[T]) Changed() <-chan struct{} { return s.wake }

// Take returns the topics that changed since the last Take, each once, in
// the order of their first change, and forgets them. After a receive from
// Changed it has the topics that woke the subscriber, or none when an
// earlier Take took them.
func (s *Subscription[T]) Take() []T {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.pending
	s.pending = nil
	clear(s.seen)
	return out
}

// Close ends the subscription: Notify reaches it no more.
func (s *Subscription[T]) Close() {
	s.n.mu.Lock()
	defer s.n.mu.Unlock()
	delete(s.n.subs, s)
}

// add records that topics changed and wakes the subscriber.
func (s *Subscription[T]) add(topics []T) {
	s.mu.Lock()
	for _, t := range topics {
		if _, ok := s.seen[t]; !ok {
			s.seen[t] = struct{}{}
			s.pending = append(s.pending, t)
		}
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default: // a wake-up is pending already
	}
}
