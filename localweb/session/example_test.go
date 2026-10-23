// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"

	"github.com/GSI-HPC/go-clikit/localweb"
	"github.com/GSI-HPC/go-clikit/localweb/session"
)

// A program serves its pages behind a session. The first navigation
// exchanges the token for a cookie; a page puts the session's CSRF token
// into its forms and into the header its script sends; and a refusal tells
// the user where the address to open is.
func ExampleManager_Middleware() {
	token := "LAUNCHTOKEN" // session.NewToken() in a program
	sessions, err := session.New(session.Options{
		Token: token, Cookie: "prog_ui_", Header: "X-Prog-CSRF",
		Refuse: func(w http.ResponseWriter, r *http.Request, f localweb.Refusal) {
			f.Message += "\nOpen the address that prog ui printed in your terminal, or run prog ui again."
			localweb.Refuse(w, r, f)
		},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	page := template.Must(template.New("page").Parse(
		`<body hx-headers='{"X-Prog-CSRF": "{{.}}"}'><form method="post"><input type="hidden" name="csrf" value="{{.}}">`))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		_ = page.Execute(w, session.CSRF(r.Context()))
	})
	h := sessions.Middleware(mux)

	for _, target := range []string{"/?token=" + token, "/?token=OLD", "/"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.Host = "127.0.0.1:7374"
		h.ServeHTTP(w, r)
		if w.Code == http.StatusSeeOther {
			fmt.Println(w.Code, "to", w.Header().Get("Location"), "with the cookie", w.Result().Cookies()[0].Name)
		} else {
			fmt.Print(w.Code, " ", w.Body)
		}
	}
	// Output:
	// 303 to / with the cookie prog_ui_7374
	// 403 This link holds an old or wrong token.
	// Open the address that prog ui printed in your terminal, or run prog ui again.
	// 403 This browser is not signed in.
	// Open the address that prog ui printed in your terminal, or run prog ui again.
}
