// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"context"
	"io"
)

// AdvanceWriter returns a writer that writes to w and advances the
// innermost span in ctx by the bytes w took, from each call, a short write
// included. It is meant for a span given Work(UnitBytes, ...), so that its
// amount is bytes; a span given no Work counts UnitItems, as Advance does.
// The writer advances nothing once the span has ended, and a write returns
// what w returned. It returns w itself when ctx carries no span, which
// costs nothing.
//
// The writer has Write alone, so io.Copy into it reads and writes through
// a buffer: the io.ReaderFrom of w, such as the sendfile and splice of an
// *os.File or a net.Conn, is not used. A path that delegated to it would
// report the amount only once, at the end. Where the fast path matters
// more than the progress, copy to w itself and advance the span after.
func AdvanceWriter(ctx context.Context, w io.Writer) io.Writer {
	s := SpanFrom(ctx)
	if s == nil {
		return w
	}
	return &advanceWriter{w: w, span: s}
}

// AdvanceReader returns a reader that reads from r and advances the
// innermost span in ctx by the bytes each read gave, a read that also
// returns an error included. It is meant for a span given Work(UnitBytes,
// ...), so that its amount is bytes; a span given no Work counts
// UnitItems, as Advance does. The reader advances nothing once the span has
// ended, and a read returns what r returned. It returns r itself when ctx
// carries no span, which costs nothing.
//
// The reader has Read alone, so io.Copy from it does not use the
// io.WriterTo of r, as AdvanceWriter's doc says of io.ReaderFrom.
func AdvanceReader(ctx context.Context, r io.Reader) io.Reader {
	s := SpanFrom(ctx)
	if s == nil {
		return r
	}
	return &advanceReader{r: r, span: s}
}

// advanceWriter is the writer AdvanceWriter returns.
type advanceWriter struct {
	w    io.Writer
	span *Span
}

// Write writes p to the writer it wraps, and advances the span by what that
// took.
func (c *advanceWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.span.Advance(int64(n))
	}
	return n, err
}

// advanceReader is the reader AdvanceReader returns.
type advanceReader struct {
	r    io.Reader
	span *Span
}

// Read reads into p from the reader it wraps, and advances the span by what
// that gave.
func (c *advanceReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.span.Advance(int64(n))
	}
	return n, err
}
