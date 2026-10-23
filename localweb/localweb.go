// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package localweb is the guard of a web page that a command-line program
// serves to the user who started it, on the user's own machine: the
// listener, the server, and the checks a request passes before the
// program's handler sees it.
//
// Such a server acts as the user, and whoever can send it a request acts
// as the user too. Other users of the machine can reach a loopback port;
// every page in the user's browser can send requests to it; and a page of
// another site can make its own domain name resolve to 127.0.0.1 (DNS
// rebinding). The package keeps all three out:
//
//   - Listen binds a loopback address and nothing else, or a Unix socket
//     that nobody but the user can reach or replace. Its Bound tells the
//     Host header values the server answers to.
//   - PeerListener closes, on Linux, each TCP connection that a process of
//     another user opened, before a request is read.
//   - Hosts refuses a request whose Host header names another server,
//     which is how DNS rebinding shows.
//   - SameOrigin refuses a request that a page of another site caused, by
//     the Sec-Fetch-Site and Origin headers a browser sets and no page can
//     change. Every port of 127.0.0.1 is the same site, so SameSite cookies
//     alone do not keep out the pages that another program serves there.
//   - Headers sets the Content-Security-Policy, the frame, referrer and
//     resource policies, and nosniff on every answer.
//   - Serve serves with every timeout set, and stops within a grace period
//     when its context ends.
//
// localweb/session exchanges the launch token for a session cookie and
// holds the CSRF token, and localweb/sse streams the live parts of a page.
//
// The checks are middleware of the type func(http.Handler) http.Handler, so
// that an http.ServeMux and a router such as chi can both take them. A
// program puts them in this order, outermost first:
//
//	h := localweb.Headers("")(localweb.Hosts(b.Answers, localweb.HostsOptions{})(
//		localweb.SameOrigin(localweb.Origin{Navigate: true})(sessions.Middleware(mux))))
//
// A check that refuses a request answers it with a Refusal: a status, a
// message the client reads, and a reason for the program's log. The
// messages are an interface of the kit (doc/decisions.md, decision 34). A
// Refuser lets the program write them in its own form, such as JSON, and
// log the reason.
//
// The kit reads no environment: the program passes its user ID, the
// address to listen on and the policy to send.
package localweb

import "net/http"

// Refusal is the answer of a check that refuses a request.
type Refusal struct {
	// Status is the HTTP status of the answer.
	Status int
	// Message is the sentence the client reads. It says what was wrong
	// and never what would have been right, such as the Host the server
	// answers to.
	Message string
	// Reason is why the check refused the request, for the program's
	// log, such as "Sec-Fetch-Site is cross-site". It holds no secret.
	Reason string
}

// Refuser answers a request that a check refuses.
type Refuser func(w http.ResponseWriter, r *http.Request, f Refusal)

// Refuse is the Refuser a check uses when it is given none: it answers
// with f.Status and f.Message as one line of plain text, which no cache
// keeps.
func Refuse(w http.ResponseWriter, _ *http.Request, f Refusal) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(f.Status)
	_, _ = w.Write([]byte(f.Message + "\n"))
}

// refuser returns r, or Refuse when r is nil.
func refuser(r Refuser) Refuser {
	if r == nil {
		return Refuse
	}
	return r
}

// SafeMethod reports whether a request of method m changes nothing: GET
// and HEAD. A server of this package changes nothing on such a request, so
// that a navigation from another site, which the checks let through, does
// nothing but show a page.
func SafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead
}
