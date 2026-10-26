// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package docsite

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	// {{< api "progress" "Span.End" >}}, with or without a text after it.
	apiRef = regexp.MustCompile(`\{\{<\s*api\s+"([^"]*)"\s+"([^"]*)"`)
	// {{< example "progress/display" "ExampleNewTree" >}}, or a helper of
	// the examples, such as {{< example "progress" "reset" >}}.
	exampleRef = regexp.MustCompile(`\{\{<\s*example\s+"([^"]*)"\s+"([^"]*)"`)
	// A link written out to a name on pkg.go.dev.
	urlRef = regexp.MustCompile(`pkg\.go\.dev/github\.com/GSI-HPC/go-clikit/([a-z/]+)#([A-Za-z0-9_.]+)`)
)

// names returns the names pkg.go.dev gives an anchor in the package at
// dir: its exported functions, types, constants and variables, the
// methods and fields of its types as Type.Name, and, with tests, the
// Example functions of its tests.
func names(t *testing.T, dir string, tests bool) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	got := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") != tests {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					got[d.Name.Name] = true
					continue
				}
				recv := d.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				if idx, ok := recv.(*ast.IndexExpr); ok {
					recv = idx.X
				}
				if idx, ok := recv.(*ast.IndexListExpr); ok {
					recv = idx.X
				}
				if id, ok := recv.(*ast.Ident); ok {
					got[id.Name+"."+d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						got[s.Name.Name] = true
						if st, ok := s.Type.(*ast.StructType); ok {
							for _, fld := range st.Fields.List {
								for _, n := range fld.Names {
									got[s.Name.Name+"."+n.Name] = true
								}
							}
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							got[n.Name] = true
						}
					}
				}
			}
		}
	}
	return got
}

// TestLinks holds every name the site links to on pkg.go.dev, and every
// example it shows, to a name the code has.
func TestLinks(t *testing.T) {
	content := filepath.Join(root, "site", "content")
	cache := map[string]map[string]bool{}
	lookup := func(pkg string, tests bool) map[string]bool {
		key := pkg + map[bool]string{true: " tests"}[tests]
		if n, ok := cache[key]; ok {
			return n
		}
		n := names(t, filepath.Join(root, filepath.FromSlash(pkg)), tests)
		cache[key] = n
		return n
	}
	checked := 0
	err := filepath.WalkDir(content, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		text := string(b)
		rel, _ := filepath.Rel(root, p)
		for _, m := range append(apiRef.FindAllStringSubmatch(text, -1), urlRef.FindAllStringSubmatch(text, -1)...) {
			checked++
			n := lookup(m[1], false)
			if n == nil {
				t.Errorf("%s: no package %s", rel, m[1])
			} else if m[2] != "" && !n[m[2]] {
				t.Errorf("%s: %s has no name %s", rel, m[1], m[2])
			}
		}
		for _, m := range exampleRef.FindAllStringSubmatch(text, -1) {
			checked++
			if !lookup(m[1], true)[m[2]] {
				t.Errorf("%s: the tests of %s have no function %s", rel, m[1], m[2])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Error("the site links to no name: the patterns of this test no longer match its pages")
	}
}
