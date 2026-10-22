// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package page renders the HTML of a local web server from templates, and
// answers the requests htmx sends.
//
// A program parses its templates into one Set, in which every page and
// every fragment is a block of its own name. A fragment renders the same
// in the page that holds it, in the answer to a request of htmx, and in an
// event of the page's stream (localweb/sse), because it is the same block
// of the same set. A layout places the block of the page it shows with the
// function include, which runs a block whose name is a value:
//
//	{{define "layout"}}<main>{{include .Content .}}</main>{{end}}
//	{{define "page-hosts"}}<table>{{include "hosts-table" .}}</table>{{end}}
//
// A handler answers with Respond, which renders the whole block before it
// writes anything, so that a failed render leaves the answer to the
// program:
//
//	if err := set.Respond(w, http.StatusOK, "layout", v); err != nil {
//		log.Error("render", "err", err)
//		http.Error(w, "prog ui could not render this page; its log says why", http.StatusInternalServerError)
//	}
//
// A program checks at its start that every block it names is there, with
// Has. With Options.Reload the templates are parsed anew for each render,
// from a directory the program names, for working on them without
// building the program again.
package page

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
)

// Options configures Parse.
type Options struct {
	// Funcs are the functions the templates call, besides include.
	Funcs template.FuncMap
	// Reload parses the templates anew for each render, for a program run
	// on the files of a directory, such as os.DirFS(dir). A template that
	// no longer parses fails the render.
	Reload bool
}

// Set is the parsed templates of a program. Its methods are safe for
// concurrent use.
type Set struct {
	fsys     fs.FS
	patterns []string
	o        Options
	set      *template.Template
}

// Parse parses the files of fsys that match the patterns, as
// template.ParseFS does, into one Set. It adds the function include, which
// a template calls as {{include NAME DATA}}: it renders the block NAME
// with DATA and returns its HTML, so that the name of a block can be a
// value.
func Parse(fsys fs.FS, o Options, patterns ...string) (*Set, error) {
	s := &Set{fsys: fsys, patterns: patterns, o: o}
	set, err := s.parse()
	if err != nil {
		return nil, err
	}
	s.set = set
	return s, nil
}

// parse parses the templates.
func (s *Set) parse() (*template.Template, error) {
	var set *template.Template
	funcs := template.FuncMap{}
	for name, fn := range s.o.Funcs {
		funcs[name] = fn
	}
	funcs["include"] = func(name string, data any) (template.HTML, error) {
		html, err := render(set, name, data)
		return template.HTML(html), err //nolint:gosec // G203: the output of a template, escaped in its context
	}
	set, err := template.New("").Funcs(funcs).ParseFS(s.fsys, s.patterns...)
	if err != nil {
		return nil, fmt.Errorf("page: %w", err)
	}
	return set, nil
}

// templates returns the templates to render: the parsed ones, or under
// Options.Reload, the files parsed anew.
func (s *Set) templates() (*template.Template, error) {
	if s.o.Reload {
		return s.parse()
	}
	return s.set, nil
}

// Has reports whether the block name is in the set.
func (s *Set) Has(name string) bool {
	set, err := s.templates()
	return err == nil && set.Lookup(name) != nil
}

// Render renders the block name with data and returns its HTML: the
// fragment of an answer or of an event of a stream.
func (s *Set) Render(name string, data any) (string, error) {
	set, err := s.templates()
	if err != nil {
		return "", err
	}
	return render(set, name, data)
}

// render renders the block name of set with data.
func render(set *template.Template, name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := set.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("page: render %s: %w", name, err)
	}
	return buf.String(), nil
}

// Respond renders the block name with data and answers with its HTML and
// status. When the render fails, it writes nothing and returns the error,
// so that the program answers in its own way and logs why.
func (s *Set) Respond(w http.ResponseWriter, status int, name string, data any) error {
	html, err := s.Render(name, data)
	if err != nil {
		return err
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(html)))
	w.WriteHeader(status)
	_, _ = w.Write([]byte(html)) //nolint:gosec // G705: the output of a template, escaped in its context
	return nil
}
