// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package htmx_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/GSI-HPC/go-clikit/localweb/assets"
	"github.com/GSI-HPC/go-clikit/localweb/htmx"
)

// A program serves the scripts as the static files of localweb/assets, and
// puts Config into its pages.
func Example() {
	vendor, err := assets.New(htmx.Files(), "/htmx/", assets.Options{})
	if err != nil {
		fmt.Println(err)
		return
	}
	mux := http.NewServeMux()
	mux.Handle("GET /htmx/", vendor)

	target := vendor.URL(htmx.Script)
	fmt.Println(strings.HasPrefix(target, "/htmx/"), strings.HasSuffix(target, "/htmx.min.js"))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	fmt.Println(w.Code, w.Header().Get("Content-Type"))
	fmt.Println(htmx.Config)
	// Output:
	// true true
	// 200 text/javascript; charset=utf-8
	// {"allowEval":false,"allowScriptTags":false,"includeIndicatorStyles":false,"selfRequestsOnly":true,"historyCacheSize":0}
}
