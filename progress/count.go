// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package progress

import (
	"context"
	"io"
)

// CountWriter returns a writer that writes to w and advances the innermost
// span in ctx by the bytes w took, from each call, a short write included.
// It is meant for a span given Work(Bytes, ...), so that its amount is
// bytes; a span given no Work counts Items, as Advance does. The writer
// advances nothing once the span has ended, and a write returns what w
// returned. It returns w itself when ctx carries no span, which costs
// nothing.
func CountWriter(ctx context.Context, w io.Writer) io.Writer {
	s := SpanFrom(ctx)
	if s == nil {
		return w
	}
	return &countWriter{w: w, span: s}
}

// CountReader returns a reader that reads from r and advances the innermost
// span in ctx by the bytes each read gave, a read that also returns an
// error included. It is meant for a span given Work(Bytes, ...), so that its
// amount is bytes; a span given no Work counts Items, as Advance does. The
// reader advances nothing once the span has ended, and a read returns what r
// returned. It returns r itself when ctx carries no span, which costs
// nothing.
func CountReader(ctx context.Context, r io.Reader) io.Reader {
	s := SpanFrom(ctx)
	if s == nil {
		return r
	}
	return &countReader{r: r, span: s}
}

// countWriter is the writer CountWriter returns.
type countWriter struct {
	w    io.Writer
	span *Span
}

// Write writes p to the writer it wraps, and advances the span by what that
// took.
func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.span.Advance(int64(n))
	}
	return n, err
}

// countReader is the reader CountReader returns.
type countReader struct {
	r    io.Reader
	span *Span
}

// Read reads into p from the reader it wraps, and advances the span by what
// that gave.
func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.span.Advance(int64(n))
	}
	return n, err
}
