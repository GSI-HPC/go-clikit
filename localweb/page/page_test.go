// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package page_test

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/GSI-HPC/go-clikit/localweb/page"
)

// files returns a file system of one template file, t.html.
func files(src string) fstest.MapFS {
	return fstest.MapFS{"t.html": &fstest.MapFile{Data: []byte(src)}}
}

// mustParse parses src as t.html, or stops the test.
func mustParse(t *testing.T, src string, o page.Options) *page.Set {
	t.Helper()
	s, err := page.Parse(files(src), o, "*.html")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const blocks = `{{define "layout"}}<main>{{include .Content .}}</main>{{end}}` +
	`{{define "page"}}<h1>{{.Title}}</h1>{{include "frag" .}}{{end}}` +
	`{{define "frag"}}<p>{{.Title}}</p>{{end}}`

// Parse fails with a message that starts with "page:".
func TestParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fsys fstest.MapFS
		pat  string
	}{
		{"no match", files("x"), "*.txt"},
		{"syntax", files(`{{define "a"}}{{if}}{{end}}`), "*.html"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := page.Parse(tc.fsys, page.Options{}, tc.pat)
			if err == nil || !strings.HasPrefix(err.Error(), "page:") {
				t.Fatalf("got %v, want an error starting \"page:\"", err)
			}
			if s != nil {
				t.Errorf("got a set with an error")
			}
		})
	}
}

// A function in Options.Funcs is available, and Parse leaves the map alone.
func TestParseFuncs(t *testing.T) {
	t.Parallel()

	funcs := template.FuncMap{"up": strings.ToUpper}
	s := mustParse(t, `{{define "a"}}{{up .}}{{end}}`, page.Options{Funcs: funcs})
	got, err := s.Render("a", "hi")
	if err != nil || got != "HI" {
		t.Errorf("Render = %q, %v, want HI", got, err)
	}
	if _, ok := funcs["include"]; ok || len(funcs) != 1 {
		t.Errorf("Funcs changed: %v", funcs)
	}
}

// include renders a block named by a value, and nests.
func TestInclude(t *testing.T) {
	t.Parallel()

	s := mustParse(t, blocks, page.Options{})
	data := map[string]any{"Content": "page", "Title": "T"}
	got, err := s.Render("layout", data)
	want := "<main><h1>T</h1><p>T</p></main>"
	if err != nil || got != want {
		t.Errorf("Render = %q, %v, want %q", got, err, want)
	}
}

// An error inside an included block fails the outer render.
func TestIncludeErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	funcs := template.FuncMap{"fail": func() (string, error) { return "", boom }}
	src := `{{define "outer"}}<b>{{include "inner" .}}</b>{{end}}` +
		`{{define "inner"}}{{fail}}{{end}}` +
		`{{define "lost"}}{{include "nope" .}}{{end}}`
	s := mustParse(t, src, page.Options{Funcs: funcs})

	_, err := s.Render("outer", nil)
	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "page: render outer") {
		t.Errorf("outer: got %v, want page: render outer wrapping boom", err)
	}
	_, err = s.Render("lost", nil)
	if err == nil || !strings.Contains(err.Error(), "page: render nope") {
		t.Errorf("lost: got %v, want a failed render of nope", err)
	}
}

// A block that includes itself, or blocks that include each other, fail
// the render past MaxIncludeDepth, with and without Dev, and the set
// renders as before afterwards.
func TestIncludeCycle(t *testing.T) {
	t.Parallel()

	const src = `{{define "a"}}x{{include "a" .}}{{end}}` +
		`{{define "b"}}{{include "c" .}}{{end}}` +
		`{{define "c"}}{{include "b" .}}{{end}}` +
		`{{define "ok"}}<p>{{include "frag" .}}</p>{{end}}` +
		`{{define "frag"}}{{.}}{{end}}`
	for _, dev := range []bool{false, true} {
		t.Run(fmt.Sprint("dev=", dev), func(t *testing.T) {
			t.Parallel()
			s := mustParse(t, src, page.Options{Dev: dev})
			for _, tc := range []struct{ name, want string }{
				{"a", "page: render a: include a: nested more than 100 deep"},
				{"b", "page: render b: include c: nested more than 100 deep"},
			} {
				got, err := s.Render(tc.name, nil)
				if err == nil || err.Error() != tc.want || got != "" {
					t.Errorf("Render(%s) = %q, %v, want %q", tc.name, got, err, tc.want)
				}
				w := httptest.NewRecorder()
				if err := s.Respond(w, http.StatusOK, tc.name, nil); err == nil || w.Body.Len() != 0 {
					t.Errorf("Respond(%s): got %v and %q, want an error and no body", tc.name, err, w.Body)
				}
				if got, err := s.Render("ok", "y"); err != nil || got != "<p>y</p>" {
					t.Errorf("Render(ok) after %s = %q, %v, want <p>y</p>", tc.name, got, err)
				}
			}
		})
	}
}

// include nests MaxIncludeDepth deep, and no deeper.
func TestIncludeDepth(t *testing.T) {
	t.Parallel()

	funcs := template.FuncMap{"dec": func(n int) int { return n - 1 }}
	s := mustParse(t, `{{define "n"}}{{if .}}.{{include "n" (dec .)}}{{end}}{{end}}`, page.Options{Funcs: funcs})
	got, err := s.Render("n", page.MaxIncludeDepth)
	if want := strings.Repeat(".", page.MaxIncludeDepth); err != nil || got != want {
		t.Errorf("Render(n, %d) = %q, %v, want %d dots", page.MaxIncludeDepth, got, err, page.MaxIncludeDepth)
	}
	_, err = s.Render("n", page.MaxIncludeDepth+1)
	if want := "page: render n: include n: nested more than 100 deep"; err == nil || err.Error() != want {
		t.Errorf("Render(n, %d): got %v, want %q", page.MaxIncludeDepth+1, err, want)
	}
}

// Has reports the blocks of the set.
func TestHas(t *testing.T) {
	t.Parallel()

	s := mustParse(t, blocks, page.Options{})
	if !s.Has("frag") {
		t.Error("Has(frag) = false, want true")
	}
	if s.Has("nope") {
		t.Error("Has(nope) = true, want false")
	}
}

// Render escapes data, and a missing block fails.
func TestRender(t *testing.T) {
	t.Parallel()

	s := mustParse(t, `{{define "p"}}<p>{{.}}</p>{{end}}`, page.Options{})
	got, err := s.Render("p", "<script>")
	if want := "<p>&lt;script&gt;</p>"; err != nil || got != want {
		t.Errorf("Render = %q, %v, want %q", got, err, want)
	}
	got, err = s.Render("nope", nil)
	if err == nil || got != "" || !strings.HasPrefix(err.Error(), "page: render nope") {
		t.Errorf("missing: got %q, %v, want page: render nope", got, err)
	}
}

// Respond writes the status, the headers and the body.
func TestRespond(t *testing.T) {
	t.Parallel()

	s := mustParse(t, `{{define "p"}}<p>{{.}}</p>{{end}}`, page.Options{})
	w := httptest.NewRecorder()
	if err := s.Respond(w, http.StatusTeapot, "p", "é"); err != nil {
		t.Fatal(err)
	}
	const body = "<p>é</p>"
	if w.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTeapot)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "9" {
		t.Errorf("Content-Length = %q, want 9", got)
	}
	if got := w.Body.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

// A failed render writes nothing.
func TestRespondFailure(t *testing.T) {
	t.Parallel()

	s := mustParse(t, `{{define "p"}}x{{end}}`, page.Options{})
	w := httptest.NewRecorder()
	err := s.Respond(w, http.StatusCreated, "nope", nil)
	if err == nil {
		t.Fatal("got no error")
	}
	if w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
		t.Errorf("wrote %d %q %v, want nothing", w.Code, w.Body, w.Header())
	}
}

// Without Dev the first parse stays; with it, the files are read anew.
func TestDev(t *testing.T) {
	t.Parallel()

	for _, dev := range []bool{false, true} {
		t.Run(fmt.Sprint("dev=", dev), func(t *testing.T) {
			t.Parallel()
			want := "old"
			if dev {
				want = "new"
			}
			if got := renderAfterEdit(t, page.Options{Dev: dev}); got != want {
				t.Errorf("Render = %q, want %q", got, want)
			}
		})
	}
}

// The deprecated field Reload reads the files anew, as Dev does.
func TestReloadIsDev(t *testing.T) {
	t.Parallel()

	if got := renderAfterEdit(t, page.Options{Reload: true}); got != "new" {
		t.Errorf("Render = %q, want new", got)
	}
}

// renderAfterEdit parses a block a of "old" with o, changes it to "new",
// and returns what Render renders of it.
func renderAfterEdit(t *testing.T, o page.Options) string {
	t.Helper()
	fsys := files(`{{define "a"}}old{{end}}`)
	s, err := page.Parse(fsys, o, "*.html")
	if err != nil {
		t.Fatal(err)
	}
	fsys["t.html"] = &fstest.MapFile{Data: []byte(`{{define "a"}}new{{end}}`)}
	got, err := s.Render("a", nil)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Under Dev a template that stops parsing fails every use of the set.
func TestDevBroken(t *testing.T) {
	t.Parallel()

	fsys := files(`{{define "a"}}ok{{end}}`)
	s, err := page.Parse(fsys, page.Options{Dev: true}, "*.html")
	if err != nil {
		t.Fatal(err)
	}
	fsys["t.html"] = &fstest.MapFile{Data: []byte(`{{define "a"}}{{if}}{{end}}`)}

	if _, err := s.Render("a", nil); err == nil || !strings.HasPrefix(err.Error(), "page:") {
		t.Errorf("Render: got %v, want a page: error", err)
	}
	w := httptest.NewRecorder()
	if err := s.Respond(w, http.StatusOK, "a", nil); err == nil || w.Body.Len() != 0 {
		t.Errorf("Respond: got %v and %q, want an error and no body", err, w.Body)
	}
	if s.Has("a") {
		t.Error("Has(a) = true, want false")
	}
}

// Render is safe for concurrent use, under Dev too.
func TestConcurrentRender(t *testing.T) {
	t.Parallel()

	for _, dev := range []bool{false, true} {
		s := mustParse(t, blocks, page.Options{Dev: dev})
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 20 {
					got, err := s.Render("layout", map[string]any{"Content": "page", "Title": "T"})
					if err != nil || got != "<main><h1>T</h1><p>T</p></main>" {
						t.Errorf("Render = %q, %v", got, err)
						return
					}
				}
			}()
		}
		wg.Wait()
	}
}
