// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package docsite

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLogLines holds every line of the event log a page shows to the
// golden log, which the tests of progress pin, so that a page never shows
// a line the kit no longer writes.
func TestLogLines(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join(root, "progress", "testdata", "log-v1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]bool{}
	for line := range strings.SplitSeq(string(golden), "\n") {
		lines[line] = true
	}
	checked := 0
	err = filepath.WalkDir(filepath.Join(root, "site", "content"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for line := range bytes.SplitSeq(text, []byte("\n")) {
			if !bytes.HasPrefix(line, []byte(`{"v":1,`)) {
				continue
			}
			checked++
			if !lines[string(line)] {
				t.Errorf("%s shows a line that is not in progress/testdata/log-v1.jsonl:\n%s", path, line)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Error("no page shows a line of the event log; the test checks nothing")
	}
}
