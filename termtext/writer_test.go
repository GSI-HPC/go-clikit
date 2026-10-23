// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package termtext_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// held is how many bytes of a line the escape writer holds back at most.
const held = 64 << 10

// writeAll writes in to a new escape writer in pieces of size bytes, closes
// it and returns what it wrote.
func writeAll(t *testing.T, in string, size int) string {
	t.Helper()
	var out bytes.Buffer
	w := termtext.NewEscapeWriter(&out)
	for rest := []byte(in); len(rest) > 0; {
		n := min(size, len(rest))
		if got, err := w.Write(rest[:n]); got != n || err != nil {
			t.Fatalf("Write of %d bytes = %d, %v", n, got, err)
		}
		rest = rest[n:]
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	return out.String()
}

// The writer passes lines on escaped, keeps newline and tab, and ends the
// line that did not end when it is closed.
func TestTheEscapeWriterEscapesLineByLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, in, want string
	}{
		{"nothing", "", ""},
		{"plain lines", "one\ntwo\n", "one\ntwo\n"},
		{"newline and tab survive", "a\tb\n\n", "a\tb\n\n"},
		{"a line that did not end", "one\ntwo", "one\ntwo\n"},
		{"an empty line", "\n", "\n"},
		{"controls are escaped", "a\x1b]0;x\x07\nb\x00\n", "a\\x1b]0;x\\x07\nb\\x00\n"},
		{"a carriage return is escaped", "10%\r20%\ndone\n", "10%\\r20%\ndone\n"},
		{"a CRLF line end is kept", "10%\r20%\r\ndone\r\n", "10%\\r20%\r\ndone\r\n"},
		{"a carriage return the stream ends in is escaped", "one\r\ntwo\r", "one\r\ntwo\\r\n"},
		{"bidirectional controls", "\u202eexe\n", "\\u202eexe\n"},
		{"bytes that are not UTF-8", "a\x9bb\xff\n", "a\\x9bb\\xff\n"},
		{"a rune that is cut by the end", "a\xe2\x82", "a\\xe2\\x82\n"},
		{"wide text", "ノード\n", "ノード\n"},
	} {
		if got := writeAll(t, tc.in, len(tc.in)+1); got != tc.want {
			t.Errorf("%s: writing %q gave %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

// What is written does not depend on how the input was cut into writes: a
// rune that comes in two writes is whole, and one the input does not hold
// is not made up.
func TestTheEscapeWriterWritesTheSameWhateverTheWrites(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"one\ntwo\nthree",
		"ノード\n\u202e\x1b[2J\r\nGrüße 😀 end\n",
		"bad \xe2\x82 bytes \xf0\x9f\x98 and \x80\x80\x80 more\n",
		"crlf\r\nlines\r\r\n\r\r\n\rand\ra\r",
		strings.Repeat("a", held) + "\r\ndone",
		strings.Repeat("a", held-1) + "\r\ndone\r",
		strings.Repeat("a", held) + "\r\xe2\x82\ndone",
		strings.Repeat("a", held) + "\rb\r",
		strings.Repeat("é", held),
		strings.Repeat("ノ", held) + "\n" + strings.Repeat("😀", held/2) + "x\ny",
		strings.Repeat("a", held+1),
		strings.Repeat("a", 2*held+7) + "\x80\x80\x80\x80",
		strings.Repeat("a", held-2) + "\x80\x80\x80\ndone",
		strings.Repeat("😀", held) + "\xf0\x9f\x98",
	} {
		want := termtext.EscapeLines(in)
		if in != "" && !strings.HasSuffix(in, "\n") {
			want += "\n"
		}
		for _, size := range []int{1, 2, 3, 5, 7, 100, held - 1, held, held + 1, 1 << 20} {
			if got := writeAll(t, in, size); got != want {
				t.Errorf("writing %d bytes in pieces of %d gave %d bytes that are not the %d bytes wanted",
					len(in), size, len(got), len(want))
			}
		}
	}
}

// A line that goes on without a newline is written out, so that no more
// than 64 KiB of it is held back, however much is written at once.
func TestTheEscapeWriterHoldsBackNoMoreThanItsBound(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, in string
	}{
		{"one write", strings.Repeat("a", 3*held)},
		{"wide runes", strings.Repeat("ノ", held)},
		{"a rune that is cut", strings.Repeat("é", held)},
	} {
		var out bytes.Buffer
		w := termtext.NewEscapeWriter(&out)
		if _, err := w.Write([]byte(tc.in)); err != nil {
			t.Fatalf("%s: Write = %v", tc.name, err)
		}
		if out.Len() < len(tc.in)-held {
			t.Errorf("%s: wrote %d of %d bytes, so more than %d were held back", tc.name, out.Len(), len(tc.in), held)
		}
		if bytes.ContainsRune(out.Bytes(), '\n') {
			t.Errorf("%s: wrote a newline before the line ended", tc.name)
		}
	}
}

// A line cut at the bound that ends right there still gets its newline from
// Close.
func TestTheEscapeWriterEndsALineThatWasWrittenOutWhole(t *testing.T) {
	t.Parallel()
	in := strings.Repeat("a", held+1)
	var out bytes.Buffer
	w := termtext.NewEscapeWriter(&out)
	if _, err := w.Write([]byte(in)); err != nil {
		t.Fatalf("Write = %v", err)
	}
	if out.String() != in {
		t.Fatalf("wrote %d bytes before Close, want %d", out.Len(), len(in))
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if out.String() != in+"\n" {
		t.Errorf("Close wrote %d bytes more, want a newline", out.Len()-len(in))
	}
}

// A writer that fails by its writes: it records what it was given and fails
// from the nth call on.
type failingWriter struct {
	calls, failFrom int
	got             []string
}

func (f *failingWriter) Write(p []byte) (int, error) {
	f.calls++
	f.got = append(f.got, string(p))
	if f.calls >= f.failFrom {
		return 0, fmt.Errorf("write %d: %w", f.calls, io.ErrShortWrite)
	}
	return len(p), nil
}

// Write consumes all it is given, returns the first error of w, and goes on
// with the lines after the one that failed; so does Close.
func TestTheEscapeWriterReturnsTheFirstErrorOfTheWriter(t *testing.T) {
	t.Parallel()
	fw := &failingWriter{failFrom: 2}
	w := termtext.NewEscapeWriter(fw)
	in := "one\ntwo\nthree\nfour"
	n, err := w.Write([]byte(in))
	if n != len(in) || !errors.Is(err, io.ErrShortWrite) || !strings.Contains(err.Error(), "write 2") {
		t.Fatalf("Write = %d, %v, want %d and the error of the second write", n, err, len(in))
	}
	if want := []string{"one\n", "two\n", "three\n"}; strings.Join(fw.got, "|") != strings.Join(want, "|") {
		t.Errorf("w was written %q, want %q", fw.got, want)
	}
	if err := w.Close(); !errors.Is(err, io.ErrShortWrite) || !strings.Contains(err.Error(), "write 4") {
		t.Errorf("Close = %v, want the error of the fourth write", err)
	}
	if fw.got[3] != "four\n" {
		t.Errorf("Close wrote %q, want %q", fw.got[3], "four\n")
	}
}

// A line cut at the bound reports the error of w too, and the writer goes on.
func TestTheEscapeWriterReturnsTheErrorOfAPartialWrite(t *testing.T) {
	t.Parallel()
	fw := &failingWriter{failFrom: 1}
	w := termtext.NewEscapeWriter(fw)
	in := strings.Repeat("a", held+1)
	if n, err := w.Write([]byte(in)); n != len(in) || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write = %d, %v, want %d and an error", n, err, len(in))
	}
	if n, err := w.Write([]byte("b\n")); n != 2 || !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("the next Write = %d, %v, want 2 and an error", n, err)
	}
	if fw.got[1] != "b\n" {
		t.Errorf("the next Write wrote %q, want %q", fw.got[1], "b\n")
	}
}

// Close does not close w, writes nothing when no line is open, and does
// nothing the second time; Write after Close writes nothing.
func TestTheEscapeWriterCloses(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	w := termtext.NewEscapeWriter(&out)
	if _, err := w.Write([]byte("done\n")); err != nil {
		t.Fatalf("Write = %v", err)
	}
	if err := w.Close(); err != nil || out.String() != "done\n" {
		t.Errorf("Close = %v after writing %q, want nothing more", err, out.String())
	}
	if err := w.Close(); err != nil || out.String() != "done\n" {
		t.Errorf("the second Close = %v after writing %q, want nothing more", err, out.String())
	}
	if n, err := w.Write([]byte("late\n")); n != 0 || !errors.Is(err, io.ErrClosedPipe) || out.String() != "done\n" {
		t.Errorf("Write after Close = %d, %v after writing %q, want 0, io.ErrClosedPipe and nothing", n, err, out.String())
	}

	fw := &failingWriter{failFrom: 99}
	w = termtext.NewEscapeWriter(fw)
	if _, err := w.Write([]byte("open")); err != nil {
		t.Fatalf("Write = %v", err)
	}
	if err := w.Close(); err != nil || len(fw.got) != 1 || fw.got[0] != "open\n" {
		t.Errorf("Close = %v, wrote %q, want %q", err, fw.got, "open\n")
	}
	if err := w.Close(); err != nil || len(fw.got) != 1 {
		t.Errorf("the second Close = %v, wrote %q, want nothing more", err, fw.got)
	}
}

// Writers that run together write whole lines, one at a time, to w, which
// needs no lock of its own; the race detector checks that.
func TestTheEscapeWriterIsSafeForConcurrentUse(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	w := termtext.NewEscapeWriter(&out)
	const writers, lines = 8, 50
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range lines {
				if _, err := fmt.Fprintf(w, "writer %d\x1b says hello\n", i); err != nil {
					t.Errorf("Write = %v", err)
				}
			}
		}()
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	counts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		counts[line]++
	}
	for i := range writers {
		line := fmt.Sprintf("writer %d\\x1b says hello", i)
		if counts[line] != lines {
			t.Errorf("%q was written %d times, want %d", line, counts[line], lines)
		}
	}
}
