// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package assets_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing/fstest"

	"github.com/GSI-HPC/go-clikit/localweb/assets"
)

// A program serves its static files under a prefix, links to them with URL
// and URLs, and a browser may keep each one for good, as its address holds
// a hash of every file.
func ExampleNew() {
	files := fstest.MapFS{
		"app.css":    {Data: []byte("body { margin: 0 }")},
		"css/a.css":  {Data: []byte("a {}")},
		"css/b.css":  {Data: []byte("b {}")},
		"css/c.json": {Data: []byte("{}")},
	}
	a, err := assets.New(files, "/static/", assets.Options{})
	if err != nil {
		fmt.Println(err)
		return
	}
	hide := func(s string) string { return strings.ReplaceAll(s, a.Version(), "VERSION") }

	fmt.Println(len(a.Version()))
	fmt.Println(hide(a.URL("app.css")))
	for _, u := range a.URLs("css/*.css") {
		fmt.Println(hide(u))
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", a)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, a.URL("app.css"), nil))
	fmt.Println(w.Code)
	fmt.Println(w.Header().Get("Content-Type"))
	fmt.Println(w.Header().Get("Cache-Control"))
	// Output:
	// 12
	// /static/VERSION/app.css
	// /static/VERSION/css/a.css
	// /static/VERSION/css/b.css
	// 200
	// text/css; charset=utf-8
	// private, max-age=31536000, immutable
}
