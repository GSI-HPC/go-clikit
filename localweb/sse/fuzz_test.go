// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package sse_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/localweb/localwebtest"
	"github.com/GSI-HPC/go-clikit/localweb/sse"
)

// Whatever name and HTML a fragment holds, a reader of the stream gets one
// event, with the fragment's name without its line breaks and its HTML, a
// carriage return written as a character reference: no fragment can end
// its event early, or start another.
func FuzzFormat(f *testing.F) {
	f.Add("runs", "<ul>\n<li>1</li>\n</ul>")
	f.Add("tasks", "a\r\nb\rc\n\ndata: x\n\nevent: y")
	f.Add("", "")
	f.Add(" x:y", ": comment\nretry: 1")
	f.Add("runs\r\ndata: x\n\nevent: y", "1")
	f.Add("\r", "\n")
	f.Fuzz(func(t *testing.T, name, data string) {
		if len(name)+len(data) > 1<<16 {
			t.Skip()
		}
		r := localwebtest.NewReader(strings.NewReader(sse.Format(sse.Event{Name: name, Data: data})))
		e, err := r.Next()
		if err != nil {
			t.Fatalf("Format(%q, %q): reading it back: %v", name, data, err)
		}
		wantName := strings.NewReplacer("\r", "", "\n", "").Replace(name)
		if want := strings.ReplaceAll(data, "\r", "&#13;"); e.Name != wantName || e.Data != want {
			t.Errorf("Format(%q, %q) reads back as %q, %q, want %q, %q", name, data, e.Name, e.Data, wantName, want)
		}
		if _, err := r.Next(); !errors.Is(err, io.EOF) {
			t.Errorf("Format(%q, %q) holds more than one event: %v", name, data, err)
		}
	})
}
