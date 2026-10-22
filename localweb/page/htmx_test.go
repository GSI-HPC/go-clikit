// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package page_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GSI-HPC/go-clikit/localweb/page"
)

// request returns a request of the given HX-Request header, if any.
func request(hx string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	if hx != "" {
		r.Header.Set("HX-Request", hx)
	}
	return r
}

// Only the header HX-Request: true marks htmx.
func TestIsHTMX(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		hx   string
		want bool
	}{
		{"true", true},
		{"", false},
		{"1", false},
		{"TRUE", false},
		{"false", false},
	} {
		t.Run("["+tc.hx+"]", func(t *testing.T) {
			t.Parallel()
			if got := page.IsHTMX(request(tc.hx)); got != tc.want {
				t.Errorf("IsHTMX = %v, want %v", got, tc.want)
			}
		})
	}
}

// Redirect answers htmx and a plain form in different ways.
func TestRedirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hx         string
		code       int
		hxRedirect string
		location   string
	}{
		{"htmx", "true", http.StatusNoContent, "/done", ""},
		{"plain", "", http.StatusSeeOther, "", "/done"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			page.Redirect(w, request(tc.hx), "/done")
			if w.Code != tc.code {
				t.Errorf("status = %d, want %d", w.Code, tc.code)
			}
			if got := w.Header().Get("HX-Redirect"); got != tc.hxRedirect {
				t.Errorf("HX-Redirect = %q, want %q", got, tc.hxRedirect)
			}
			if got := w.Header().Get("Location"); got != tc.location {
				t.Errorf("Location = %q, want %q", got, tc.location)
			}
		})
	}
}

// Retarget sets both headers.
func TestRetarget(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	page.Retarget(w, "#dialog", "innerHTML")
	if got := w.Header().Get("HX-Retarget"); got != "#dialog" {
		t.Errorf("HX-Retarget = %q", got)
	}
	if got := w.Header().Get("HX-Reswap"); got != "innerHTML" {
		t.Errorf("HX-Reswap = %q", got)
	}
}
