// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliout

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// WriteJSON writes v to w as JSON, as encoding/json encodes it, indented by
// two spaces and ended by a newline, with two differences:
//
//   - <, > and & are written as they are, not as <, > and
//     &: the output is read in a terminal or by a program, never put
//     into HTML.
//   - A rune that termtext escapes and encoding/json writes as it is, such
//     as DEL, a C1 control or a bidirectional control, is written as a JSON
//     escape, \u007f, \u009b or ‮, so that a string from elsewhere
//     cannot act on the terminal the JSON is read in. A program that
//     decodes the JSON gets the same strings.
//
// A slice is written an element at a time, each encoded on its own, so
// that the output of a command on many targets is never held whole twice
// over, encoded and indented; the bytes are those of the slice encoded
// whole. The elements written before an error stay written.
func WriteJSON(w io.Writer, v any) error {
	list := reflect.ValueOf(v)
	if !elementwise(list) {
		var b bytes.Buffer
		enc := encoder(&b, "")
		if err := enc.Encode(v); err != nil {
			return err
		}
		_, err := w.Write(escapeJSON(b.Bytes()))
		return err
	}
	if _, err := io.WriteString(w, "[\n"); err != nil {
		return err
	}
	var b bytes.Buffer
	enc := encoder(&b, "  ")
	for i := range list.Len() {
		b.Reset()
		b.WriteString("  ")
		// An element of a slice can be addressed, so a MarshalJSON of its
		// pointer is what encodes it in the slice; it is handed over by
		// address so that it is here too.
		if err := enc.Encode(list.Index(i).Addr().Interface()); err != nil {
			return err
		}
		// Encode ends the element with a newline, which follows the comma
		// that parts it from the next.
		if i < list.Len()-1 {
			b.Truncate(b.Len() - 1)
			b.WriteString(",\n")
		}
		if _, err := w.Write(escapeJSON(b.Bytes())); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "]\n")
	return err
}

// encoder returns the encoder of WriteJSON, writing to b with every line
// after the first indented by prefix.
func encoder(b *bytes.Buffer, prefix string) *json.Encoder {
	enc := json.NewEncoder(b)
	enc.SetIndent(prefix, "  ")
	enc.SetEscapeHTML(false)
	return enc
}

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

// elementwise reports whether v is a slice WriteJSON writes an element at a
// time: one with elements, that is no []byte, which is a string in JSON,
// and that does not marshal itself.
func elementwise(v reflect.Value) bool {
	if v.Kind() != reflect.Slice || v.Len() == 0 || v.Type().Elem().Kind() == reflect.Uint8 {
		return false
	}
	t := v.Type()
	for _, m := range []reflect.Type{jsonMarshaler, textMarshaler} {
		if t.Implements(m) || reflect.PointerTo(t).Implements(m) {
			return false
		}
	}
	return true
}

// escapeJSON returns the JSON in b with every rune termtext escapes written
// as a JSON escape. encoding/json escapes every C0 control in a string, and
// the newlines and spaces between its values are none that termtext
// escapes in lines, so only DEL and runes beyond ASCII are asked about.
func escapeJSON(b []byte) []byte {
	var out []byte
	done := 0
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == 0x7f || r >= utf8.RuneSelf {
			if s := b[i : i+size]; termtext.EscapeLines(string(s)) != string(s) {
				out = append(out, b[done:i]...)
				out = appendEscape(out, r)
				done = i + size
			}
		}
		i += size
	}
	if out == nil {
		return b
	}
	return append(out, b[done:]...)
}

// appendEscape appends the JSON escape of r: \u and four digits, or two of
// them, the halves of a surrogate pair, for a rune beyond the Basic
// Multilingual Plane.
func appendEscape(b []byte, r rune) []byte {
	if r1, r2 := utf16.EncodeRune(r); r1 != utf8.RuneError {
		return fmt.Appendf(b, `\u%04x\u%04x`, r1, r2)
	}
	return fmt.Appendf(b, `\u%04x`, r)
}
