// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package page_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing/fstest"

	"github.com/GSI-HPC/go-clikit/localweb/page"
)

// A program parses its templates into one set, in which a layout places the
// block of the page it shows, and a page places a fragment, which an answer
// to htmx renders on its own.
func ExampleSet_Respond() {
	fsys := fstest.MapFS{"ui.html": &fstest.MapFile{Data: []byte(
		`{{define "layout"}}<main>{{include .Content .}}</main>{{end}}` +
			`{{define "page-hosts"}}<table>{{include "hosts-table" .}}</table>{{end}}` +
			`{{define "hosts-table"}}{{range .Hosts}}<tr><td>{{.}}</td></tr>{{end}}{{end}}`)}}
	set, err := page.Parse(fsys, page.Options{}, "*.html")
	if err != nil {
		fmt.Println(err)
		return
	}

	w := httptest.NewRecorder()
	data := map[string]any{"Content": "page-hosts", "Hosts": []string{"node1", "<node2>"}}
	if err := set.Respond(w, http.StatusOK, "layout", data); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(w.Code, w.Header().Get("Content-Type"))
	fmt.Println(w.Body)
	// Output:
	// 200 text/html; charset=utf-8
	// <main><table><tr><td>node1</td></tr><tr><td>&lt;node2&gt;</td></tr></table></main>
}

// After a request that changed something, htmx loads the new page from the
// header HX-Redirect, and a plain form follows the redirect.
func ExampleRedirect() {
	for _, hx := range []string{"true", ""} {
		r := httptest.NewRequest(http.MethodPost, "/jobs", nil)
		if hx != "" {
			r.Header.Set("HX-Request", hx)
		}
		w := httptest.NewRecorder()
		page.Redirect(w, r, "/jobs/7")
		fmt.Printf("%d HX-Redirect=%q Location=%q\n", w.Code, w.Header().Get("HX-Redirect"), w.Header().Get("Location"))
	}
	// Output:
	// 204 HX-Redirect="/jobs/7" Location=""
	// 303 HX-Redirect="" Location="/jobs/7"
}
