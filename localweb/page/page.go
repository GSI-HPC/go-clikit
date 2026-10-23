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
	"sync"
)

// MaxIncludeDepth is how deep include may nest in one render. A block
// that includes itself, or blocks that include each other, fail the
// render with an error past it, as each include starts the depth limit of
// text/template again, and a cycle would otherwise end the process with a
// stack overflow (decision 41).
const MaxIncludeDepth = 100

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
	// set is the templates as parsed, which no render executes: each
	// render executes a clone of them from runs, which counts its
	// includes.
	set  *template.Template
	runs sync.Pool
}

// run is one render: the templates it executes, how deep include is
// nested in it, and the error of an include past MaxIncludeDepth.
type run struct {
	set   *template.Template
	depth int
	err   error
}

// Parse parses the files of fsys that match the patterns, as
// template.ParseFS does, into one Set. It adds the function include, which
// a template calls as {{include NAME DATA}}: it renders the block NAME
// with DATA and returns its HTML, so that the name of a block can be a
// value. include nests at most MaxIncludeDepth deep in one render.
func Parse(fsys fs.FS, o Options, patterns ...string) (*Set, error) {
	s := &Set{fsys: fsys, patterns: patterns, o: o}
	set, err := s.parse(&run{})
	if err != nil {
		return nil, err
	}
	s.set = set
	s.runs.New = func() any {
		r := &run{}
		r.set = template.Must(s.set.Clone()).Funcs(template.FuncMap{"include": r.include})
		return r
	}
	return s, nil
}

// parse parses the templates, with the include of r.
func (s *Set) parse(r *run) (*template.Template, error) {
	funcs := template.FuncMap{}
	for name, fn := range s.o.Funcs {
		funcs[name] = fn
	}
	funcs["include"] = r.include
	set, err := template.New("").Funcs(funcs).ParseFS(s.fsys, s.patterns...)
	if err != nil {
		return nil, fmt.Errorf("page: %w", err)
	}
	return set, nil
}

// include is the function include of the templates r executes.
func (r *run) include(name string, data any) (template.HTML, error) {
	if r.depth == MaxIncludeDepth {
		r.err = fmt.Errorf("include %s: nested more than %d deep", name, MaxIncludeDepth)
		return "", r.err
	}
	r.depth++
	html, err := r.render(name, data)
	r.depth--
	return template.HTML(html), err //nolint:gosec // G203: the output of a template, escaped in its context
}

// render renders the block name with data.
func (r *run) render(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := r.set.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("page: render %s: %w", name, err)
	}
	return buf.String(), nil
}

// Has reports whether the block name is in the set.
func (s *Set) Has(name string) bool {
	set := s.set
	if s.o.Reload {
		var err error
		if set, err = s.parse(&run{}); err != nil {
			return false
		}
	}
	return set.Lookup(name) != nil
}

// Render renders the block name with data and returns its HTML: the
// fragment of an answer or of an event of a stream. A render whose
// includes nest past MaxIncludeDepth fails with an error that names the
// block of the include that went too deep.
func (s *Set) Render(name string, data any) (string, error) {
	var r *run
	if s.o.Reload {
		r = &run{}
		set, err := s.parse(r)
		if err != nil {
			return "", err
		}
		r.set = set
	} else {
		r = s.runs.Get().(*run)
		r.depth, r.err = 0, nil
		defer s.runs.Put(r)
	}
	html, err := r.render(name, data)
	if r.err != nil {
		return "", fmt.Errorf("page: render %s: %w", name, r.err)
	}
	return html, err
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
