// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package localwebtest helps to test a local web server that uses the
// packages of localweb: a Client that signs in with the launch token and
// sends its requests as a page of the server's own origin does, and a
// Reader of the events of a stream.
//
//	srv := httptest.NewServer(handler)
//	c := localwebtest.Login(t, srv.URL, token)
//	c.Header.Set("X-Prog-CSRF", csrfFromThePage)
//	resp := c.PostForm(t, "/actions/stop", url.Values{"id": {"1"}})
//
// A server behind localweb.Hosts answers only to the address it bound, so
// a test serves its handler through httptest.NewServer, whose URL is
// 127.0.0.1 and its port, and gives the Hosts check that Host.
package localwebtest

import (
	"bufio"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/localweb/session"
	"github.com/GSI-HPC/go-clikit/localweb/sse"
)

// Client sends requests to a server as its pages do in a browser that has
// signed in.
type Client struct {
	// Base is the server's address, such as "http://127.0.0.1:7374".
	Base string
	// Header is set on every request, such as the session's CSRF token.
	Header http.Header
	hc     *http.Client
}

// Login opens base with the launch token, as the user's browser opens the
// address a program prints, and returns a Client that holds the session
// cookie. It fails the test unless the server answers with a redirect
// that sets a cookie and leads to a page it serves.
func Login(tb testing.TB, base, token string) *Client {
	tb.Helper()
	jar, _ := cookiejar.New(nil) // New returns no error
	c := &Client{Base: base, Header: http.Header{}, hc: &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	req, err := http.NewRequestWithContext(tb.Context(), http.MethodGet,
		base+"/?"+url.Values{session.TokenParam: {token}}.Encode(), nil)
	if err != nil {
		tb.Fatalf("localwebtest: login: %v", err)
	}
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	resp, err := c.hc.Do(req)
	if err != nil {
		tb.Fatalf("localwebtest: login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || len(resp.Cookies()) == 0 {
		tb.Fatalf("localwebtest: login: got %s with %d cookies, want 303 See Other with the session cookie",
			resp.Status, len(resp.Cookies()))
	}
	return c
}

// Do sends req as a page of the server's origin does, with Origin and
// Sec-Fetch-Site set, and with Header, and returns the answer, whose body
// the test closes. A redirect is returned, not followed. It fails the test
// when the request cannot be sent.
func (c *Client) Do(tb testing.TB, req *http.Request) *http.Response {
	tb.Helper()
	for k, v := range c.Header {
		req.Header[k] = v
	}
	req.Header.Set("Origin", c.Base)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp, err := c.hc.Do(req)
	if err != nil {
		tb.Fatalf("localwebtest: %s %s: %v", req.Method, req.URL, err)
	}
	return resp
}

// Get sends a GET of path, as a page's script does.
func (c *Client) Get(tb testing.TB, path string) *http.Response {
	tb.Helper()
	return c.Do(tb, c.request(tb, http.MethodGet, path, nil))
}

// PostForm sends form to path as a POST, as a page's form does.
func (c *Client) PostForm(tb testing.TB, path string, form url.Values) *http.Response {
	tb.Helper()
	req := c.request(tb, http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.Do(tb, req)
}

// Events opens the event stream at path and returns its Reader. The
// stream ends when the test does. It fails the test unless the server
// answers 200 with an event stream.
func (c *Client) Events(tb testing.TB, path string) *Reader {
	tb.Helper()
	req := c.request(tb, http.MethodGet, path, nil)
	req.Header.Set("Accept", "text/event-stream")
	resp := c.Do(tb, req)
	tb.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		tb.Fatalf("localwebtest: events %s: got %s, %q, want 200 OK, text/event-stream",
			path, resp.Status, resp.Header.Get("Content-Type"))
	}
	return NewReader(resp.Body)
}

// request returns a request of method for path, which ends with the test.
func (c *Client) request(tb testing.TB, method, path string, body io.Reader) *http.Request {
	tb.Helper()
	req, err := http.NewRequestWithContext(tb.Context(), method, c.Base+path, body)
	if err != nil {
		tb.Fatalf("localwebtest: %s %s: %v", method, path, err)
	}
	return req
}

// Reader reads the events of a Server-Sent Events stream.
type Reader struct {
	sc *bufio.Scanner
	// Retry is the last retry field the stream sent, as it wrote it.
	Retry string
}

// NewReader returns a Reader of the stream r.
func NewReader(r io.Reader) *Reader {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	return &Reader{sc: sc}
}

// Next returns the next event of the stream, with its data lines joined
// by a line feed. It skips comments, such as keep-alives, and the retry
// field, and returns io.EOF at the end of the stream.
func (r *Reader) Next() (sse.Event, error) {
	var e sse.Event
	var data []string
	for r.sc.Scan() {
		line := r.sc.Text()
		if line == "" {
			if data != nil {
				e.Data = strings.Join(data, "\n")
				return e, nil
			}
			// An event without data is no event, and its name goes with
			// it.
			e = sse.Event{}
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			e.Name = value
		case "data":
			data = append(data, value)
		case "retry":
			r.Retry = value
		}
	}
	if err := r.sc.Err(); err != nil {
		return sse.Event{}, err
	}
	return sse.Event{}, io.EOF
}
