// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"net/http"
	"slices"
	"strings"
)

// ContentSecurityPolicy is the policy Headers sends when it is given none.
// A page may load and contact its own origin and nothing else: no inline
// script or style, no plugin, no <base>, no frame around it, and no form
// that posts elsewhere. Images may also be data: URLs, which a bundler
// makes of small images.
const ContentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"font-src 'self'; connect-src 'self'; manifest-src 'self'; form-action 'self'; base-uri 'none'; " +
	"frame-ancestors 'none'"

// Headers returns middleware that sets the security headers on every
// answer, a refusal included, so it is the outermost check:
//
//   - Content-Security-Policy: csp, or ContentSecurityPolicy when csp is
//     empty.
//   - Referrer-Policy: same-origin. The address of the first navigation
//     holds the launch token, and no other site gets a referrer. Not
//     no-referrer: under it a browser sends a form's POST with the Origin
//     "null", which SameOrigin refuses.
//   - X-Content-Type-Options: nosniff, X-Frame-Options: DENY,
//     Cross-Origin-Opener-Policy and Cross-Origin-Resource-Policy:
//     same-origin.
//
// It sets no Cache-Control: NoStore does, where the answers must not be
// kept, so that files with a hash in their name can be cached.
func Headers(csp string) func(http.Handler) http.Handler {
	if csp == "" {
		csp = ContentSecurityPolicy
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", csp)
			h.Set("Referrer-Policy", "same-origin")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			next.ServeHTTP(w, r)
		})
	}
}

// NoStore sets Cache-Control: no-store on every answer of next: what a
// page shows of the user's machine must not outlive the session in a
// cache. A handler of next may set another Cache-Control, as one that
// serves files with a hash in their name does.
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// HostsOptions configures Hosts.
type HostsOptions struct {
	// Refuse answers a refused request; nil is Refuse.
	Refuse Refuser
}

// Hosts returns middleware that refuses every request whose Host header
// answers does not accept, as Bound.Answers does, with 421 Misdirected
// Request. This is the defence against DNS rebinding: a page of another
// site that makes its own name resolve to 127.0.0.1 reaches the server as
// its own origin, where the browser lets it read the answers, but its
// requests still name that site in Host.
//
// The refusal does not say which Host the server answers to. Hosts panics
// when answers is nil (doc/decisions.md, decision 40).
func Hosts(answers func(host string) bool, o HostsOptions) func(http.Handler) http.Handler {
	if answers == nil {
		panic("localweb: Hosts needs an answers func")
	}
	refuse := refuser(o.Refuse)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !answers(r.Host) {
				refuse(w, r, Refusal{
					Status:  http.StatusMisdirectedRequest,
					Message: "This server answers to its loopback address only.",
					Reason:  "Host is " + r.Host,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// OriginPolicy says which requests SameOrigin and CrossSite let in.
type OriginPolicy struct {
	// Navigate lets in a navigation of the whole page from anywhere, as
	// when the user follows a link or opens a bookmark: a GET or HEAD
	// whose Sec-Fetch-Mode is navigate and whose Sec-Fetch-Dest is
	// document. Set it for the pages; leave it off for what a page asks
	// for itself, such as its fragments, its event stream and its forms,
	// so that no page of another site starts anything.
	Navigate bool
	// Others are origins let in besides the request's own, written as a
	// browser writes them, such as "http://127.0.0.1:5173" for a
	// development server that proxies to this one. Each weakens the check
	// and is for a developer's machine only.
	Others []string
	// Refuse answers a request that SameOrigin refuses; nil is Refuse.
	// CrossSite ignores it.
	Refuse Refuser
}

// Origin is the former name of OriginPolicy.
//
// Deprecated: Use OriginPolicy; the type is a policy, not an origin
// (decision 36).
type Origin = OriginPolicy

// SameOrigin returns middleware that refuses, with 403 Forbidden, every
// request that CrossSite says came from another site.
func SameOrigin(o OriginPolicy) func(http.Handler) http.Handler {
	refuse := refuser(o.Refuse)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reason := CrossSite(r, o); reason != "" {
				refuse(w, r, Refusal{
					Status:  http.StatusForbidden,
					Message: "The request came from another site.",
					Reason:  reason,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CrossSite says why r came from another site than the server's own
// origin, or returns "" when it did not. It reads o.Navigate and o.Others
// and ignores o.Refuse, as it refuses nothing itself. It reads two
// headers, which a browser sets and no page can set or remove:
//
//   - Sec-Fetch-Site must be same-origin. A request without it passes to
//     the Origin check: it comes from a client that is no browser, or from
//     a browser too old to send it, for which the SameSite=Strict session
//     cookie is the defence. "none", a request the user started by typing
//     the address or opening a bookmark, and "same-site" and "cross-site"
//     pass only as a navigation that o.Navigate lets in. "same-site" is
//     no better than "cross-site": every other port of 127.0.0.1 is the
//     same site, and belongs to other programs or other users.
//   - Origin must be the request's own origin, "http://" and its Host, or
//     one of o.Others. A request that may change something must carry it;
//     a GET or HEAD may leave it out, as a browser does for a navigation.
func CrossSite(r *http.Request, o OriginPolicy) string {
	switch site := r.Header.Get("Sec-Fetch-Site"); site {
	case "", "same-origin":
	default:
		if !o.Navigate || !navigation(r) {
			return "Sec-Fetch-Site is " + site
		}
	}
	origin, ok := r.Header["Origin"]
	switch {
	case !ok && SafeMethod(r.Method):
		return ""
	case !ok:
		return "no Origin on a " + r.Method
	case len(origin) == 1 && (origin[0] == ownOrigin(r) || slices.Contains(o.Others, origin[0])):
		return ""
	default:
		return "Origin is " + strings.Join(origin, ", ")
	}
}

// navigation reports whether r navigates a whole page and may change
// nothing.
func navigation(r *http.Request) bool {
	return SafeMethod(r.Method) && r.Header.Get("Sec-Fetch-Mode") == "navigate" &&
		r.Header.Get("Sec-Fetch-Dest") == "document"
}

// ownOrigin returns the origin r was sent to.
func ownOrigin(r *http.Request) string {
	if r.TLS != nil {
		return "https://" + r.Host
	}
	return "http://" + r.Host
}
