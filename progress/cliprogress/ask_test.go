// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-clikit/progress/cliprogress"
	"github.com/GSI-HPC/go-clikit/progress/progresstest"
)

func TestAsk(t *testing.T) {
	for _, tc := range []struct {
		name, in  string
		answer    string
		err       error
		out, rest string
	}{
		{"a line", "yes\nnext", "yes", nil, "Delete 3 hosts? ", "next"},
		{"a line from a terminal", "3\r\n", "3", nil, "Delete 3 hosts? ", ""},
		{"an empty line", "\n", "", nil, "Delete 3 hosts? ", ""},
		{"a line that does not end", "no", "no", nil, "Delete 3 hosts? \n", ""},
		{"nothing", "", "", io.EOF, "Delete 3 hosts? \n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(tc.in)
			var out strings.Builder
			answer, err := cliprogress.Ask(context.Background(), in, &out, "Delete 3 hosts? ")
			if answer != tc.answer || !errors.Is(err, tc.err) {
				t.Errorf("Ask = %q, %v; want %q, %v", answer, err, tc.answer, tc.err)
			}
			if out.String() != tc.out {
				t.Errorf("wrote %q, want %q", out.String(), tc.out)
			}
			if rest, _ := io.ReadAll(in); string(rest) != tc.rest {
				t.Errorf("left %q of the input, want %q", rest, tc.rest)
			}
		})
	}
}

// A file given as the input, rather than a person, ends the question.
func TestAskRefusesALongAnswer(t *testing.T) {
	var out strings.Builder
	answer, err := cliprogress.Ask(context.Background(), strings.NewReader(strings.Repeat("y", 5000)), &out, "? ")
	if answer != "" || err == nil || err.Error() != "the answer is longer than 4096 bytes" {
		t.Errorf("Ask = %q, %v; want the error of a long answer", answer, err)
	}
	if out.String() != "? \n" {
		t.Errorf("wrote %q", out.String())
	}
}

var errRead = errors.New("read failed")

// failingReader fails every read.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errRead }

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errRead }

func TestAskFails(t *testing.T) {
	if _, err := cliprogress.Ask(context.Background(), failingReader{}, io.Discard, "? "); !errors.Is(err, errRead) {
		t.Errorf("a read that fails: error %v, want %v", err, errRead)
	}
	if _, err := cliprogress.Ask(context.Background(), strings.NewReader("y\n"), failingWriter{}, "? "); !errors.Is(err, errRead) {
		t.Errorf("a prompt that cannot be written: error %v, want %v", err, errRead)
	}
}

// A context that ends asks nothing, and one that ends while Ask waits
// ends the question, as an interrupt does.
func TestAskEndsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out strings.Builder
	if _, err := cliprogress.Ask(ctx, strings.NewReader("y\n"), &out, "? "); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Errorf("an ended context: error %v, wrote %q; want its error and nothing", err, out.String())
	}

	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	defer wr.Close()
	ctx, cancel = context.WithCancel(context.Background())
	interrupt := &cancelOnWrite{cancel: cancel}
	if _, err := cliprogress.Ask(ctx, rd, interrupt, "? "); !errors.Is(err, context.Canceled) {
		t.Errorf("an interrupt: error %v, want %v", err, context.Canceled)
	}
	if interrupt.b.String() != "? \n" {
		t.Errorf("an interrupt: wrote %q, want the prompt and a newline", interrupt.b.String())
	}
}

// cancelOnWrite ends a context as the prompt is written, as an interrupt
// while the question waits would.
type cancelOnWrite struct {
	b      strings.Builder
	cancel func()
}

func (c *cancelOnWrite) Write(p []byte) (int, error) {
	c.cancel()
	return c.b.Write(p)
}

// The display is taken off the terminal while the question is asked, and
// put back once it is answered.
func TestAskTakesTheDisplayOff(t *testing.T) {
	screen := &progresstest.Screen{Width: 80}
	c := newClock()
	r, err := cliprogress.Start(context.Background(), terminal(screen, c))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cmd := progress.Start(r.Context(), progress.KindCommand, "delete cluster")
	r.Draw()
	answer, err := cliprogress.Ask(ctx, strings.NewReader("y\n"), r.Writer(screen), "Delete it? ")
	if !cliprogress.Yes(answer) || err != nil {
		t.Errorf("Ask = %q, %v", answer, err)
	}
	r.Draw()
	cmd.End(nil)
	r.Finish(false)
	if got := screen.String(); !strings.Contains(got, "Delete it? ") {
		t.Errorf("the terminal shows\n%s", got)
	}
}

func TestYes(t *testing.T) {
	for answer, want := range map[string]bool{
		"y": true, "Y": true, "yes": true, " YES ": true, "Yes\t": true,
		"": false, "n": false, "no": false, "yes!": false, "ye": false, "3": false,
	} {
		if got := cliprogress.Yes(answer); got != want {
			t.Errorf("Yes(%q) = %v, want %v", answer, got, want)
		}
	}
}
