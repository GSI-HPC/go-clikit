// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package assets_test

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GSI-HPC/go-clikit/localweb/assets"
)

const prefix = "/static/"

// errFS is a file system that cannot be opened.
type errFS struct{}

func (errFS) Open(string) (fs.File, error) { return nil, errors.New("boom") }

// readFailFS lists its files but cannot read any of them.
type readFailFS struct{ fstest.MapFS }

func (readFailFS) ReadFile(string) ([]byte, error) { return nil, errors.New("unreadable") }

func newAssets(t *testing.T, fsys fs.FS, o assets.Options) *assets.Assets {
	t.Helper()
	a, err := assets.New(fsys, prefix, o)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func get(a *assets.Assets, method, target string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestNewRefusesABadPrefix(t *testing.T) {
	t.Parallel()

	for _, p := range []string{"", "static/", "/static", "static"} {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			a, err := assets.New(fstest.MapFS{}, p, assets.Options{})
			if err == nil || a != nil || !strings.Contains(err.Error(), "assets:") {
				t.Errorf("New(%q) = %v, %v, want an assets error", p, a, err)
			}
		})
	}
	if _, err := assets.New(fstest.MapFS{}, "/", assets.Options{}); err != nil {
		t.Errorf("New with the prefix %q: %v", "/", err)
	}
}

func TestNewFailsOnAnFSError(t *testing.T) {
	t.Parallel()

	tests := map[string]fs.FS{
		"walk": errFS{},
		"read": readFailFS{fstest.MapFS{"a": {Data: []byte("x")}}},
	}
	for name, fsys := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			a, err := assets.New(fsys, prefix, assets.Options{})
			if err == nil || a != nil || !strings.HasPrefix(err.Error(), "assets:") {
				t.Errorf("New = %v, %v, want an error that names assets", a, err)
			}
		})
	}
}

func version(t *testing.T, fsys fs.FS) string {
	t.Helper()
	return newAssets(t, fsys, assets.Options{}).Version()
}

func TestVersion(t *testing.T) {
	t.Parallel()

	base := fstest.MapFS{"a": {Data: []byte("bc")}, "d/b": {Data: []byte("x")}}
	v := version(t, base)
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(v) {
		t.Errorf("Version = %q, want 12 hex digits", v)
	}
	same := fstest.MapFS{"d/b": {Data: []byte("x")}, "a": {Data: []byte("bc")}}
	if got := version(t, same); got != v {
		t.Errorf("same files: Version = %q, want %q", got, v)
	}

	tests := map[string]fstest.MapFS{
		"a byte":   {"a": {Data: []byte("bd")}, "d/b": {Data: []byte("x")}},
		"a name":   {"c": {Data: []byte("bc")}, "d/b": {Data: []byte("x")}},
		"a length": {"a": {Data: []byte("b")}, "d/b": {Data: []byte("x")}},
		"an extra": {"a": {Data: []byte("bc")}, "d/b": {Data: []byte("x")}, "e": {}},
	}
	for name, fsys := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := version(t, fsys); got == v {
				t.Errorf("Version = %q after changing %s, want another", got, name)
			}
		})
	}
}

func TestVersionSeesTheSplitBetweenFiles(t *testing.T) {
	t.Parallel()

	one := version(t, fstest.MapFS{"a": {Data: []byte("bc")}})
	two := version(t, fstest.MapFS{"ab": {Data: []byte("c")}})
	if one == two {
		t.Errorf("Version = %q for both splits, want different ones", one)
	}
	split := version(t, fstest.MapFS{"a": {Data: []byte("b")}, "c": {Data: []byte("")}})
	join := version(t, fstest.MapFS{"a": {Data: []byte("bc")}})
	if split == join {
		t.Errorf("Version = %q for both, want different ones", split)
	}
}

func TestVersionSkipsDirectoriesAndOtherEntries(t *testing.T) {
	t.Parallel()

	plain := version(t, fstest.MapFS{"a": {Data: []byte("x")}})
	extra := version(t, fstest.MapFS{
		"a":     {Data: []byte("x")},
		"empty": {Mode: fs.ModeDir},
		"pipe":  {Mode: fs.ModeNamedPipe},
	})
	if plain != extra {
		t.Errorf("Version = %q with a directory and a pipe, want %q", extra, plain)
	}
}

func TestDevVersion(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"app.css": {Data: []byte("one")}}
	a := newAssets(t, fsys, assets.Options{Dev: true})
	if a.Version() != assets.DevVersion || assets.DevVersion != "dev" {
		t.Errorf("Version = %q, want %q", a.Version(), assets.DevVersion)
	}
	target := a.URL("app.css")
	if target != "/static/dev/app.css" {
		t.Errorf("URL = %q", target)
	}
	for _, want := range []string{"one", "changed"} {
		w := get(a, http.MethodGet, target)
		if w.Code != http.StatusOK || w.Body.String() != want {
			t.Errorf("got %d %q, want 200 %q", w.Code, w.Body, want)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("Cache-Control = %q, want no-cache", cc)
		}
		fsys["app.css"] = &fstest.MapFile{Data: []byte("changed")}
	}
}

func TestDevDoesNotReadTheFilesInNew(t *testing.T) {
	t.Parallel()

	if _, err := assets.New(errFS{}, prefix, assets.Options{Dev: true}); err != nil {
		t.Errorf("New with Dev: %v", err)
	}
}

func TestURL(t *testing.T) {
	t.Parallel()

	a := newAssets(t, fstest.MapFS{"css/app.css": {Data: []byte("x")}}, assets.Options{})
	want := prefix + a.Version() + "/css/app.css"
	if got := a.URL("css/app.css"); got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
	if got := a.URL("missing"); got != prefix+a.Version()+"/missing" {
		t.Errorf("URL of a missing file = %q", got)
	}
}

func TestURLs(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"css/b.css":     {Data: []byte("b")},
		"css/a.css":     {Data: []byte("a")},
		"css/c.js":      {Data: []byte("c")},
		"css/dir.css":   {Mode: fs.ModeDir},
		"css/sub/x.css": {Data: []byte("x")},
	}
	a := newAssets(t, fsys, assets.Options{})
	p := prefix + a.Version() + "/"
	tests := []struct {
		pattern string
		want    []string
	}{
		{"css/*.css", []string{p + "css/a.css", p + "css/b.css"}},
		{"css/*", []string{p + "css/a.css", p + "css/b.css", p + "css/c.js"}},
		{"none/*", nil},
		{"[", nil},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			t.Parallel()
			got := a.URLs(tt.pattern)
			if len(got) != len(tt.want) || (len(got) > 0 && !reflect.DeepEqual(got, tt.want)) {
				t.Errorf("URLs(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestServeHTTP(t *testing.T) {
	t.Parallel()

	a := newAssets(t, fstest.MapFS{
		"app.css":      {Data: []byte("body{}")},
		"js/app.js":    {Data: []byte("1;")},
		"blob.unknown": {Data: []byte("zz")},
	}, assets.Options{})

	tests := []struct {
		method, target, typ, body string
	}{
		{http.MethodGet, a.URL("app.css"), "text/css; charset=utf-8", "body{}"},
		{http.MethodGet, a.URL("js/app.js"), "text/javascript; charset=utf-8", "1;"},
		{http.MethodGet, a.URL("blob.unknown"), "application/octet-stream", "zz"},
		{http.MethodHead, a.URL("app.css"), "text/css; charset=utf-8", ""},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			t.Parallel()
			w := get(a, tt.method, tt.target)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			h := w.Header()
			if got := h.Get("Content-Type"); got != tt.typ {
				t.Errorf("Content-Type = %q, want %q", got, tt.typ)
			}
			if got := h.Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
				t.Errorf("Cache-Control = %q", got)
			}
			if got, want := h.Get("Content-Length"), strconv.Itoa(len(w.Body.String())); tt.method == http.MethodGet && got != want {
				t.Errorf("Content-Length = %q, want %q", got, want)
			}
			if tt.method == http.MethodGet && w.Body.String() != tt.body {
				t.Errorf("body = %q, want %q", w.Body, tt.body)
			}
			if tt.method == http.MethodHead && h.Get("Content-Length") != "6" {
				t.Errorf("HEAD Content-Length = %q, want 6", h.Get("Content-Length"))
			}
		})
	}
}

func TestServeHTTPRefusesOtherMethods(t *testing.T) {
	t.Parallel()

	a := newAssets(t, fstest.MapFS{"a.css": {Data: []byte("x")}}, assets.Options{})
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		w := get(a, m, a.URL("a.css"))
		if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
			t.Errorf("%s: got %d with Allow %q, want 405 and %q", m, w.Code, w.Header().Get("Allow"), "GET, HEAD")
		}
	}
}

func TestServeHTTPAnswers404(t *testing.T) {
	t.Parallel()

	a := newAssets(t, fstest.MapFS{
		"a.css":      {Data: []byte("x")},
		"dir/b.css":  {Data: []byte("y")},
		"empty":      {Mode: fs.ModeDir},
		"other/x.js": {Data: []byte("z")},
	}, assets.Options{})
	v := a.Version()
	tests := map[string]string{
		"another version":     prefix + "0123456789ab/a.css",
		"outside the prefix":  "/other/" + v + "/a.css",
		"the prefix alone":    "/static/",
		"no trailing slash":   "/static",
		"the version alone":   prefix + v,
		"the version and /":   prefix + v + "/",
		"a dot":               prefix + v + "/.",
		"dot dot":             prefix + v + "/../a.css",
		"dot dot inside":      prefix + v + "/dir/../a.css",
		"a leading slash":     prefix + v + "//a.css",
		"a directory":         prefix + v + "/dir",
		"an empty directory":  prefix + v + "/empty",
		"a missing file":      prefix + v + "/missing.css",
		"a missing directory": prefix + v + "/nodir/a.css",
	}
	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, m := range []string{http.MethodGet, http.MethodHead} {
				if w := get(a, m, target); w.Code != http.StatusNotFound {
					t.Errorf("%s %s = %d, want 404", m, target, w.Code)
				}
			}
		})
	}
}

func TestServeHTTPAnswers404WhenTheReadFails(t *testing.T) {
	t.Parallel()

	a := newAssets(t, readFailFS{fstest.MapFS{"a.css": {Data: []byte("x")}}}, assets.Options{Dev: true})
	if w := get(a, http.MethodGet, a.URL("a.css")); w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestContentType(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"a.css":          "text/css; charset=utf-8",
		"a.js":           "text/javascript; charset=utf-8",
		"a.mjs":          "text/javascript; charset=utf-8",
		"a.json":         "application/json",
		"a.js.map":       "application/json",
		"a.webmanifest":  "application/manifest+json",
		"a.html":         "text/html; charset=utf-8",
		"a.txt":          "text/plain; charset=utf-8",
		"a.svg":          "image/svg+xml",
		"a.png":          "image/png",
		"a.jpg":          "image/jpeg",
		"a.jpeg":         "image/jpeg",
		"a.gif":          "image/gif",
		"a.webp":         "image/webp",
		"a.ico":          "image/vnd.microsoft.icon",
		"a.woff2":        "font/woff2",
		"a.woff":         "font/woff",
		"A.CSS":          "text/css; charset=utf-8",
		"dir/A.WoFF2":    "font/woff2",
		"a.exe":          "application/octet-stream",
		"noextension":    "application/octet-stream",
		"dir.css/nofile": "application/octet-stream",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := assets.ContentType(name); got != want {
				t.Errorf("ContentType(%q) = %q, want %q", name, got, want)
			}
		})
	}
}
