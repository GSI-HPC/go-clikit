// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package docsite

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the site's generated files")

// root is the module's directory, two up from this package's.
const root = "../.."

// golden compares got with the generated file at name, under site/, or
// writes it there with -update.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(root, "site", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v: run go test ./internal/docsite -update", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("site/%s is not what the code makes now: run go test ./internal/docsite -update", name)
	}
}
