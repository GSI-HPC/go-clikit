// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package htmx_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/localweb/htmx"
)

func TestFilesAreTheUpstreamReleases(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, sum string }{
		{htmx.Script, "d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717"},
		{htmx.SSE, "3b5992a541619babefc4c169505af474df5c3039da51e59b96ccf9241ecd61d2"},
		{htmx.Morph, "a6437e55b1b6a07bc421f0d230266a39399b6826c6ed19e0ed9c63b707444a5f"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := fs.ReadFile(htmx.Files(), tt.name)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != tt.sum {
				t.Errorf("sha256 of %s = %s, want %s: the bytes must be the upstream release's, "+
					"and a new version changes Version and this sum together", tt.name, got, tt.sum)
			}
		})
	}
}

func TestScriptHoldsItsVersion(t *testing.T) {
	t.Parallel()

	data, err := fs.ReadFile(htmx.Files(), htmx.Script)
	if err != nil {
		t.Fatal(err)
	}
	if want := `version:"` + htmx.Version + `"`; !strings.Contains(string(data), want) {
		t.Errorf("%s does not contain %s: Version and the file must change together", htmx.Script, want)
	}
}

func TestFilesHoldThreeFilesOnly(t *testing.T) {
	t.Parallel()

	entries, err := fs.ReadDir(htmx.Files(), ".")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	want := []string{htmx.Morph, htmx.SSE, htmx.Script}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("Files = %q, want %q", got, want)
	}
}

func TestConfig(t *testing.T) {
	t.Parallel()

	var got map[string]any
	if err := json.Unmarshal([]byte(htmx.Config), &got); err != nil {
		t.Fatalf("Config is not JSON: %v", err)
	}
	want := map[string]any{
		"allowEval":                 false,
		"allowScriptTags":           false,
		"includeIndicatorStyles":    false,
		"selfRequestsOnly":          true,
		"historyCacheSize":          float64(0),
		"historyRestoreAsHxRequest": false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Config = %v, want %v (keys %v)", got, want, slices.Sorted(maps.Keys(want)))
	}
}

// Every key of Config is a setting of the htmx in Files, so that a new
// htmx that drops one fails here and not in a browser.
func TestConfigKeysAreSettingsOfTheScript(t *testing.T) {
	t.Parallel()

	data, err := fs.ReadFile(htmx.Files(), htmx.Script)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(htmx.Config), &config); err != nil {
		t.Fatalf("Config is not JSON: %v", err)
	}
	for key := range config {
		if !strings.Contains(string(data), ","+key+":") && !strings.Contains(string(data), "{"+key+":") {
			t.Errorf("%s has no setting %s", htmx.Script, key)
		}
	}
}
