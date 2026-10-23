// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package sse_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GSI-HPC/go-clikit/localweb"
	"github.com/GSI-HPC/go-clikit/localweb/localwebtest"
	"github.com/GSI-HPC/go-clikit/localweb/sse"
)

// A subscriber learns each topic once, in the order of its first change,
// however many changes it missed, and one wake-up stands for them all.
func TestASubscriptionKeepsTheExactTopics(t *testing.T) {
	t.Parallel()

	var n sse.Notifier[string]
	n.Notify("before") // no subscriber yet
	s := n.Subscribe()
	defer s.Close()
	n.Notify("runs", "run/1")
	n.Notify("tasks", "runs")
	n.Notify("run/1")
	select {
	case <-s.Changed():
	default:
		t.Fatal("Changed did not receive after Notify")
	}
	select {
	case <-s.Changed():
		t.Fatal("Changed received twice for one burst")
	default:
	}
	if got, want := s.Take(), []string{"runs", "run/1", "tasks"}; !slices.Equal(got, want) {
		t.Errorf("Take = %q, want %q", got, want)
	}
	if got := s.Take(); got != nil {
		t.Errorf("a second Take = %q, want nothing", got)
	}
	n.Notify("runs")
	<-s.Changed()
	if got := s.Take(); !slices.Equal(got, []string{"runs"}) {
		t.Errorf("Take after the set was taken = %q, want [runs]", got)
	}
	s.Close()
	n.Notify("runs")
	select {
	case <-s.Changed():
		t.Error("a closed subscription was woken")
	default:
	}
}

// Notify reaches every subscriber, and never blocks on one that does not
// take its topics.
func TestNotifyReachesEverySubscriber(t *testing.T) {
	t.Parallel()

	var n sse.Notifier[int]
	a, b := n.Subscribe(), n.Subscribe()
	for i := range 1000 {
		n.Notify(i % 3)
	}
	for _, s := range []*sse.Subscription[int]{a, b} {
		<-s.Changed()
		if got := s.Take(); !slices.Equal(got, []int{0, 1, 2}) {
			t.Errorf("Take = %v, want [0 1 2]", got)
		}
	}
}

// An event is its name and a data line for each line of its HTML; a
// carriage return becomes a character reference, as it would end a line.
func TestFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		e    sse.Event
		want string
	}{
		{sse.Event{Name: "runs", Data: "<p>1</p>"}, "event: runs\ndata: <p>1</p>\n\n"},
		{sse.Event{Name: "runs", Data: "<p>\n1\r\n</p>"}, "event: runs\ndata: <p>\ndata: 1&#13;\ndata: </p>\n\n"},
		{sse.Event{Name: "empty"}, "event: empty\ndata: \n\n"},
	}
	for _, tt := range tests {
		if got := sse.Format(tt.e); got != tt.want {
			t.Errorf("Format(%+v) = %q, want %q", tt.e, got, tt.want)
		}
	}
}

// page is the state of a page that a test changes: a fragment's HTML by
// its topic, and the renders the stream asked for.
type page struct {
	mu      sync.Mutex
	html    map[string]string
	renders [][]string
	fail    error
	n       sse.Notifier[string]
}

func newPage() *page {
	return &page{html: map[string]string{"runs": "<ul></ul>", "tasks": "<ol></ol>"}}
}

// set changes the HTML of topic and notifies it.
func (p *page) set(topic, html string) {
	p.mu.Lock()
	p.html[topic] = html
	p.mu.Unlock()
	p.n.Notify(topic)
}

// render renders the fragments of changed, or all of them.
func (p *page) render(changed []string) ([]sse.Event, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.renders = append(p.renders, changed)
	if p.fail != nil {
		return nil, p.fail
	}
	if changed == nil {
		changed = []string{"runs", "tasks"}
	}
	var es []sse.Event
	for _, t := range changed {
		es = append(es, sse.Event{Name: t, Data: p.html[t]})
	}
	return es, nil
}

// serve serves the stream of p on a test server, and returns a reader of
// it and the error Serve returned, once it has.
func serve(t *testing.T, p *page, o sse.Options) (*localwebtest.Reader, <-chan error) {
	t.Helper()
	errc := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errc <- sse.Serve(w, r, &p.n, p.render, o)
	}))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type is %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control is %q", got)
	}
	return localwebtest.NewReader(resp.Body), errc
}

// next reads the next event of r.
func next(t *testing.T, r *localwebtest.Reader) sse.Event {
	t.Helper()
	e, err := r.Next()
	if err != nil {
		t.Fatalf("reading the stream: %v", err)
	}
	return e
}

// A stream sends every fragment first, then each fragment that changed,
// once its HTML differs from what the stream sent last.
func TestAStreamSendsTheBaselineThenWhatChanged(t *testing.T) {
	t.Parallel()

	p := newPage()
	r, _ := serve(t, p, sse.Options{Coalesce: time.Millisecond})
	for _, want := range []sse.Event{{Name: "runs", Data: "<ul></ul>"}, {Name: "tasks", Data: "<ol></ol>"}} {
		if got := next(t, r); got != want {
			t.Errorf("baseline: got %+v, want %+v", got, want)
		}
	}
	if r.Retry != "2000" {
		t.Errorf("retry is %q, want 2000", r.Retry)
	}
	// The same HTML is not sent again: the next event is the one that
	// changed.
	p.set("runs", "<ul></ul>")
	p.set("tasks", "<ol><li>1</li></ol>")
	if got, want := next(t, r), (sse.Event{Name: "tasks", Data: "<ol><li>1</li></ol>"}); got != want {
		t.Errorf("after a change: got %+v, want %+v", got, want)
	}
}

// Changes that come within the coalescing time cost one render, which
// gets their topics in the order of their first change.
func TestChangesInABurstCostOneRender(t *testing.T) {
	t.Parallel()

	p := newPage()
	r, _ := serve(t, p, sse.Options{Coalesce: 200 * time.Millisecond})
	next(t, r)
	next(t, r)
	p.set("tasks", "<ol>1</ol>")
	p.set("runs", "<ul>1</ul>")
	p.set("tasks", "<ol>2</ol>")
	got := []sse.Event{next(t, r), next(t, r)}
	want := []sse.Event{{Name: "tasks", Data: "<ol>2</ol>"}, {Name: "runs", Data: "<ul>1</ul>"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if want := [][]string{nil, {"tasks", "runs"}}; !slices.EqualFunc(p.renders, want, slices.Equal) {
		t.Errorf("renders %q, want %q", p.renders, want)
	}
}

// A wake-up without topics renders nothing; an idle stream sends a
// comment now and then.
func TestAnIdleStreamKeepsAlive(t *testing.T) {
	t.Parallel()

	p := newPage()
	errc := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errc <- sse.Serve(w, r, &p.n, p.render, sse.Options{Coalesce: time.Millisecond, KeepAlive: 5 * time.Millisecond})
	}))
	defer srv.Close()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	p.n.Notify()
	var b strings.Builder
	buf := make([]byte, 512)
	for !strings.Contains(b.String(), ": keep-alive\n\n") {
		n, err := resp.Body.Read(buf)
		if err != nil {
			t.Fatalf("reading the stream: %v", err)
		}
		b.Write(buf[:n])
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.renders) != 1 {
		t.Errorf("renders %q, want the baseline alone", p.renders)
	}
}

// A negative duration turns its thing off, and never panics: no retry
// field, no keep-alive, and a change that is rendered at once.
func TestNegativeOptionsTurnTheirThingOff(t *testing.T) {
	t.Parallel()

	p := newPage()
	r, _ := serve(t, p, sse.Options{Coalesce: -1, KeepAlive: -1, Retry: -time.Second})
	next(t, r)
	next(t, r)
	p.set("runs", "<ul>1</ul>")
	if got, want := next(t, r), (sse.Event{Name: "runs", Data: "<ul>1</ul>"}); got != want {
		t.Errorf("after a change: got %+v, want %+v", got, want)
	}
	if r.Retry != "" {
		t.Errorf("retry is %q, want none", r.Retry)
	}
}

// The stream ends without an error when the client goes or the server
// stops.
func TestAStreamEndsWithItsClientOrItsServer(t *testing.T) {
	t.Parallel()

	p := newPage()
	done := make(chan struct{})
	errc := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errc <- sse.Serve(w, r, &p.n, p.render, sse.Options{Done: done})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	next(t, localwebtest.NewReader(resp.Body))
	cancel()
	if err := <-errc; err != nil {
		t.Errorf("Serve after the client went: %v", err)
	}
	_ = resp.Body.Close()

	req, _ = http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	close(done)
	if err := <-errc; err != nil {
		t.Errorf("Serve after the server stopped: %v", err)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Errorf("the stream did not end cleanly: %v", err)
	}
}

// Without Done, a stream ends when the localweb.Serve that serves it
// starts to stop, and does not hold the stop for the grace period.
func TestAStreamEndsWhenLocalwebServeStops(t *testing.T) {
	t.Parallel()

	p := newPage()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		errc <- sse.Serve(w, r, &p.n, p.render, sse.Options{})
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- localweb.Serve(ctx, []net.Listener{l}, h, localweb.ServeOptions{Grace: time.Hour}) }()

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+l.Addr().String(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	next(t, localwebtest.NewReader(resp.Body))
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Errorf("localweb.Serve = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the stream held the stop")
	}
	if err := <-errc; err != nil {
		t.Errorf("sse.Serve = %v, want nil", err)
	}
}

// A first render that fails, or names an event with a line break, writes
// nothing, so that the program can answer the request.
func TestAFailedFirstRenderWritesNothing(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	for _, render := range []func([]string) ([]sse.Event, error){
		func([]string) ([]sse.Event, error) { return nil, boom },
		func([]string) ([]sse.Event, error) { return []sse.Event{{Name: "a\nb"}}, nil },
	} {
		var n sse.Notifier[string]
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/events", nil)
		err := sse.Serve(w, r, &n, render, sse.Options{})
		if err == nil || !strings.HasPrefix(err.Error(), "sse: render the first events: ") {
			t.Errorf("Serve = %v, want the render's error", err)
		}
		if w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
			t.Errorf("Serve wrote %q with the headers %v", w.Body, w.Header())
		}
	}
}

// A later render that fails, or names an event with a line break, ends
// the stream with its error, and the browser opens it again.
func TestAFailedLaterRenderEndsTheStream(t *testing.T) {
	t.Parallel()

	for _, topic := range []string{"runs", "a\nb"} {
		p := newPage()
		r, errc := serve(t, p, sse.Options{Coalesce: time.Millisecond})
		next(t, r)
		next(t, r)
		if topic == "runs" {
			p.mu.Lock()
			p.fail = errors.New("boom")
			p.mu.Unlock()
		}
		p.n.Notify(topic)
		if err := <-errc; err == nil || !strings.HasPrefix(err.Error(), "sse: render the events of ") {
			t.Errorf("%q: Serve = %v, want the render's error", topic, err)
		}
	}
}

// flaky is a ResponseWriter whose flushes fail from the nth on, with err.
type flaky struct {
	*httptest.ResponseRecorder
	mu      sync.Mutex
	flushes int
	n       int
	err     error
}

func (f *flaky) FlushError() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushes++
	if f.flushes >= f.n {
		return f.err
	}
	return nil
}

// A ResponseWriter that cannot flush cannot stream, which is an error; a
// flush that fails later is a client that went, which is none.
func TestAFlushThatFails(t *testing.T) {
	t.Parallel()

	gone := errors.New("broken pipe")
	tests := []struct {
		name    string
		w       *flaky
		o       sse.Options
		notify  bool
		wantErr bool
	}{
		{"no flusher", &flaky{n: 1, err: http.ErrNotSupported}, sse.Options{}, false, true},
		{"gone at the baseline", &flaky{n: 1, err: gone}, sse.Options{}, false, false},
		{"gone at a change", &flaky{n: 2, err: gone}, sse.Options{Coalesce: time.Millisecond}, true, false},
		{"gone at a keep-alive", &flaky{n: 2, err: gone}, sse.Options{KeepAlive: time.Millisecond}, false, false},
	}
	for _, tt := range tests {
		tt.w.ResponseRecorder = httptest.NewRecorder()
		p := newPage()
		if tt.notify {
			go func() {
				for range 100 {
					p.set("runs", time.Now().String())
					time.Sleep(time.Millisecond)
				}
			}()
		}
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		err := sse.Serve(tt.w, r, &p.n, p.render, tt.o)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: Serve = %v", tt.name, err)
		}
	}
}
