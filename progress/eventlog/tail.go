// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package eventlog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
)

// MaxLine is the longest line a Tail reads, in bytes, its newline
// included; a longer one is left out, whole, and the line after it read. The log writes no line
// nearly as long: every text of an event has its bound.
const MaxLine = 1 << 20

// headLen is how much of the start of the log a Tail remembers, to tell
// a log made again in its place from the one it read.
const headLen = 128

// Feed takes the lines a Tail reads.
type Feed interface {
	// Line takes one whole line of the log, without its newline. b is
	// the Tail's, and only good until Line returns.
	Line(b []byte)
	// Reset forgets every line taken: the log was cut short or made
	// again, and is read again from its start, the rotated copy first.
	Reset()
}

// Tail reads an event log as it grows, polled: each Poll gives a Feed
// the whole lines written since the one before, and keeps a line still
// being written for the next. It is not safe for concurrent use.
//
// It follows the log as progress/cliprogress rotates it, to its name with
// ".1" after it (decision 31). The first Poll reads that copy before the
// log, so that the runs before the last rotation count too, and every
// Poll after it what was written to the copy since, while it is the file
// it was: a command that had the log open when another rotated it goes on
// writing there. When the log has been moved to the copy, Poll reads the
// rest of it there and starts the new log from its start, and the Feed
// sees each line once. When the log is anything else than it was, a file
// shorter than what was read, one that starts with other bytes, or
// another file that is not the copy, Poll resets the Feed and reads the
// copy and the log again from their starts. A log that is missing is an
// empty one.
type Tail struct {
	path string
	feed Feed
	// log is where the Tail stands in the log, and old in its rotated
	// copy.
	log, old position
	started  bool
	// line is the buffer the lines are put together in, kept from one
	// Poll to the next.
	line []byte
}

// position is where a Tail stands in a file.
type position struct {
	info os.FileInfo
	off  int64
	// partial is the start of a line still being written.
	partial []byte
	// head is the start of the file as it was read.
	head []byte
}

// NewTail returns a Tail that reads the log at path into feed.
func NewTail(path string, feed Feed) *Tail {
	return &Tail{path: path, feed: feed}
}

// Poll gives the Feed the whole lines written since the last Poll, as the
// Tail's comment says, and reports whether it gave it any or reset it.
// A log or a copy that is missing is no error; one that cannot be opened
// or read is, and Poll then gives the Feed what it read before the
// error, and tries again on the next call.
func (t *Tail) Poll() (bool, error) {
	f, info, err := open(t.path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	changed := false
	if t.log.info != nil && (info == nil || !os.SameFile(t.log.info, info)) {
		if old, err := os.Stat(t.path + ".1"); err == nil && os.SameFile(t.log.info, old) {
			// Rotated: the copy is the log that was read.
			t.old, t.log = t.log, position{}
		} else if info != nil {
			changed = t.reset()
		}
	} else if info != nil && t.log.info != nil && (info.Size() < t.log.off || !sameHead(f, t.log.head)) {
		changed = t.reset()
	}
	n, err := t.readOld()
	changed = changed || n > 0
	if err != nil || info == nil {
		return changed, err
	}
	t.started = true
	n, err = t.read(f, info, &t.log)
	return changed || n > 0, err
}

// reset resets the Feed and has the copy and the log read from their
// starts. It reports true, that the Feed changed.
func (t *Tail) reset() bool {
	t.feed.Reset()
	t.log, t.old, t.started = position{}, position{}, false
	return true
}

// readOld reads the rotated copy of the log: from its start on the first
// Poll, and what was written to it since on the others, while it is the
// file it was. A copy that has been replaced is the log of before, read
// as the copy no more: a later Poll finds the log rotated.
func (t *Tail) readOld() (int, error) {
	if t.started && t.old.info == nil {
		return 0, nil
	}
	f, info, err := open(t.path + ".1")
	if err != nil || info == nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	if t.old.info != nil && !os.SameFile(t.old.info, info) {
		t.old = position{}
		return 0, nil
	}
	return t.read(f, info, &t.old)
}

// open opens the file at path, and returns it with its info; both are nil
// for a file that is missing.
func open(path string) (*os.File, os.FileInfo, error) {
	f, err := os.Open(path)
	var info os.FileInfo
	if err == nil {
		info, err = f.Stat()
	}
	if err != nil {
		// A nil file is closed without harm.
		_ = f.Close()
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("reading the event log: %w", err)
	}
	return f, info, nil
}

// sameHead reports whether f starts with head.
func sameHead(f *os.File, head []byte) bool {
	b := make([]byte, len(head))
	n, _ := f.ReadAt(b, 0)
	return bytes.Equal(b[:n], head)
}

// read gives the Feed the whole lines of f from where p stands, but for
// those that are blank, and moves p on. It returns how many lines it gave.
func (t *Tail) read(f *os.File, info os.FileInfo, p *position) (int, error) {
	p.info = info
	br := bufio.NewReaderSize(io.NewSectionReader(f, p.off, math.MaxInt64-p.off), 64<<10)
	t.line = append(t.line[:0], p.partial...)
	lines := 0
	var err error
	for {
		var chunk []byte
		chunk, err = br.ReadSlice('\n')
		p.off += int64(len(chunk))
		// A line over the limit is read to its end, but not kept.
		if len(t.line) <= MaxLine {
			t.line = append(t.line, chunk...)
		}
		if err == nil {
			if line := bytes.TrimSpace(t.line); len(t.line) <= MaxLine && len(line) > 0 {
				t.feed.Line(line)
				lines++
			}
			t.line = t.line[:0]
			continue
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			break
		}
	}
	p.partial = append(p.partial[:0], t.line...)
	if want := int(min(p.off, headLen)); want > len(p.head) {
		head := make([]byte, want)
		n, _ := f.ReadAt(head, 0)
		p.head = head[:n]
	}
	if errors.Is(err, io.EOF) {
		return lines, nil
	}
	return lines, fmt.Errorf("reading the event log: %w", err)
}
