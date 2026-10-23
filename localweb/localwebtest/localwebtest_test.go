// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localwebtest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/GSI-HPC/go-clikit/localweb"
	"github.com/GSI-HPC/go-clikit/localweb/localwebtest"
	"github.com/GSI-HPC/go-clikit/localweb/session"
	"github.com/GSI-HPC/go-clikit/localweb/sse"
)

const (
	token  = "LAUNCHTOKEN"
	header = "X-Prog-CSRF"
)

// fakeTB is a testing.TB that keeps what a helper reports; Fatalf ends the
// goroutine, as it does in a test.
type fakeTB struct {
	testing.TB
	ctx context.Context

	mu       sync.Mutex
	fatals   []string
	cleanups []func()
}

func (f *fakeTB) Helper()                  {}
func (f *fakeTB) Context() context.Context { return f.ctx }
func (f *fakeTB) Cleanup(fn func())        { f.cleanups = append(f.cleanups, fn) }

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.fatals = append(f.fatals, fmt.Sprintf(format, args...))
	f.mu.Unlock()
	runtime.Goexit()
}

// fails runs fn with a fakeTB and returns what it reported as fatal.
func fails(t *testing.T, fn func(tb testing.TB)) []string {
	t.Helper()
	f := &fakeTB{ctx: t.Context()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(f)
	}()
	<-done
	for _, c := range slices.Backward(f.cleanups) {
		c()
	}
	return f.fatals
}

// server serves a page that writes its CSRF token, a POST that echoes its
// form, and an event stream, behind the checks of localweb and a session.
func server(t *testing.T) *httptest.Server {
	t.Helper()
	sessions, err := session.New(session.Options{Token: token, CookiePrefix: "prog_ui_", CSRFHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	var n sse.Notifier[string]
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, session.CSRF(r.Context()))
	})
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.PostForm.Encode())
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		_ = sse.Serve(w, r, &n, func([]string) ([]sse.Event, error) {
			return []sse.Event{{Name: "runs", Data: "<ul>\n</ul>"}}, nil
		}, sse.Options{})
	})
	srv := httptest.NewServer(nil)
	b := &localweb.Bound{Addrs: []netip.AddrPort{srv.Listener.Addr().(*net.TCPAddr).AddrPort()}}
	srv.Config.Handler = localweb.Hosts(b.Answers, localweb.HostsOptions{})(localweb.SameOrigin(localweb.OriginPolicy{Navigate: true})(sessions.Middleware(mux)))
	t.Cleanup(srv.Close)
	return srv
}

// A signed-in Client sends requests as the server's own pages do: they
// pass the Host, origin and session checks, and a POST passes the CSRF
// check with the token the page holds.
func TestAClientSignsInAndActsAsAPage(t *testing.T) {
	t.Parallel()

	srv := server(t)
	c := localwebtest.Login(t, srv.URL, token)
	resp := c.Get(t, "/")
	csrf, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(csrf) == 0 {
		t.Fatalf("GET /: %s %q", resp.Status, csrf)
	}

	resp = c.PostForm(t, "/echo", url.Values{"a": {"1"}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a POST without the CSRF token: %s, want 403", resp.Status)
	}
	c.Header.Set(header, string(csrf))
	resp = c.PostForm(t, "/echo", url.Values{"a": {"1"}})
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "a=1" {
		t.Errorf("a POST with the CSRF token: %s %q, want 200 a=1", resp.Status, body)
	}

	r := c.Events(t, "/events")
	e, err := r.Next()
	if err != nil || e != (sse.Event{Name: "runs", Data: "<ul>\n</ul>"}) || r.Retry != "2000" {
		t.Errorf("the first event: %+v, %v, retry %q", e, err, r.Retry)
	}
}

// Each helper fails the test with what went wrong.
func TestTheHelpersFailTheTest(t *testing.T) {
	t.Parallel()

	srv := server(t)
	down := httptest.NewServer(nil)
	down.Close()
	tests := []struct {
		name string
		fn   func(tb testing.TB)
		want string
	}{
		{"wrong token", func(tb testing.TB) { localwebtest.Login(tb, srv.URL, "WRONG") }, "want 303 See Other"},
		{"no server", func(tb testing.TB) { localwebtest.Login(tb, down.URL, token) }, "localwebtest: login: "},
		{"bad base", func(tb testing.TB) { localwebtest.Login(tb, "http://\x7f", token) }, "localwebtest: login: "},
		{"bad path", func(tb testing.TB) {
			localwebtest.Login(t, srv.URL, token).Get(tb, "/\x7f")
		}, "localwebtest: GET /\x7f: "},
		{"server gone", func(tb testing.TB) {
			c := localwebtest.Login(t, srv.URL, token)
			c.Base = down.URL
			c.Get(tb, "/")
		}, "localwebtest: GET http://"},
		{"no stream", func(tb testing.TB) {
			localwebtest.Login(t, srv.URL, token).Events(tb, "/")
		}, `localwebtest: events /: got 200 OK, "text/plain; charset=utf-8"`},
	}
	for _, tt := range tests {
		got := fails(t, tt.fn)
		if len(got) != 1 || !strings.Contains(got[0], tt.want) {
			t.Errorf("%s: reported %q, want %q", tt.name, got, tt.want)
		}
	}
}

// The Reader skips comments and events without data, keeps the retry,
// and joins the lines of data.
func TestTheReaderReadsEvents(t *testing.T) {
	t.Parallel()

	r := localwebtest.NewReader(strings.NewReader("retry: 2000\n\n: keep-alive\n\nevent: lost\n\n" +
		"event: runs\ndata: <ul>\ndata: </ul>\nid: 1\n\ndata:x\n\n"))
	var got []sse.Event
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, e)
	}
	want := []sse.Event{{Name: "runs", Data: "<ul>\n</ul>"}, {Data: "x"}}
	if !slices.Equal(got, want) || r.Retry != "2000" {
		t.Errorf("got %+v and retry %q, want %+v and 2000", got, r.Retry, want)
	}

	r = localwebtest.NewReader(strings.NewReader("data: " + strings.Repeat("x", 1<<20) + "\n\n"))
	if _, err := r.Next(); err == nil || errors.Is(err, io.EOF) {
		t.Errorf("a line over 1 MiB: %v, want an error", err)
	}
}
