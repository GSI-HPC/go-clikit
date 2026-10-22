// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliout_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GSI-HPC/go-clikit/cliout"
)

// byPointer marshals itself through its pointer, which encoding/json calls
// for an element of a slice, since it can address one.
type byPointer struct{ N int }

func (p *byPointer) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `{"pointer":%d}`, p.N), nil
}

// byValue marshals itself through its value.
type byValue struct{ N int }

func (v byValue) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `{"value":[%d,{}]}`, v.N), nil
}

// asText is a slice that marshals itself whole.
type asText []int

func (a asText) MarshalText() ([]byte, error) { return fmt.Appendf(nil, "%d items", len(a)), nil }

// result is what a command on a target returns.
type result struct {
	Target string `json:"target"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// whole is what encoding v whole writes, without HTML escapes.
func whole(t *testing.T, v any) string {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// A slice is written an element at a time, and what is written is, byte for
// byte, what encoding the whole slice wrote: nested slices and maps, empty
// ones, elements that marshal themselves through their pointer or their
// value, and the values that are no slice of elements, a []byte, an empty
// or nil slice, a slice that marshals itself.
func TestWriteJSONAsTheWholeWouldBe(t *testing.T) {
	results := []*result{{Target: "exe0001", Output: "<b>&\n"}, nil, {Target: "exe0002", Error: "exit 1"}}
	for name, v := range map[string]any{
		"results":            results,
		"one result":         results[:1],
		"strings":            []string{"<a>", "&", "\u2028"},
		"anything":           []any{map[string]any{"a": []int{}, "b": map[string]any{}}, nil, []any{1, "x", []int{2}}, 3.5},
		"empty fields":       []struct{ A, B []int }{{nil, []int{}}, {}},
		"pointer marshalers": []byPointer{{1}, {2}},
		"value marshalers":   []byValue{{1}, {2}},
		"raw messages":       []json.RawMessage{json.RawMessage(`{"a":[1,2]}`), json.RawMessage(`[]`)},
		"nested slices":      [][]int{{1, 2}, {}, nil},
		"bytes":              []byte("bytes"),
		"empty":              []int{},
		"nil":                []int(nil),
		"a slice as text":    asText{1, 2},
		"a map":              map[string][]int{"a": {1}},
		"a string":           "text",
	} {
		var got bytes.Buffer
		if err := cliout.WriteJSON(&got, v); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if want := whole(t, v); got.String() != want {
			t.Errorf("%s:\n%s\nwant:\n%s", name, got.String(), want)
		}
	}
}

func TestWriteJSONWritesNoHTMLEscapes(t *testing.T) {
	var got bytes.Buffer
	if err := cliout.WriteJSON(&got, []string{"<b>&</b>"}); err != nil {
		t.Fatal(err)
	}
	if want := "[\n  \"<b>&</b>\"\n]\n"; got.String() != want {
		t.Errorf("wrote %q, want %q", got.String(), want)
	}
}

// A string cannot act on the terminal, and decodes to what it was.
func TestWriteJSONEscapesWhatTermtextEscapes(t *testing.T) {
	text := "ok\x7f \u009b2J \u202eevil\u202c \x1b[1A é ✅"
	for name, v := range map[string]any{
		"a string":  text,
		"a slice":   []string{text, "plain"},
		"a map":     map[string]string{"\u009bkey": text},
		"raw bytes": json.RawMessage("\"a\xffb\""),
	} {
		var got bytes.Buffer
		if err := cliout.WriteJSON(&got, v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.ContainsAny(got.String(), "\x7f\u009b\u202e\u202c\x1b") || !utf8.ValidString(got.String()) {
			t.Errorf("%s: wrote %q, with what acts on a terminal", name, got.String())
		}
		if !strings.Contains(got.String(), "é ✅") && name != "raw bytes" {
			t.Errorf("%s: wrote %q, without the runes that need no escape", name, got.String())
		}
		if name == "raw bytes" {
			if want := "\"a\\ufffdb\"\n"; got.String() != want {
				t.Errorf("%s: wrote %q, want %q", name, got.String(), want)
			}
			continue
		}
		back := map[string]any{}
		wrapped := `{"v":` + got.String() + `}`
		if err := json.Unmarshal([]byte(wrapped), &back); err != nil {
			t.Fatalf("%s: %v in %s", name, err, wrapped)
		}
		if want := whole(t, map[string]any{"v": v}); whole(t, back) != want {
			t.Errorf("%s: decodes to %v, want %v", name, back, v)
		}
	}
}

// marked marshals itself into a large string, and records that it did.
type marked struct {
	i    int
	seen *[]string
}

func (m marked) MarshalJSON() ([]byte, error) {
	*m.seen = append(*m.seen, fmt.Sprintf("marshal %d", m.i))
	return json.Marshal(strings.Repeat("x", 1<<16))
}

// recording records each write, in the order of the work.
type recording struct{ seen *[]string }

func (r recording) Write(p []byte) (int, error) {
	*r.seen = append(*r.seen, fmt.Sprintf("write %d bytes", len(p)))
	return len(p), nil
}

// An element is written before the next is encoded, so the JSON of a slice
// is never held whole.
func TestWriteJSONWritesAsItEncodes(t *testing.T) {
	var seen []string
	list := make([]marked, 3)
	for i := range list {
		list[i] = marked{i, &seen}
	}
	if err := cliout.WriteJSON(recording{&seen}, list); err != nil {
		t.Fatal(err)
	}
	want := []string{"write 2 bytes", "marshal 0", "write 65542 bytes", "marshal 1", "write 65542 bytes", "marshal 2", "write 65541 bytes", "write 2 bytes"}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Errorf("the work went:\n%s\nwant:\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
	}
}

// failingAfter lets n writes through, and fails those after them.
type failingAfter struct{ n *int }

func (f failingAfter) Write(p []byte) (int, error) {
	if *f.n == 0 {
		return 0, errWrite
	}
	*f.n--
	return len(p), nil
}

// unencodable cannot be encoded.
type unencodable struct{}

func (unencodable) MarshalJSON() ([]byte, error) { return nil, errWrite }

func TestWriteJSONFails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		v      any
		writes int
	}{
		{"a value that cannot be encoded", unencodable{}, 9},
		{"an element that cannot be encoded", []unencodable{{}}, 9},
		{"the first write", "text", 0},
		{"the opening bracket", []int{1, 2}, 0},
		{"an element", []int{1, 2}, 1},
		{"the closing bracket", []int{1, 2}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.writes
			if err := cliout.WriteJSON(failingAfter{&n}, tc.v); !errors.Is(err, errWrite) {
				t.Errorf("error %v, want %v", err, errWrite)
			}
		})
	}
}
