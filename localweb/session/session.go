// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package session lets into a local web server only the browser that
// opened the address the program printed: the address carries a launch
// token, which the first navigation exchanges for a session cookie, and
// each session has a CSRF token that every request that may change
// something sends back.
//
// The program makes the token with NewToken, prints its address with the
// token in the query parameter TokenParam, and puts Manager.Middleware
// behind the checks of package localweb, which refuse the requests of
// other sites and of other Host names:
//
//	sessions, err := session.New(session.Options{Token: token, CookiePrefix: "prog_ui_", CSRFHeader: "X-Prog-CSRF"})
//	h := localweb.Headers("")(localweb.Hosts(b.Answers, localweb.HostsOptions{})(
//		localweb.SameOrigin(localweb.OriginPolicy{Navigate: true})(sessions.Middleware(mux))))
//
// A page puts CSRF(r.Context()) into its forms, in the field
// Options.CSRFField, and into the header Options.CSRFHeader of the
// requests its script sends, such as htmx's hx-headers.
//
// The cookie is HttpOnly, so no script reads it, and SameSite=Strict, so
// that a browser leaves it out of a request that another site starts. Its
// name ends in the port of the request's Host, because a browser keeps
// cookies by host name and not by port: two servers on one machine each
// keep their own session. It has no Secure attribute, which a browser
// refuses over the plain HTTP of a loopback listener, and no lifetime: it
// ends with the browser's session, and its value with the process.
//
// The names and texts this package sends are an interface of the kit
// (doc/decisions.md, decision 34).
package session

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/GSI-HPC/go-clikit/localweb"
)

const (
	// TokenParam is the query parameter that carries the launch token in
	// the address the program prints.
	TokenParam = "token"
	// DefaultCSRFField is the form field of the CSRF token when
	// Options.CSRFField is empty.
	DefaultCSRFField = "csrf"
	// DefaultField is the former name of DefaultCSRFField.
	//
	// Deprecated: Use DefaultCSRFField, which says which token the field
	// carries (decision 36).
	DefaultField = DefaultCSRFField
	// DefaultMaxBody is the most bytes the body of a request that may
	// change something holds when Options.MaxBody is zero.
	DefaultMaxBody = 64 << 10
)

// NewToken returns a new secret: 32 bytes from the system's random source,
// in the URL-safe base64 alphabet without padding, which needs no escaping
// in a URL or a cookie. A launch token lives in memory only and ends with
// the process, so that a bookmark of an earlier launch signs nobody in.
func NewToken() string {
	var b [32]byte
	// crypto/rand never fails; it ends the process when it cannot read.
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// Options configures New.
type Options struct {
	// Token is the launch token, from NewToken.
	Token string
	// CookiePrefix is the start of the session cookie's name, such as
	// "prog_ui_"; the port of the request's Host follows.
	CookiePrefix string
	// CSRFHeader is the request header that carries the CSRF token, such
	// as "X-Prog-CSRF".
	CSRFHeader string
	// CSRFField is the form field that carries the CSRF token, for a form
	// a browser sends without script; "" is DefaultCSRFField.
	CSRFField string
	// MaxBody is the most bytes the body of a request that may change
	// something holds; 0 is DefaultMaxBody.
	MaxBody int64
	// Refuse answers a refused request; nil is localweb.Refuse. A program
	// that serves pages adds the line that tells the user where to find
	// the address to open.
	Refuse localweb.Refuser

	// Cookie is the former name of CookiePrefix, read while CookiePrefix
	// is empty.
	//
	// Deprecated: Use CookiePrefix; the port follows it (decision 36).
	Cookie string
	// Header is the former name of CSRFHeader, read while CSRFHeader is
	// empty.
	//
	// Deprecated: Use CSRFHeader, which says which token it carries
	// (decision 36).
	Header string
	// Field is the former name of CSRFField, read while CSRFField is
	// empty.
	//
	// Deprecated: Use CSRFField, which says which token it carries
	// (decision 36).
	Field string
}

// Manager holds the sessions of one launch.
type Manager struct {
	o Options

	mu sync.Mutex
	// csrf maps the ID of each session to its CSRF token.
	csrf map[string]string
}

// New returns the Manager of the launch o.Token. It refuses an Options
// without a token, a cookie prefix or a CSRF header.
func New(o Options) (*Manager, error) {
	o.CookiePrefix = cmp.Or(o.CookiePrefix, o.Cookie)
	o.CSRFHeader = cmp.Or(o.CSRFHeader, o.Header)
	o.CSRFField = cmp.Or(o.CSRFField, o.Field, DefaultCSRFField)
	if o.Token == "" || o.CookiePrefix == "" || o.CSRFHeader == "" {
		return nil, errors.New("session: Options needs a Token, a CookiePrefix and a CSRFHeader")
	}
	if o.MaxBody == 0 {
		o.MaxBody = DefaultMaxBody
	}
	if o.Refuse == nil {
		o.Refuse = localweb.Refuse
	}
	return &Manager{o: o, csrf: map[string]string{}}, nil
}

// Sessions is the former name of Manager.
//
// Deprecated: Use Manager, which does not nearly repeat the name of the
// package (decision 36).
type Sessions = Manager

// csrfKey is the context key of a request's CSRF token.
type csrfKey struct{}

// CSRF returns the CSRF token of the session of the request whose context
// is ctx, or "" outside Manager.Middleware.
func CSRF(ctx context.Context) string {
	s, _ := ctx.Value(csrfKey{}).(string)
	return s
}

// Middleware runs next for the requests of a session, with the session's
// CSRF token in the request's context (CSRF):
//
//   - A GET or HEAD with TokenParam in its query starts a session when the
//     token is the launch's, and redirects, 303 See Other, to the same
//     address without the token, so that it leaves the address bar and
//     the history; a wrong token is refused.
//   - A request without the cookie of a session is refused, 403 Forbidden.
//   - A request that may change something has its body capped at
//     Options.MaxBody, and its form parsed by the time next runs; it must
//     carry the session's CSRF token in Options.CSRFHeader or, failing
//     that, in the form field Options.CSRFField.
func (s *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		port, ok := requestPort(r.Host)
		if !ok {
			s.o.Refuse(w, r, localweb.MisdirectedRefusal(r.Host))
			return
		}
		if localweb.SafeMethod(r.Method) && r.URL.Query().Has(TokenParam) {
			s.exchange(w, r, port)
			return
		}
		csrf, ok := s.session(r, port)
		if !ok {
			s.o.Refuse(w, r, localweb.Refusal{
				Status: http.StatusForbidden, Message: "This browser is not signed in.",
				Reason: "no session cookie",
			})
			return
		}
		if !localweb.SafeMethod(r.Method) && !s.checkWrite(w, r, csrf) {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), csrfKey{}, csrf)))
	})
}

// exchange starts a session for a request with the launch token, and
// redirects to its address without the token.
func (s *Manager) exchange(w http.ResponseWriter, r *http.Request, port string) {
	q := r.URL.Query()
	if !sameSecret(q.Get(TokenParam), s.o.Token) {
		s.o.Refuse(w, r, localweb.Refusal{
			Status: http.StatusForbidden, Message: "This link holds an old or wrong token.",
			Reason: "wrong launch token",
		})
		return
	}
	id, csrf := NewToken(), NewToken()
	s.mu.Lock()
	s.csrf[id] = csrf
	s.mu.Unlock()
	// Secure would keep the cookie from plain HTTP, which is all a
	// loopback listener without a certificate speaks.
	http.SetCookie(w, &http.Cookie{
		Name: s.o.CookiePrefix + port, Value: id, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	q.Del(TokenParam)
	// A path that begins with two slashes would name another host in
	// the Location header.
	to := url.URL{Path: "/" + strings.TrimLeft(r.URL.Path, "/"), RawQuery: q.Encode()}
	// No cache may keep an answer under an address that holds the token.
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, to.String(), http.StatusSeeOther)
}

// session returns the CSRF token of the request's session, if it has one.
func (s *Manager) session(r *http.Request, port string) (string, bool) {
	c, err := r.Cookie(s.o.CookiePrefix + port)
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	csrf, ok := s.csrf[c.Value]
	return csrf, ok
}

// checkWrite lets through a request that may change something only when it
// carries the session's CSRF token, and caps and parses its body. It
// answers the request itself when it refuses it.
func (s *Manager) checkWrite(w http.ResponseWriter, r *http.Request, csrf string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, s.o.MaxBody)
	if err := r.ParseForm(); err != nil {
		f := localweb.Refusal{Status: http.StatusBadRequest, Message: "The form is malformed.", Reason: err.Error()}
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			f.Status, f.Message = http.StatusRequestEntityTooLarge, "The form is larger than "+size(s.o.MaxBody)+"."
		}
		s.o.Refuse(w, r, f)
		return false
	}
	sent := r.Header.Get(s.o.CSRFHeader)
	if sent == "" {
		sent = r.PostForm.Get(s.o.CSRFField)
	}
	if !sameSecret(sent, csrf) {
		s.o.Refuse(w, r, localweb.Refusal{
			Status: http.StatusForbidden, Message: "The request lacks this page's CSRF token. Reload the page.",
			Reason: "wrong or no CSRF token",
		})
		return false
	}
	return true
}

// size words n bytes, in KiB when it is a whole number of them.
func size(n int64) string {
	if n%1024 == 0 {
		return fmt.Sprintf("%d KiB", n/1024)
	}
	return fmt.Sprintf("%d bytes", n)
}

// requestPort returns the port a request was sent to, as its Host header
// names it, 80 when it names none. It refuses a port that is not a
// number, which the cookie's name must not hold; localweb.Hosts refuses
// such a Host before.
func requestPort(host string) (string, bool) {
	_, port, err := net.SplitHostPort(host)
	if err != nil || port == "" {
		return "80", true
	}
	_, err = strconv.ParseUint(port, 10, 16)
	return port, err == nil && strings.Trim(port, "0123456789") == ""
}

// sameSecret compares a value a client sent with a secret in constant
// time, so that the time an answer takes tells nothing of how much of a
// guess was right. An empty secret matches nothing.
func sameSecret(sent, secret string) bool {
	return secret != "" && subtle.ConstantTimeCompare([]byte(sent), []byte(secret)) == 1
}
