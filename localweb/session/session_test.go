// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/localweb"
	"github.com/GSI-HPC/go-clikit/localweb/session"
)

const (
	token  = "LAUNCHTOKEN"
	host   = "127.0.0.1:7374"
	cookie = "prog_ui_7374"
	header = "X-Prog-CSRF"
)

// newSessions returns sessions of the launch token, changed by fn, and
// their middleware around a handler that writes the request's CSRF token.
func newSessions(t *testing.T, fn func(*session.Options)) http.Handler {
	t.Helper()
	o := session.Options{Token: token, CookiePrefix: "prog_ui_", CSRFHeader: header}
	if fn != nil {
		fn(&o)
	}
	s, err := session.New(o)
	if err != nil {
		t.Fatal(err)
	}
	return s.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("csrf=" + session.CSRF(r.Context())))
	}))
}

// serve sends a request to h and returns the answer.
func serve(h http.Handler, method, target string, body string, set func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Host = host
	if body != "" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if set != nil {
		set(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// login signs in to h and returns the session cookie and its CSRF token.
func login(t *testing.T, h http.Handler) (*http.Cookie, string) {
	t.Helper()
	w := serve(h, http.MethodGet, "/?token="+token, "", nil)
	cs := w.Result().Cookies()
	if w.Code != http.StatusSeeOther || len(cs) != 1 {
		t.Fatalf("login: got %d with cookies %v, want 303 and one cookie", w.Code, cs)
	}
	w = serve(h, http.MethodGet, "/", "", func(r *http.Request) { r.AddCookie(cs[0]) })
	csrf, ok := strings.CutPrefix(w.Body.String(), "csrf=")
	if w.Code != http.StatusOK || !ok || csrf == "" {
		t.Fatalf("after login: got %d %q, want 200 and the CSRF token", w.Code, w.Body)
	}
	return cs[0], csrf
}

// New refuses Options that would let anyone in, or no one.
func TestNewNeedsATokenACookieAndAHeader(t *testing.T) {
	t.Parallel()

	for _, o := range []session.Options{
		{CookiePrefix: "c_", CSRFHeader: "X-C"},
		{Token: token, CSRFHeader: "X-C"},
		{Token: token, CookiePrefix: "c_"},
	} {
		if _, err := session.New(o); err == nil {
			t.Errorf("New(%+v) took Options with a field missing", o)
		}
	}
}

// New refuses a cookie prefix and a CSRF header that net/http would not
// send, and says which.
func TestNewRefusesNamesThatAreNoTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		o    session.Options
		want string
	}{
		{session.Options{Token: token, CookiePrefix: "prog ui ", CSRFHeader: header}, `Options.CookiePrefix "prog ui "`},
		{session.Options{Token: token, CookiePrefix: "prog;ui", CSRFHeader: header}, `Options.CookiePrefix "prog;ui"`},
		{session.Options{Token: token, CookiePrefix: "prog_ui_", CSRFHeader: "X-Prog CSRF"}, `Options.CSRFHeader "X-Prog CSRF"`},
		{session.Options{Token: token, CookiePrefix: "prog_ui_", CSRFHeader: "X-Prog:CSRF"}, `Options.CSRFHeader "X-Prog:CSRF"`},
	}
	for _, tt := range tests {
		if _, err := session.New(tt.o); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("New(%+v) = %v, want an error that names %s", tt.o, err, tt.want)
		}
	}
}

// The first navigation exchanges the token for a cookie of the port, and
// leaves the token out of the address it redirects to.
func TestTheTokenIsExchangedForASessionCookie(t *testing.T) {
	t.Parallel()

	h := newSessions(t, nil)
	tests := []struct {
		method, target, location string
	}{
		{http.MethodGet, "/runs/1?token=" + token + "&tab=log", "/runs/1?tab=log"},
		{http.MethodHead, "/?token=" + token, "/"},
		// Two slashes would name another host in Location.
		{http.MethodGet, "//example.org/x?token=" + token, "/example.org/x"},
	}
	for _, tt := range tests {
		w := serve(h, tt.method, tt.target, "", nil)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != tt.location {
			t.Errorf("%s %s: got %d to %q, want 303 to %q", tt.method, tt.target, w.Code, w.Header().Get("Location"), tt.location)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s %s: Cache-Control is %q, want no-store", tt.method, tt.target, got)
		}
		cs := w.Result().Cookies()
		if len(cs) != 1 {
			t.Fatalf("%s %s: got cookies %v, want one", tt.method, tt.target, cs)
		}
		c := cs[0]
		if c.Name != cookie || c.Value == "" || c.Value == token || c.Path != "/" || !c.HttpOnly ||
			c.SameSite != http.SameSiteStrictMode || c.Secure || c.MaxAge != 0 || !c.Expires.IsZero() {
			t.Errorf("%s %s: got the cookie %+v", tt.method, tt.target, c)
		}
	}
}

// Every refusal says what was wrong in the words of the kit, and the
// request reaches no handler.
func TestRequestsWithoutASessionAreRefused(t *testing.T) {
	t.Parallel()

	h := newSessions(t, nil)
	c, _ := login(t, h)
	tests := []struct {
		name, method, target string
		set                  func(*http.Request)
		status               int
		message              string
	}{
		{
			"wrong token", http.MethodGet, "/?token=WRONG", nil,
			http.StatusForbidden, "This link holds an old or wrong token.",
		},
		{
			"empty token", http.MethodGet, "/?token=", nil,
			http.StatusForbidden, "This link holds an old or wrong token.",
		},
		{
			"no cookie", http.MethodGet, "/", nil,
			http.StatusForbidden, "This browser is not signed in.",
		},
		{
			"a POST does not exchange", http.MethodPost, "/?token=" + token, nil,
			http.StatusForbidden, "This browser is not signed in.",
		},
		{
			"unknown session", http.MethodGet, "/", func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: cookie, Value: "made-up"})
			},
			http.StatusForbidden, "This browser is not signed in.",
		},
		{
			"another port's cookie", http.MethodGet, "/", func(r *http.Request) {
				r.AddCookie(c)
				r.Host = "127.0.0.1:7375"
			},
			http.StatusForbidden, "This browser is not signed in.",
		},
		{
			"a port that is no number", http.MethodGet, "/", func(r *http.Request) {
				r.AddCookie(c)
				r.Host = "127.0.0.1:x"
			},
			http.StatusMisdirectedRequest, "This server answers to its loopback address only.",
		},
		{
			"a port too large", http.MethodGet, "/", func(r *http.Request) {
				r.Host = "127.0.0.1:65536"
			},
			http.StatusMisdirectedRequest, "This server answers to its loopback address only.",
		},
		{
			"a signed port", http.MethodGet, "/", func(r *http.Request) {
				r.Host = "127.0.0.1:+80"
			},
			http.StatusMisdirectedRequest, "This server answers to its loopback address only.",
		},
	}
	for _, tt := range tests {
		w := serve(h, tt.method, tt.target, "", tt.set)
		if w.Code != tt.status || w.Body.String() != tt.message+"\n" {
			t.Errorf("%s: got %d %q, want %d %q", tt.name, w.Code, w.Body, tt.status, tt.message)
		}
	}
}

// A Host without a port was sent to port 80, and its cookie is named so.
func TestAHostWithoutAPortIsPort80(t *testing.T) {
	t.Parallel()

	h := newSessions(t, nil)
	w := serve(h, http.MethodGet, "/?token="+token, "", func(r *http.Request) { r.Host = "127.0.0.1" })
	if cs := w.Result().Cookies(); len(cs) != 1 || cs[0].Name != "prog_ui_80" {
		t.Errorf("got cookies %v, want prog_ui_80", cs)
	}
}

// A request that may change something carries the session's CSRF token,
// in the header or in the form, and its body is capped.
func TestARequestThatChangesSomethingCarriesTheCSRFToken(t *testing.T) {
	t.Parallel()

	h := newSessions(t, nil)
	c, csrf := login(t, h)
	withCookie := func(more func(*http.Request)) func(*http.Request) {
		return func(r *http.Request) {
			r.AddCookie(c)
			if more != nil {
				more(r)
			}
		}
	}
	tests := []struct {
		name    string
		body    string
		set     func(*http.Request)
		status  int
		message string
	}{
		{"header", "a=1", func(r *http.Request) { r.Header.Set(header, csrf) }, http.StatusOK, "csrf=" + csrf},
		{"field", "a=1&csrf=" + url.QueryEscape(csrf), nil, http.StatusOK, "csrf=" + csrf},
		{
			"none", "a=1", nil,
			http.StatusForbidden, "The request lacks this page's CSRF token. Reload the page.\n",
		},
		{
			"wrong header beats a right field", "csrf=" + url.QueryEscape(csrf), func(r *http.Request) { r.Header.Set(header, "x") },
			http.StatusForbidden, "The request lacks this page's CSRF token. Reload the page.\n",
		},
		{
			"too large", "a=" + strings.Repeat("x", session.DefaultMaxBody), nil,
			http.StatusRequestEntityTooLarge, "The form is larger than 64 KiB.\n",
		},
		{"malformed", "a=%zz", nil, http.StatusBadRequest, "The form is malformed.\n"},
	}
	for _, tt := range tests {
		w := serve(h, http.MethodPost, "/stop", tt.body, withCookie(tt.set))
		if w.Code != tt.status || w.Body.String() != tt.message {
			t.Errorf("%s: got %d %q, want %d %q", tt.name, w.Code, w.Body, tt.status, tt.message)
		}
	}
}

// The limit of the body and the form field are the program's to choose,
// and so is the form of a refusal.
func TestTheOptionsChangeTheLimitTheFieldAndTheRefusal(t *testing.T) {
	t.Parallel()

	var got localweb.Refusal
	h := newSessions(t, func(o *session.Options) {
		o.MaxBody = 100
		o.CSRFField = "token-of-form"
		o.Refuse = func(w http.ResponseWriter, _ *http.Request, f localweb.Refusal) {
			got = f
			w.WriteHeader(f.Status)
		}
	})
	c, csrf := login(t, h)
	w := serve(h, http.MethodPost, "/", "token-of-form="+url.QueryEscape(csrf), func(r *http.Request) { r.AddCookie(c) })
	if w.Code != http.StatusOK {
		t.Errorf("the token in the field Options.CSRFField: got %d, want 200", w.Code)
	}
	w = serve(h, http.MethodPost, "/", strings.Repeat("x", 101), func(r *http.Request) { r.AddCookie(c) })
	if w.Code != http.StatusRequestEntityTooLarge || got.Message != "The form is larger than 100 bytes." || got.Reason == "" {
		t.Errorf("a body of 101 bytes: got %d and %+v", w.Code, got)
	}
}

// The former names of the fields are read while the new ones are empty,
// and the new ones win.
func TestTheFormerNamesOfTheOptionsStillWork(t *testing.T) {
	t.Parallel()

	h := newSessions(t, func(o *session.Options) {
		*o = session.Options{Token: token, Cookie: "prog_ui_", Header: header, Field: "f"}
	})
	c, csrf := login(t, h)
	if c.Name != cookie {
		t.Errorf("the cookie of Cookie is %q, want %q", c.Name, cookie)
	}
	for _, set := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set(header, csrf) },
		nil,
	} {
		body := "f=" + url.QueryEscape(csrf)
		w := serve(h, http.MethodPost, "/", body, func(r *http.Request) {
			r.AddCookie(c)
			if set != nil {
				set(r)
			}
		})
		if w.Code != http.StatusOK {
			t.Errorf("the token in Header or Field: got %d, want 200", w.Code)
		}
	}

	h = newSessions(t, func(o *session.Options) {
		o.Cookie, o.Header, o.Field = "old_", "X-Old", "old"
		o.CSRFField = "new"
	})
	c, csrf = login(t, h)
	w := serve(h, http.MethodPost, "/", "new="+url.QueryEscape(csrf), func(r *http.Request) { r.AddCookie(c) })
	if c.Name != cookie || w.Code != http.StatusOK {
		t.Errorf("the new names: cookie %q and %d, want %q and 200", c.Name, w.Code, cookie)
	}
	if session.DefaultField != session.DefaultCSRFField {
		t.Errorf("DefaultField is %q, want %q", session.DefaultField, session.DefaultCSRFField)
	}
}

// Sessions is still the manager, under its former name.
func TestSessionsIsManager(t *testing.T) {
	t.Parallel()

	var s *session.Sessions
	s, err := session.New(session.Options{Token: token, CookiePrefix: "prog_ui_", CSRFHeader: header})
	if err != nil || s == nil {
		t.Errorf("New = %v, %v", s, err)
	}
}

// Outside the middleware a request has no CSRF token, and every token is
// new.
func TestCSRFIsEmptyOutsideASessionAndTokensDiffer(t *testing.T) {
	t.Parallel()

	if got := session.CSRF(context.Background()); got != "" {
		t.Errorf("CSRF of a bare context is %q", got)
	}
	a, b := session.NewToken(), session.NewToken()
	if a == b || len(a) != 43 || strings.ContainsAny(a, "+/=") {
		t.Errorf("NewToken gave %q and %q, want two URL-safe tokens of 43 characters", a, b)
	}
}
