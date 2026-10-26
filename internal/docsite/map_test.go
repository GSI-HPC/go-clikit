// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package docsite

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const module = "github.com/GSI-HPC/go-clikit"

// imports is a package of the kit as the site's map draws it: the
// packages of the kit it imports, and the modules from outside the
// standard library.
type imports struct {
	Path    string   `json:"path"`
	Imports []string `json:"imports"`
	// Edges are the imports the map draws: those the package does not
	// also reach through another of its imports.
	Edges    []string `json:"edges"`
	Requires []string `json:"requires"`
}

// listPackages returns every package of the kit that the site describes,
// with what it imports, as go list tells for Linux, so that the map is the
// same whatever system the test runs on: the root, which holds no API,
// and internal ones are left out.
func listPackages(t *testing.T) []imports {
	t.Helper()
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []imports
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		rel, ok := strings.CutPrefix(p.ImportPath, module+"/")
		if !ok || strings.HasPrefix(rel, "internal/") {
			continue
		}
		pkg := imports{Path: rel, Imports: []string{}, Requires: []string{}}
		for _, imp := range p.Imports {
			if kit, ok := strings.CutPrefix(imp, module+"/"); ok {
				pkg.Imports = append(pkg.Imports, kit)
			} else if strings.Contains(strings.SplitN(imp, "/", 2)[0], ".") {
				pkg.Requires = append(pkg.Requires, imp)
			}
		}
		pkgs = append(pkgs, pkg)
	}
	byPath := map[string][]string{}
	for _, p := range pkgs {
		byPath[p.Path] = p.Imports
	}
	var reaches func(from, to string) bool
	reaches = func(from, to string) bool {
		for _, imp := range byPath[from] {
			if imp == to || reaches(imp, to) {
				return true
			}
		}
		return false
	}
	for i, p := range pkgs {
		pkgs[i].Edges = []string{}
		for _, imp := range p.Imports {
			through := false
			for _, other := range p.Imports {
				if other != imp && reaches(other, imp) {
					through = true
				}
			}
			if !through {
				pkgs[i].Edges = append(pkgs[i].Edges, imp)
			}
		}
	}
	return pkgs
}

// TestPackageMap holds the map of the site to the imports go list
// reports, and holds the site to one entry and one page for each
// package.
func TestPackageMap(t *testing.T) {
	pkgs := listPackages(t)
	b, err := json.MarshalIndent(pkgs, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "data/imports.json", append(b, '\n'))

	raw, err := os.ReadFile(filepath.Join(root, "site", "data", "packages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		Path  string `json:"path"`
		Group string `json:"group"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("site/data/packages.json: %v", err)
	}
	var want, got []string
	for _, p := range pkgs {
		want = append(want, p.Path)
	}
	groups := map[string]bool{"progress": true, "command": true, "files": true, "web": true}
	for _, e := range entries {
		got = append(got, e.Path)
		if !groups[e.Group] {
			t.Errorf("site/data/packages.json: %s is in group %q, which the map does not draw", e.Path, e.Group)
		}
		page := filepath.Join(root, "site", "content", "packages", path.Base(e.Path)+".md")
		text, err := os.ReadFile(page)
		if err != nil {
			t.Errorf("%s has no page: %v", e.Path, err)
			continue
		}
		if !bytes.Contains(text, []byte("\nimport: "+e.Path+"\n")) {
			t.Errorf("%s does not say import: %s in its front matter", page, e.Path)
		}
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("site/data/packages.json lists %v, the module holds %v", got, want)
	}
}
