// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package sse_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"

	"github.com/GSI-HPC/go-clikit/localweb"
	"github.com/GSI-HPC/go-clikit/localweb/localwebtest"
	"github.com/GSI-HPC/go-clikit/localweb/sse"
)

// Store is what a program shows, with the topics it changes in.
type Store struct {
	mu       sync.Mutex
	runs     []string
	notifier sse.Notifier[string]
}

// AddRun changes the topic "runs".
func (s *Store) AddRun(name string) {
	s.mu.Lock()
	s.runs = append(s.runs, name)
	s.mu.Unlock()
	s.notifier.Notify("runs")
}

// fragment renders the fragment of topic.
func (s *Store) fragment(topic string) sse.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sse.Event{Name: topic, Data: fmt.Sprintf("<ul>%d runs</ul>", len(s.runs))}
}

// A page names its fragments, and its stream sends them first, then each
// one whose topic changed.
func ExampleServe() {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	store := &Store{}
	stream := func(w http.ResponseWriter, r *http.Request) {
		_ = sse.Serve(w, r, &store.notifier, func(changed []string) ([]sse.Event, error) {
			if changed == nil {
				changed = []string{"runs"}
			}
			var es []sse.Event
			for _, t := range changed {
				es = append(es, store.fragment(t))
			}
			return es, nil
		}, sse.Options{Done: ctx.Done()})
	}
	srv := httptest.NewServer(localweb.SameOrigin(localweb.Origin{})(http.HandlerFunc(stream)))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	events := localwebtest.NewReader(resp.Body)
	e, _ := events.Next()
	fmt.Println(e.Name, e.Data)
	store.AddRun("create")
	e, _ = events.Next()
	fmt.Println(e.Name, e.Data)
	// Output:
	// runs <ul>0 runs</ul>
	// runs <ul>1 runs</ul>
}

// A subscriber learns each topic that changed once, in the order of its
// first change.
func ExampleSubscription_Take() {
	var n sse.Notifier[string]
	sub := n.Subscribe()
	defer sub.Close()
	n.Notify("runs", "run/1")
	n.Notify("tasks", "runs")
	<-sub.Changed()
	fmt.Println(sub.Take(), slices.Equal(sub.Take(), nil))
	// Output: [runs run/1 tasks] true
}

// An event is a data line for each line of its HTML.
func ExampleFormat() {
	fmt.Printf("%q\n", sse.Format(sse.Event{Name: "runs", Data: "<ul>\n<li>create</li>\n</ul>"}))
	// Output: "event: runs\ndata: <ul>\ndata: <li>create</li>\ndata: </ul>\n\n"
}
