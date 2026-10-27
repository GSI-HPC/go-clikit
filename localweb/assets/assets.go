// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package assets serves the static files of a local web page, such as its
// CSS, its scripts and its fonts, at addresses that change when a file
// does, so that a browser may keep each one for good.
//
// A program embeds its files, so that it works offline and loads nothing
// from elsewhere, and serves them under a prefix:
//
//	//go:embed static
//	var static embed.FS
//
//	files, _ := fs.Sub(static, "static")
//	a, err := assets.New(files, "/static/", assets.Options{})
//	mux.Handle("GET /static/", a)
//
// A file is served at PREFIX/VERSION/NAME, where VERSION is a hash of
// every file: a new build of the program serves its files under a new
// address, and a page links to them with URL, such as from a template
// function:
//
//	<link rel="stylesheet" href="{{asset "app.css"}}">
//
// With Options.Dev the files are read anew on each request, from a
// directory the program names, and browsers check them each time: for
// working on the files without building the program again.
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// DevVersion is the VERSION of every address under Options.Dev.
const DevVersion = "dev"

// The Cache-Control of a file: one with a hash in its address never
// changes, and one under Options.Dev may change at any time.
const (
	cacheImmutable = "private, max-age=31536000, immutable"
	cacheDev       = "no-cache"
)

// Options configures New.
type Options struct {
	// Dev reads each file anew on each request, under the version
	// DevVersion, and has browsers check it each time, for a program run
	// on the files of a directory, such as os.DirFS(dir).
	Dev bool
}

// Set is the static files of a page, an http.Handler that serves them.
// Its methods are safe for concurrent use.
type Set struct {
	fsys    fs.FS
	prefix  string
	version string
	dev     bool
}

// New returns the files of fsys, served under prefix, a path that starts
// and ends with a slash, such as "/static/". Unless o.Dev is set, it reads
// every file once to make the version, and an error of fsys is its error.
func New(fsys fs.FS, prefix string, o Options) (*Set, error) {
	if !strings.HasPrefix(prefix, "/") || !strings.HasSuffix(prefix, "/") {
		return nil, fmt.Errorf("assets: the prefix %q does not start and end with a slash", prefix)
	}
	a := &Set{fsys: fsys, prefix: prefix, version: DevVersion, dev: o.Dev}
	if o.Dev {
		return a, nil
	}
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		// A name ends at its NUL, and a file at its length, so that no
		// two sets of files hash the same.
		h.Write([]byte(name + "\x00" + strconv.Itoa(len(data)) + "\x00"))
		h.Write(data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assets: %w", err)
	}
	a.version = hex.EncodeToString(h.Sum(nil))[:12]
	return a, nil
}

// Version returns the version in the addresses of the files: twelve hex
// digits of their hash, or DevVersion.
func (a *Set) Version() string { return a.version }

// URL returns the address of the file name, such as
// "/static/0123456789ab/app.css" for "app.css". It does not look whether
// the file is there.
func (a *Set) URL(name string) string {
	return a.prefix + a.version + "/" + name
}

// URLs returns the addresses of the files whose names match pattern, as
// fs.Glob matches them, in the order of their names, such as every
// "css/*.css" of a page. A pattern that is malformed matches nothing.
func (a *Set) URLs(pattern string) []string {
	names, _ := fs.Glob(a.fsys, pattern)
	urls := make([]string, 0, len(names))
	for _, name := range names {
		if info, err := fs.Stat(a.fsys, name); err == nil && info.Mode().IsRegular() {
			urls = append(urls, a.URL(name))
		}
	}
	return urls
}

// ServeHTTP answers a GET or HEAD of PREFIX/VERSION/NAME with the file
// NAME. It answers 404 for an address of another version, which an older
// page asks for, a name that is not a regular file, and an address outside
// the prefix; and 405 for any other method. A file is served with a
// Content-Type from a fixed list, never from the host's own, and
// application/octet-stream for an extension the list does not have.
func (a *Set) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, a.prefix)
	version, name, _ := strings.Cut(rest, "/")
	if !ok || version != a.version || !fs.ValidPath(name) || name == "." {
		http.NotFound(w, r)
		return
	}
	data, err := a.read(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ContentType(name))
	h.Set("Content-Length", strconv.Itoa(len(data)))
	if a.dev {
		h.Set("Cache-Control", cacheDev)
	} else {
		h.Set("Cache-Control", cacheImmutable)
	}
	_, _ = w.Write(data) //nolint:gosec // G705: a file of the program's, served with its own type
}

// read returns the content of the regular file name.
func (a *Set) read(name string) ([]byte, error) {
	info, err := fs.Stat(a.fsys, name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("no regular file")
	}
	return fs.ReadFile(a.fsys, name)
}

// ContentType returns the media type ServeHTTP sends for the file name, by
// its extension, in any case.
func ContentType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".json", ".map":
		return "application/json"
	case ".webmanifest":
		return "application/manifest+json"
	case ".html":
		return "text/html; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".ico":
		return "image/vnd.microsoft.icon"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	}
	return "application/octet-stream"
}
