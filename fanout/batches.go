// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-nodeset"
)

// BatchOptions say how Batches splits a set and paces its batches.
type BatchOptions struct {
	// Step names the step a display shows the batches under, such as
	// "power on".
	Step string
	// Size is the most nodes a batch holds. The set is split evenly into
	// as few batches as that allows, so ten nodes at 8 go as 5 and 5, not
	// 8 and 2. Below one, the set is one batch.
	Size int
	// Limit is the most targets run works on at once in a batch. Batches
	// does not enforce it, but records it on each batch's span, where a
	// display shows it and progresstest.Check holds run to it. Zero
	// records none.
	Limit int
	// Pause is how long to wait between two batches. The wait ends early,
	// and its timer is stopped, when the context ends.
	Pause time.Duration
	// BeforePause is called before each pause, with its length. There is
	// no pause, and no call, when Pause is zero.
	BeforePause func(pause time.Duration)
	// Before is called before batch i of n is run, counting from 0, once
	// the pause before it is over.
	Before func(i, n int, batch *nodeset.NodeSet)
}

// Batch is one of the batches Batches split a set into, and how it ended.
type Batch struct {
	// Nodes are the nodes of the batch.
	Nodes *nodeset.NodeSet
	// Started says whether the batch was run.
	Started bool
	// Err is what running the batch returned, a skip included. For a
	// batch that was not run it is the context's error when the context
	// had ended by its turn, whether an earlier batch failed or not, and
	// ErrNotTried when an earlier batch failed.
	Err error
}

// ErrNotTried is the error of a batch left out because an earlier one
// failed, as Batch.Err holds it. The batch's span ends skipped, so that a
// display counts its nodes as done, but the error is no skip: errors.Is
// does not find progress.ErrSkipped in it, since it says that work was
// left out because of a failure, not on purpose, and a command that
// counts the work that failed, as err != nil and not a skip, counts it.
var ErrNotTried = errors.New("not tried: an earlier batch failed")

// Batches runs a set of nodes in batches, one after the other, and returns
// every batch in order with how it ended. A batch is run by calling run,
// which returns once the work for every node of the batch has returned;
// the next is run once o.Pause has passed after that. A batch whose run
// returns an error stops the run: the batches after it are not tried. A
// skip, an error that is progress.ErrSkipped as errors.Is tells, such as
// one progress.Skip returns, is no failure, though: the batch ends skipped
// and the run goes on. An interrupt stops the run too, at the next pause
// or, without one, before the next batch; the batch under way when it
// came is left to stop by itself, as a pool does, and the first batch is
// always run, so that the work for each of its nodes reports how the
// interrupt found it. The batches an interrupt, or the end of ctx, leaves
// out are left out for that, even after one that failed: the failure of a
// batch the interrupt cut short is no reason of its own. Before and
// BeforePause are called on the calling goroutine, for the notes a command
// prints, and so is run. Batches recovers no panic: one in run, Before or
// BeforePause goes up the calling goroutine, and progress.Bus.Close ends
// the spans it left open canceled.
//
// The work is reported under the span ctx carries as a step, o.Step, whose
// Total is every node of the set, with a span for each batch, all of them
// queued before the first is run. run is called with the context of its
// batch's span, which is marked running first, and reports the work for
// each node of the batch as a target under it, queued before the first
// runs, as Map does; the batch ends with run's error, skipped for a skip.
// Each pause is a wait, "stagger". A batch not tried ends skipped, and one
// the context ended before ends canceled, whether it was interrupted or
// ran out of time, and either counts as its Total, so that the step's
// count reaches its own. The step ends with what ended the run: the error
// of the batch that failed, or else the context's, or else nil, however
// many batches skipped. It and a wait the context ended keep the
// context's error as it is, so a deadline ends them failed, as timeouts,
// and an interrupt canceled.
func Batches(ctx context.Context, nodes *nodeset.NodeSet, o BatchOptions, run func(ctx context.Context, batch *nodeset.NodeSet) error) []Batch {
	parts := 1
	if n := nodes.Len(); o.Size > 0 && n > o.Size {
		// Rounded up without adding to n, which a Size near the largest
		// int would overflow.
		parts = n / o.Size
		if n%o.Size != 0 {
			parts++
		}
	}
	chunks := nodes.Split(parts)

	stepCtx, step := progress.Start(ctx, progress.KindStep, o.Step,
		progress.WithFlags(progress.Fold), progress.Total(nodes.Len()))
	out := make([]Batch, len(chunks))
	ctxs := make([]context.Context, len(chunks))
	spans := make([]*progress.Span, len(chunks))
	for i, chunk := range chunks {
		out[i].Nodes = chunk
		ctxs[i], spans[i] = progress.Start(stepCtx, progress.KindBatch, fmt.Sprintf("%d/%d", i+1, len(chunks)),
			progress.Queued(), progress.Batch(i+1, len(chunks)), progress.Node(chunk.String()),
			progress.Total(chunk.Len()), progress.Limit(o.Limit))
	}

	// err is what ended the run: the error of the batch that failed, or
	// the context's, or nil.
	var err error
	for i, chunk := range chunks {
		if i > 0 {
			if err == nil && o.Pause > 0 {
				pause(ctx, stepCtx, o)
			}
			if cause := ctx.Err(); cause != nil {
				for j := i; j < len(chunks); j++ {
					out[j].Err = cause
					spans[j].End(leftOut{cause})
				}
				if err == nil {
					err = cause
				}
				break
			}
			if err != nil {
				for j := i; j < len(chunks); j++ {
					out[j].Err = ErrNotTried
					spans[j].Skip(ErrNotTried.Error())
				}
				break
			}
		}
		if o.Before != nil {
			o.Before(i, len(chunks), chunk)
		}
		spans[i].Run()
		ran := run(ctxs[i], chunk)
		out[i].Started, out[i].Err = true, ran
		spans[i].End(ran)
		if !errors.Is(ran, progress.ErrSkipped) {
			err = ran
		}
	}
	step.End(err)
	return out
}

// leftOut is what a batch or a target the context left out, or cut short,
// ends with: canceled, since it did not fail, whether the context was
// interrupted or ran out of time, so that it counts as its Total.
type leftOut struct{ error }

// ProgressClass says that the batch or target was left out, or cut short,
// not that it failed.
func (leftOut) ProgressClass() progress.Class { return progress.ClassCanceled }

// pause waits between two batches, until o.Pause has passed or ctx has
// ended, when it stops its timer, and reports the wait under the step.
func pause(ctx, stepCtx context.Context, o BatchOptions) {
	if o.BeforePause != nil {
		o.BeforePause(o.Pause)
	}
	_, wait := progress.Start(stepCtx, progress.KindWait, "stagger", progress.Timeout(o.Pause))
	timer := time.NewTimer(o.Pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		wait.End(ctx.Err())
	case <-timer.C:
		wait.End(nil)
	}
}
