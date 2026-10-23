// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testHost   = "127.0.0.1:7374"
	testOrigin = "http://127.0.0.1:7374"
	evil       = "http://evil.example"
)

// request makes a request to testHost with the headers h, which may hold
// a header more than once.
func request(method string, h map[string][]string) *http.Request {
	r := httptest.NewRequest(method, "http://"+testHost+"/", nil)
	for k, v := range h {
		r.Header[k] = v
	}
	return r
}

func TestCrossSite(t *testing.T) {
	t.Parallel()

	nav := map[string][]string{"Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Dest": {"document"}}
	with := func(site string, more ...map[string][]string) map[string][]string {
		h := map[string][]string{}
		if site != "" {
			h["Sec-Fetch-Site"] = []string{site}
		}
		for _, m := range more {
			for k, v := range m {
				h[k] = v
			}
		}
		return h
	}
	origin := func(v ...string) map[string][]string { return map[string][]string{"Origin": v} }
	navigate := Origin{Navigate: true}
	others := Origin{Others: []string{"http://127.0.0.1:5173", "http://localhost:5173"}}

	tests := []struct {
		name   string
		method string
		h      map[string][]string
		o      Origin
		want   string
	}{
		// Sec-Fetch-Site absent: no browser, or an old one.
		{name: "absent GET", method: "GET", h: with(""), want: ""},
		{name: "absent HEAD", method: "HEAD", h: with(""), want: ""},
		{name: "absent POST", method: "POST", h: with(""), want: "no Origin on a POST"},
		{name: "absent DELETE", method: "DELETE", h: with(""), want: "no Origin on a DELETE"},
		{name: "absent POST own origin", method: "POST", h: with("", origin(testOrigin)), want: ""},
		{name: "absent POST other origin", method: "POST", h: with("", origin(evil)), want: "Origin is " + evil},
		{name: "absent GET own origin", method: "GET", h: with("", origin(testOrigin)), want: ""},
		{name: "absent GET other origin", method: "GET", h: with("", origin(evil)), want: "Origin is " + evil},
		{name: "absent GET null origin", method: "GET", h: with("", origin("null")), want: "Origin is null"},
		{name: "absent POST null origin", method: "POST", h: with("", origin("null")), want: "Origin is null"},
		{name: "absent POST https origin", method: "POST", h: with("", origin("https://"+testHost)), want: "Origin is https://" + testHost},
		{name: "absent POST empty origin", method: "POST", h: with("", origin("")), want: "Origin is "},
		{
			name: "absent POST repeated origin", method: "POST", h: with("", origin(testOrigin, testOrigin)),
			want: "Origin is " + testOrigin + ", " + testOrigin,
		},
		{
			name: "absent GET repeated origin", method: "GET", h: with("", origin(testOrigin, evil)),
			want: "Origin is " + testOrigin + ", " + evil,
		},
		{name: "absent POST listed origin", method: "POST", h: with("", origin("http://127.0.0.1:5173")), o: others, want: ""},
		{name: "absent POST second listed origin", method: "POST", h: with("", origin("http://localhost:5173")), o: others, want: ""},
		{name: "absent POST unlisted origin", method: "POST", h: with("", origin(evil)), o: others, want: "Origin is " + evil},
		{name: "absent POST own origin with Others", method: "POST", h: with("", origin(testOrigin)), o: others, want: ""},
		{name: "absent POST repeated listed origin", method: "POST", h: with("", origin("http://127.0.0.1:5173", "http://127.0.0.1:5173")), o: others,
			want: "Origin is http://127.0.0.1:5173, http://127.0.0.1:5173"},

		// same-origin.
		{name: "same-origin GET", method: "GET", h: with("same-origin"), want: ""},
		{name: "same-origin POST", method: "POST", h: with("same-origin"), want: "no Origin on a POST"},
		{name: "same-origin POST own origin", method: "POST", h: with("same-origin", origin(testOrigin)), want: ""},
		{name: "same-origin POST other origin", method: "POST", h: with("same-origin", origin(evil)), want: "Origin is " + evil},
		{name: "same-origin POST Navigate", method: "POST", h: with("same-origin", origin(testOrigin)), o: navigate, want: ""},
		{name: "same-origin GET other origin Navigate", method: "GET", h: with("same-origin", origin(evil)), o: navigate, want: "Origin is " + evil},

		// cross-site, same-site and none: only a navigation, and only when allowed.
		{name: "cross-site GET", method: "GET", h: with("cross-site"), want: "Sec-Fetch-Site is cross-site"},
		{name: "cross-site POST", method: "POST", h: with("cross-site", origin(testOrigin)), want: "Sec-Fetch-Site is cross-site"},
		{name: "cross-site navigation", method: "GET", h: with("cross-site", nav), want: "Sec-Fetch-Site is cross-site"},
		{name: "cross-site navigation Navigate", method: "GET", h: with("cross-site", nav), o: navigate, want: ""},
		{name: "cross-site HEAD navigation Navigate", method: "HEAD", h: with("cross-site", nav), o: navigate, want: ""},
		{name: "cross-site POST navigation Navigate", method: "POST", h: with("cross-site", nav, origin(testOrigin)), o: navigate, want: "Sec-Fetch-Site is cross-site"},
		{name: "cross-site GET Navigate without mode", method: "GET", h: with("cross-site"), o: navigate, want: "Sec-Fetch-Site is cross-site"},
		{
			name: "cross-site GET cors Navigate", method: "GET", o: navigate, want: "Sec-Fetch-Site is cross-site",
			h: with("cross-site", map[string][]string{"Sec-Fetch-Mode": {"cors"}, "Sec-Fetch-Dest": {"document"}}),
		},
		{
			name: "cross-site GET iframe Navigate", method: "GET", o: navigate, want: "Sec-Fetch-Site is cross-site",
			h: with("cross-site", map[string][]string{"Sec-Fetch-Mode": {"navigate"}, "Sec-Fetch-Dest": {"iframe"}}),
		},
		{
			name: "cross-site GET navigate without destination Navigate", method: "GET", o: navigate, want: "Sec-Fetch-Site is cross-site",
			h: with("cross-site", map[string][]string{"Sec-Fetch-Mode": {"navigate"}}),
		},
		{name: "cross-site navigation with another origin", method: "GET", h: with("cross-site", nav, origin(evil)), o: navigate, want: "Origin is " + evil},
		{name: "cross-site navigation with its own origin", method: "GET", h: with("cross-site", nav, origin(testOrigin)), o: navigate, want: ""},

		{name: "same-site GET", method: "GET", h: with("same-site"), want: "Sec-Fetch-Site is same-site"},
		{name: "same-site POST own origin", method: "POST", h: with("same-site", origin(testOrigin)), want: "Sec-Fetch-Site is same-site"},
		{name: "same-site navigation", method: "GET", h: with("same-site", nav), want: "Sec-Fetch-Site is same-site"},
		{name: "same-site navigation Navigate", method: "GET", h: with("same-site", nav), o: navigate, want: ""},
		{name: "same-site POST Navigate", method: "POST", h: with("same-site", nav, origin(testOrigin)), o: navigate, want: "Sec-Fetch-Site is same-site"},
		{name: "same-site GET Navigate without mode", method: "GET", h: with("same-site"), o: navigate, want: "Sec-Fetch-Site is same-site"},

		{name: "none GET", method: "GET", h: with("none"), want: "Sec-Fetch-Site is none"},
		{name: "none navigation", method: "GET", h: with("none", nav), want: "Sec-Fetch-Site is none"},
		{name: "none navigation Navigate", method: "GET", h: with("none", nav), o: navigate, want: ""},
		{name: "none HEAD navigation Navigate", method: "HEAD", h: with("none", nav), o: navigate, want: ""},
		{name: "none POST Navigate", method: "POST", h: with("none", nav, origin(testOrigin)), o: navigate, want: "Sec-Fetch-Site is none"},
		{name: "none GET Navigate without mode", method: "GET", h: with("none"), o: navigate, want: "Sec-Fetch-Site is none"},
		{name: "none navigation with another origin", method: "GET", h: with("none", nav, origin(evil)), o: navigate, want: "Origin is " + evil},

		{name: "an unknown value", method: "GET", h: with("elsewhere"), o: navigate, want: "Sec-Fetch-Site is elsewhere"},
		{name: "an unknown value navigating", method: "GET", h: with("elsewhere", nav), o: navigate, want: ""},
		{name: "same-origin in another case", method: "GET", h: with("Same-Origin"), want: "Sec-Fetch-Site is Same-Origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := CrossSite(request(tt.method, tt.h), tt.o); got != tt.want {
				t.Errorf("CrossSite = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCrossSiteOnTLS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		origin string
		want   string
	}{
		{origin: "https://" + testHost},
		{origin: testOrigin, want: "Origin is " + testOrigin},
	}
	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			t.Parallel()

			r := request("POST", map[string][]string{"Origin": {tt.origin}})
			r.TLS = &tls.ConnectionState{}

			if got := CrossSite(r, Origin{}); got != tt.want {
				t.Errorf("CrossSite = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeMethod(t *testing.T) {
	t.Parallel()

	for m, want := range map[string]bool{
		"GET": true, "HEAD": true, "POST": false, "PUT": false, "DELETE": false, "PATCH": false, "OPTIONS": false,
		"get": false, "": false, "TRACE": false, "CONNECT": false,
	} {
		if got := SafeMethod(m); got != want {
			t.Errorf("SafeMethod(%q) = %v, want %v", m, got, want)
		}
	}
}

func TestRefuse(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()

	Refuse(w, request("GET", nil), Refusal{Status: http.StatusTeapot, Message: "No.", Reason: "secret reason"})

	res := w.Result()
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusTeapot)
	}
	if got := w.Body.String(); got != "No.\n" {
		t.Errorf("body = %q, want %q", got, "No.\n")
	}
	for k, want := range map[string]string{
		"Content-Type":           "text/plain; charset=utf-8",
		"X-Content-Type-Options": "nosniff",
		"Cache-Control":          "no-store",
	} {
		if got := res.Header.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

// refusalRecorder is a Refuser that keeps what it was given.
type refusalRecorder struct {
	got []Refusal
}

func (rr *refusalRecorder) refuse(w http.ResponseWriter, _ *http.Request, f Refusal) {
	rr.got = append(rr.got, f)
	w.WriteHeader(http.StatusTeapot)
}

// reached is a handler that notes it was called.
type reached struct{ n int }

func (h *reached) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.n++
	w.WriteHeader(http.StatusNoContent)
}

func TestHosts(t *testing.T) {
	t.Parallel()

	answers := func(host string) bool { return host == testHost }
	tests := []struct {
		name string
		host string
		ok   bool
	}{
		{name: "the host of the server", host: testHost, ok: true},
		{name: "another host", host: "rebind.example:7374"},
		{name: "no host", host: ""},
		{name: "the same name on another port", host: "127.0.0.1:7375"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			t.Run("default Refuser", func(t *testing.T) {
				t.Parallel()

				next := &reached{}
				r := request("GET", nil)
				r.Host = tt.host
				w := httptest.NewRecorder()

				Hosts(answers, HostsOptions{})(next).ServeHTTP(w, r)

				if tt.ok {
					if next.n != 1 || w.Code != http.StatusNoContent {
						t.Errorf("reached %d times, status %d, want once and 204", next.n, w.Code)
					}
					return
				}
				if next.n != 0 {
					t.Error("the handler was reached")
				}
				if w.Code != http.StatusMisdirectedRequest {
					t.Errorf("status = %d, want 421", w.Code)
				}
				if want := "This server answers to its loopback address only.\n"; w.Body.String() != want {
					t.Errorf("body = %q, want %q", w.Body.String(), want)
				}
				if strings.Contains(w.Body.String(), testHost) {
					t.Error("the refusal names the Host the server answers to")
				}
				if got := w.Header().Get("Cache-Control"); got != "no-store" {
					t.Errorf("Cache-Control = %q, want no-store", got)
				}
			})

			t.Run("custom Refuser", func(t *testing.T) {
				t.Parallel()

				next := &reached{}
				rr := &refusalRecorder{}
				r := request("GET", nil)
				r.Host = tt.host
				w := httptest.NewRecorder()

				Hosts(answers, HostsOptions{Refuse: rr.refuse})(next).ServeHTTP(w, r)

				if tt.ok {
					if next.n != 1 || len(rr.got) != 0 {
						t.Errorf("reached %d times, refused %d, want once and never", next.n, len(rr.got))
					}
					return
				}
				want := Refusal{
					Status:  http.StatusMisdirectedRequest,
					Message: "This server answers to its loopback address only.",
					Reason:  "Host is " + tt.host,
				}
				if next.n != 0 || len(rr.got) != 1 || rr.got[0] != want {
					t.Errorf("reached %d times, refusals %+v, want never and %+v", next.n, rr.got, want)
				}
				if w.Code != http.StatusTeapot {
					t.Errorf("status = %d, want the Refuser's 418", w.Code)
				}
			})
		})
	}
}

// Hosts refuses a nil answers when it is made, not on the first request.
func TestHostsPanicsWithoutAnswers(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "answers") {
			t.Errorf("recovered %v, want a panic that names answers", r)
		}
	}()
	Hosts(nil, HostsOptions{})
}

func TestSameOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		h          map[string][]string
		wantReason string
	}{
		{name: "a navigation of the page", method: "GET", h: map[string][]string{"Sec-Fetch-Site": {"same-origin"}}},
		{name: "a post of the page", method: "POST", h: map[string][]string{"Sec-Fetch-Site": {"same-origin"}, "Origin": {testOrigin}}},
		{
			name: "a page of another site", method: "GET", h: map[string][]string{"Sec-Fetch-Site": {"cross-site"}},
			wantReason: "Sec-Fetch-Site is cross-site",
		},
		{
			name: "a form of another site", method: "POST", h: map[string][]string{"Origin": {evil}},
			wantReason: "Origin is " + evil,
		},
		{name: "a post without an origin", method: "POST", wantReason: "no Origin on a POST"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			t.Run("default Refuser", func(t *testing.T) {
				t.Parallel()

				next := &reached{}
				w := httptest.NewRecorder()

				SameOrigin(Origin{})(next).ServeHTTP(w, request(tt.method, tt.h))

				if tt.wantReason == "" {
					if next.n != 1 {
						t.Errorf("reached %d times, want once", next.n)
					}
					return
				}
				if next.n != 0 {
					t.Error("the handler was reached")
				}
				if w.Code != http.StatusForbidden {
					t.Errorf("status = %d, want 403", w.Code)
				}
				if want := "The request came from another site.\n"; w.Body.String() != want {
					t.Errorf("body = %q, want %q", w.Body.String(), want)
				}
				if strings.Contains(w.Body.String(), tt.wantReason) {
					t.Error("the refusal holds the reason, which is for the log")
				}
			})

			t.Run("custom Refuser", func(t *testing.T) {
				t.Parallel()

				next := &reached{}
				rr := &refusalRecorder{}
				w := httptest.NewRecorder()

				SameOrigin(Origin{Refuse: rr.refuse})(next).ServeHTTP(w, request(tt.method, tt.h))

				if tt.wantReason == "" {
					if next.n != 1 || len(rr.got) != 0 {
						t.Errorf("reached %d times, refused %d, want once and never", next.n, len(rr.got))
					}
					return
				}
				want := Refusal{Status: http.StatusForbidden, Message: "The request came from another site.", Reason: tt.wantReason}
				if next.n != 0 || len(rr.got) != 1 || rr.got[0] != want {
					t.Errorf("reached %d times, refusals %+v, want never and %+v", next.n, rr.got, want)
				}
				if w.Code != http.StatusTeapot {
					t.Errorf("status = %d, want the Refuser's 418", w.Code)
				}
			})
		})
	}
}

func TestHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		csp  string
		want string
	}{
		{name: "the default policy", csp: "", want: ContentSecurityPolicy},
		{name: "a policy of the program", csp: "default-src 'self'", want: "default-src 'self'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := &reached{}
			w := httptest.NewRecorder()

			Headers(tt.csp)(next).ServeHTTP(w, request("GET", nil))

			if next.n != 1 {
				t.Errorf("reached %d times, want once", next.n)
			}
			for k, want := range map[string]string{
				"Content-Security-Policy":      tt.want,
				"Referrer-Policy":              "same-origin",
				"X-Content-Type-Options":       "nosniff",
				"X-Frame-Options":              "DENY",
				"Cross-Origin-Opener-Policy":   "same-origin",
				"Cross-Origin-Resource-Policy": "same-origin",
			} {
				if got := w.Header().Get(k); got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
			if got := w.Header().Get("Cache-Control"); got != "" {
				t.Errorf("Cache-Control = %q, want none", got)
			}
		})
	}
}

func TestHeadersOnARefusal(t *testing.T) {
	t.Parallel()

	h := Headers("")(Hosts(func(string) bool { return false }, HostsOptions{})(&reached{}))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, request("GET", nil))

	if w.Code != http.StatusMisdirectedRequest {
		t.Errorf("status = %d, want 421", w.Code)
	}
	if got := w.Header().Get("Content-Security-Policy"); got != ContentSecurityPolicy {
		t.Errorf("Content-Security-Policy = %q on a refusal, want the default", got)
	}
}

func TestNoStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		next http.HandlerFunc
		want string
	}{
		{name: "sets it", next: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }, want: "no-store"},
		{
			name: "a handler may set another",
			next: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Cache-Control", "max-age=31536000, immutable")
			},
			want: "max-age=31536000, immutable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()

			NoStore(tt.next).ServeHTTP(w, request("GET", nil))

			if got := w.Header().Get("Cache-Control"); got != tt.want {
				t.Errorf("Cache-Control = %q, want %q", got, tt.want)
			}
		})
	}
}
