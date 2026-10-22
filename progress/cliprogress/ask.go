// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package cliprogress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GSI-HPC/go-clikit/progress"
)

// maxAnswer bounds the line Ask reads.
const maxAnswer = 4096

// Ask writes prompt to out and reads one line from in, the answer, with
// every display of the Bus in ctx taken off the terminal until it returns,
// so that neither draws over the other. It returns the line without its
// end, "\n" or "\r\n". The prompt is written as it is: it is the program's
// own text, and what it quotes from elsewhere the program escapes first.
//
// Ask reads in a byte at a time, so that what follows the answer is left
// for the next reader of in. Where in ends before a line does, the answer
// is what came, and io.EOF the error when nothing came. When ctx ends
// first, Ask returns its error at once, and the line it was reading is
// lost to whoever reads in next. Either way a newline is written to out,
// so that what follows starts a line of its own. A line longer than 4096
// bytes is refused, so that a file given as in, rather than a person, ends
// the question.
//
// The package asks nothing on its own: whether a command asks where in is
// no terminal, IsTerminal tells, and what an answer, or none, means, Yes
// among them, and the exit status that follows, are the program's.
func Ask(ctx context.Context, in io.Reader, out io.Writer, prompt string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	defer progress.Suspend(ctx)()
	if _, err := io.WriteString(out, prompt); err != nil {
		return "", err
	}
	type answer struct {
		line  string
		ended bool
		err   error
	}
	got := make(chan answer, 1)
	go func() {
		line, ended, err := readLine(in)
		got <- answer{line, ended, err}
	}()
	select {
	case <-ctx.Done():
		_, _ = io.WriteString(out, "\n")
		return "", ctx.Err()
	case a := <-got:
		if !a.ended {
			_, _ = io.WriteString(out, "\n")
		}
		return a.line, a.err
	}
}

// errLongAnswer is the error of an answer longer than maxAnswer.
var errLongAnswer = fmt.Errorf("the answer is longer than %d bytes", maxAnswer)

// readLine reads in a byte at a time up to the end of a line, which ended
// says it reached, and returns the line without its end.
func readLine(in io.Reader) (line string, ended bool, err error) {
	var b bytes.Buffer
	var c [1]byte
	for {
		n, err := in.Read(c[:])
		if n == 1 {
			if c[0] == '\n' {
				return strings.TrimSuffix(b.String(), "\r"), true, nil
			}
			if b.Len() == maxAnswer {
				return "", false, errLongAnswer
			}
			b.WriteByte(c[0])
		}
		switch {
		case errors.Is(err, io.EOF) && b.Len() > 0:
			return b.String(), false, nil
		case err != nil:
			return "", false, err
		}
	}
}

// Yes reports whether answer says yes: "y" or "yes", in any case, with
// spaces around it or not.
func Yes(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}
