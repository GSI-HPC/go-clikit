// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GSI-HPC/go-clikit/progress"
	"github.com/GSI-HPC/go-nodeset"
)

// MapOptions say how Map works on its items and how it reports them.
type MapOptions[T any] struct {
	// Step names the step a display shows the items under, such as
	// "copy" or "reset the machines".
	Step string
	// Flags are given to the step, which is Fold as well: ShowLines for
	// work whose output is the product.
	Flags progress.Flags
	// Limit is how many items are worked on at once; below one is
	// DefaultLimit.
	Limit int
	// Describe says what a display names an item by, as an Item. Nil, or
	// an Item with no Node, names an item the way fmt.Sprint prints it.
	Describe func(T) Item
	// PanicLog receives the stack of a panic in the work, in Acquire or
	// in the release it gave, the front end's diagnostics; nil is the
	// process's standard error.
	PanicLog io.Writer
	// Program names the program in the line that says the work panicked,
	// and in the error a panic becomes, as Recovered does.
	Program string
	// Classify is the fallback of progress.Classify for the errors of the
	// items, which tells an item that ends canceled: the program's own
	// rule, as progress.BusOptions.Classify is the Bus's; nil, or an
	// answer of ClassNone, is ClassTarget.
	Classify func(error) progress.Class
	// Summarize sums up the items, as the error the step ends with; nil
	// is Failure, with Noun. A program gives it the error its commands
	// exit with, as Failure's text with an exit code of its own.
	// Summary.Canceled says that every item that failed ended canceled,
	// which the end of ctx does whether it was interrupted or ran out of
	// time, and an error Classify classes as canceled does as well.
	// ctx.Err, or context.Cause, tells an interrupt from a deadline: Map
	// itself puts the context's error in Summary.Failed only for the items
	// the pool left out, since an item cut short keeps the error its work
	// or Acquire returned, which is the context's only if they returned
	// it.
	Summarize func(Summary) error
	// Noun names the items in the error the default Summarize, Failure,
	// ends the step with, such as "hosts"; empty names none. A Summarize
	// of the program's is not given it.
	Noun string
	// Acquire, when it is set, takes what an item's work needs besides its
	// place in the pool, such as a place on each host it goes to, and
	// returns the function that gives that back once the work is done; a
	// nil release gives nothing back. The release is called once fn has
	// returned, panicked or called runtime.Goexit, but before the item's
	// target ends, so that a panic in it is reported as the item's. An
	// event log or a display can so show the next item that takes what it
	// gave back running before this one's target has ended.
	//
	// It is called once the item has its place in the pool, and the item
	// stays queued until it returns. An error it returns, or a panic in
	// it, is the item's, whose work is then never started and whose
	// release, if it returned one, is not called. The item's target ends
	// then, before the place is given up: skipped for a skip, an error
	// that is progress.ErrSkipped, which leaves the item out on purpose,
	// as one fn returns does; canceled, with the context's error, when
	// the context had ended by then and the error is not a panic; and
	// with the error otherwise.
	Acquire func(ctx context.Context, item T) (release func(), err error)
}

// Item is what MapOptions.Describe says of an item: what its target is
// named by, and the attributes of progress.Fields a display shows of it.
type Item struct {
	// Node names the item's target, as its span's name and as
	// progress.Fields.Node, and is the name the summary of the items that
	// failed gives it: the node, or whatever else the item is. Empty is
	// the item as fmt.Sprint prints it.
	Node string
	// Host is the address the work goes to, such as the node's service
	// processor, as progress.Fields.Host; empty says none.
	Host string
	// Role is the host role of the item, as progress.Fields.Role; empty
	// says none.
	Role string
}

// Outcome is what the work for one item came to.
type Outcome[R any] struct {
	// Value is what the work returned, which a panic in the release
	// leaves as it was; the zero value when the work was never started,
	// panicked or called runtime.Goexit.
	Value R
	// Err is the error the work returned, which it keeps when the end of
	// the context ends the item's target canceled, or the one a panic in
	// it became; a panic in the release MapOptions.Acquire gave is joined to
	// the error the work returned, or replaces it when that was nil or a
	// skip. When fn, MapOptions.Acquire or the release ended its
	// goroutine with runtime.Goexit rather than return, as t.FailNow does,
	// it is an error that says which of them did, joined to a panic in the
	// release and to the error the work returned, as a panic in the
	// release is. For an item that was never started it is the error
	// MapOptions.Acquire refused it with, or the one a panic in it became,
	// even once the context had ended, or, when the pool left the item
	// out, the context's.
	Err error
	// Started says whether the work for the item was started, which it
	// is not when the pool left the item out once the context had ended,
	// nor when MapOptions.Acquire refused it, panicked or called
	// runtime.Goexit.
	Started bool
}

// Map calls fn with every item, at most o.Limit at a time, and returns what
// each call came to in the order of the items, whatever order they finished
// in. An item that fails does not stop the others. It runs on Each: once
// ctx ends no further item is started, and those left out come back with
// the context's error. A panic in fn, o.Acquire or the release it gave
// becomes that item's error, with its stack in o.PanicLog, as Recovered has
// it, and so does a call of runtime.Goexit in them, as t.FailNow makes,
// with an error that says which of them made it, so that the item fails
// rather than its worker end without a word. The release is called however
// fn ended, before the item's target ends. Map returns once every call has
// returned.
//
// The work is reported under the span ctx carries as a step, o.Step, with
// a target for each item, as every pool reports it: every target is
// announced, queued, before the first one runs; each is marked running when
// it takes its place and ended before it gives the place up, so that a
// display never counts more running than the limit; those the pool left
// out end canceled once it is done, so that the count reaches its total;
// and the step ends once the last has, with what o.Summarize makes of the
// Summary of the items that failed, which says whether every one of them
// ended canceled, as the end of ctx leaves a pool, which Failure, the
// default, then ends canceled.
// fn is called with the context of its item's target, so that the calls it
// makes are reported under it.
//
// The end of ctx ends an item canceled, not failed, whether ctx was
// interrupted or ran out of time: one the pool left out, and one that
// o.Acquire refused, or whose work returned an error, once ctx had ended,
// with an error other than a skip, since the end of ctx is what ended it.
// Its target ends with the context's error, and its outcome keeps the
// error fn or o.Acquire returned. A panic or a call of runtime.Goexit is
// a bug, though, and fails its item even once ctx has ended, and a panic
// in the release fails it whatever fn returned, a skip included. An item
// fn left out on purpose, by returning a skip, an error that is
// progress.ErrSkipped as errors.Is tells, such as one progress.Skip
// returns, ends skipped, with the error's text as its Err, and is none of
// those that failed. An item waiting
// for o.Acquire is not yet running, and one it refused ends at once, as
// MapOptions.Acquire says.
func Map[T, R any](ctx context.Context, items []T, o MapOptions[T], fn func(ctx context.Context, item T) (R, error)) []Outcome[R] {
	limit := o.Limit
	if limit < 1 {
		limit = DefaultLimit
	}
	stepCtx, step := progress.Start(ctx, progress.KindStep, o.Step,
		progress.WithFlags(progress.Fold|o.Flags), progress.Total(len(items)), progress.Limit(limit))
	names := make([]string, len(items))
	ctxs := make([]context.Context, len(items))
	spans := make([]*progress.Span, len(items))
	for i, item := range items {
		d := o.describe(item)
		names[i] = d.Node
		ctxs[i], spans[i] = progress.Start(stepCtx, progress.KindTarget, d.Node, progress.Queued(),
			progress.Node(d.Node), progress.Host(d.Host), progress.Role(d.Role))
	}

	out := make([]Outcome[R], len(items))
	// canceled are the items whose targets ended canceled.
	canceled := make([]bool, len(items))
	Each(ctx, len(items), limit, func(i int) {
		// returned says whether the worker came to its end; one that
		// runtime.Goexit ends runs only what it deferred, and e says how
		// far it had come.
		returned := false
		var e ending[R]
		defer func() {
			if !returned {
				cause := errGoexit[e.stage]
				if e.broke != nil {
					cause = errors.Join(cause, e.broke)
				}
				var end error
				out[i].Value = e.value
				out[i].Err, end = failure(e.err, cause)
				spans[i].End(end)
			}
		}()
		release, panicked, err := o.acquire(ctxs[i], names[i], items[i])
		if err != nil {
			returned = true
			out[i].Err = err
			canceled[i] = o.refused(ctxs[i], spans[i], err, panicked)
			return
		}
		e.stage = stageWork
		spans[i].Run()
		out[i].Started = true
		call(ctxs[i], o.PanicLog, o.Program, names[i], items[i], fn, release, &e)
		returned = true
		out[i].Value = e.value
		if e.broke != nil {
			var end error
			out[i].Err, end = failure(e.err, e.broke)
			canceled[i] = o.endsCanceled(end)
			spans[i].End(end)
			return
		}
		err = e.err
		out[i].Err = err
		if errors.Is(err, progress.ErrSkipped) {
			spans[i].End(err)
			return
		}
		if err != nil && !e.panicked && ctxs[i].Err() != nil {
			err = leftOut{ctxs[i].Err()}
		}
		canceled[i] = o.endsCanceled(err)
		spans[i].End(err)
	})
	s := Summary{Total: len(items), Canceled: true}
	for i := range out {
		// An item with neither a start nor an error is one Each left
		// out, which it does only once ctx has ended.
		if !out[i].Started && out[i].Err == nil {
			out[i].Err = ctx.Err()
			canceled[i] = true
			spans[i].End(leftOut{out[i].Err})
		}
		if out[i].Err != nil && !errors.Is(out[i].Err, progress.ErrSkipped) {
			s.Failed = append(s.Failed, Failed{Name: names[i], Err: out[i].Err})
			s.Canceled = s.Canceled && canceled[i]
		}
	}
	s.Canceled = s.Canceled && len(s.Failed) > 0
	var err error
	if o.Summarize != nil {
		err = o.Summarize(s)
	} else {
		err = Failure(o.Noun, s)
	}
	step.End(err)
	return out
}

// refused ends the target of an item o.Acquire refused with err, before
// the item gives its place in the pool up, and reports whether it ended
// canceled: skipped for a skip, as one never started when ctx
// had ended by then, unless err is a panic in o.Acquire, as panicked says,
// and with err otherwise.
func (o MapOptions[T]) refused(ctx context.Context, span *progress.Span, err error, panicked bool) bool {
	if errors.Is(err, progress.ErrSkipped) {
		span.End(err)
		return false
	}
	if cause := ctx.Err(); cause != nil && !panicked {
		span.End(leftOut{cause})
		return true
	}
	span.End(err)
	return o.endsCanceled(err)
}

// endsCanceled reports whether a target that ends with err ends canceled.
func (o MapOptions[T]) endsCanceled(err error) bool {
	return err != nil && progress.Classify(err, o.Classify) == progress.ClassCanceled
}

// describe says what a display names an item by.
func (o MapOptions[T]) describe(item T) Item {
	var d Item
	if o.Describe != nil {
		d = o.Describe(item)
	}
	if d.Node == "" {
		d.Node = fmt.Sprint(item)
	}
	return d
}

// acquire takes what an item's work needs besides its place in the pool.
// A panic in o.Acquire becomes the item's error, as panicked says.
func (o MapOptions[T]) acquire(ctx context.Context, name string, item T) (release func(), panicked bool, err error) {
	if o.Acquire == nil {
		return func() {}, false, nil
	}
	defer func() {
		if p := Recovered(o.PanicLog, o.Program, name, recover()); p != nil {
			release, panicked, err = nil, true, p
		}
	}()
	release, err = o.Acquire(ctx, item)
	if err == nil && release == nil {
		release = func() {}
	}
	return release, false, err
}

// stage is how far the worker for an item has come: in MapOptions.Acquire,
// in the work, or in the release.
type stage int

const (
	stageAcquire stage = iota
	stageWork
	stageRelease
)

// ending is what became of the work for one item, as call leaves it, even
// when runtime.Goexit ends the goroutine before call returns.
type ending[R any] struct {
	// value and err are what fn returned; err is what a panic in it
	// became, as panicked says.
	value    R
	err      error
	panicked bool
	// broke is what a panic in the release became.
	broke error
	// stage is how far the worker had come.
	stage stage
}

// call calls fn with one item, and then release, however fn ended, and
// leaves what came of them in e. A panic in fn becomes its error, and one in
// release becomes e.broke, rather than the end of the process, each with
// its stack written to log. e.stage is stageRelease once fn has returned or
// panicked, so that a call of runtime.Goexit is put down to the right one.
func call[T, R any](ctx context.Context, log io.Writer, program, name string, item T, fn func(context.Context, T) (R, error), release func(), e *ending[R]) {
	defer func() {
		e.broke = Recovered(log, program, name, recover())
	}()
	defer release()
	returned := false
	defer func() {
		p := Recovered(log, program, name, recover())
		if p == nil && !returned {
			// runtime.Goexit ended fn.
			return
		}
		if p != nil {
			e.err, e.panicked = p, true
		}
		e.stage = stageRelease
	}()
	e.value, e.err = fn(ctx, item)
	returned = true
}

// failure is what an item comes to whose release broke with cause, a panic
// or a call of runtime.Goexit, after fn returned err, or whose worker cause
// ended before fn returned, when err is nil: as its outcome, cause joined to
// err, or cause alone when err is nil or a skip; and as what its
// target ends with, an error with the text of the outcome but only cause in
// its chain, so that the target is classed as cause is and not as err,
// which may be the context's.
func failure(err, cause error) (outcome, end error) {
	if err == nil || errors.Is(err, progress.ErrSkipped) {
		return cause, cause
	}
	outcome = errors.Join(err, cause)
	return outcome, releaseFailure{text: outcome.Error(), cause: cause}
}

// releaseFailure is what the target of an item ends with when its release
// broke after fn returned an error, as failure makes it.
type releaseFailure struct {
	text  string
	cause error
}

func (e releaseFailure) Error() string { return e.text }

func (e releaseFailure) Unwrap() error { return e.cause }

// errGoexit is the error of an item whose worker runtime.Goexit ended, as
// t.FailNow does, in each stage: which of MapOptions.Acquire, the work and the
// release called it rather than return.
var errGoexit = [...]error{
	stageAcquire: errors.New("acquiring what the work needs called runtime.Goexit instead of returning"),
	stageWork:    errors.New("the work called runtime.Goexit instead of returning"),
	stageRelease: errors.New("releasing what the work needed called runtime.Goexit instead of returning"),
}

// Summary is what the items of a fan-out came to, as MapOptions.Summarize
// and Failure are given it. Fields may be added to it in a minor release.
type Summary struct {
	// Total is how many items there were.
	Total int
	// Failed are the items that failed, in the order of the items: not
	// those that ended skipped.
	Failed []Failed
	// Canceled says that at least one item failed and every item that
	// failed ended canceled, as the end of the context ends the items,
	// whether it was interrupted or ran out of time (decision 11), and
	// an error the class rule classes as canceled does as well.
	Canceled bool
}

// Failed is one item that failed, as Summary lists it.
type Failed struct {
	// Name is what the item is named by, Item.Node.
	Name string
	// Err is the item's error, Outcome.Err.
	Err error
}

// Failure sums up the items of a fan-out that failed, as s has them:
// "k of n <noun> failed: <names>", or "k of n failed: <names>" without a
// noun, with each error that is not nil underneath, so that errors.Is and
// errors.As still find a cancellation among them. The names are written
// as a node set when each reads as one host name in one and none repeats,
// and otherwise as a list separated by commas, in the order given, so that
// a name such as "config volume" is not read as two hosts and the list
// names as many as the count. Its progress class is ClassCanceled when
// s.Canceled is true, as Map sets it when every item that failed ended
// canceled, such as when the context ended, by an interrupt or a
// deadline, since the error of an item is what its work returned, which
// need not say so; it is ClassTarget otherwise. It is nil when none
// failed.
func Failure(noun string, s Summary) error {
	if len(s.Failed) == 0 {
		return nil
	}
	names := make([]string, len(s.Failed))
	var kept []error
	for i, f := range s.Failed {
		names[i] = f.Name
		if f.Err != nil {
			kept = append(kept, f.Err)
		}
	}
	what := "failed"
	if noun != "" {
		what = noun + " failed"
	}
	class := progress.ClassTarget
	if s.Canceled {
		class = progress.ClassCanceled
	}
	return &failedItems{
		message: fmt.Sprintf("%d of %d %s: %s", len(names), s.Total, what, list(names)),
		errs:    kept,
		class:   class,
	}
}

// list names items as a node set when every name reads as one host name in
// one and none repeats, and as a list separated by commas otherwise.
func list(names []string) string {
	set := nodeset.New()
	for _, name := range names {
		one, err := nodeset.Parse(name)
		if err != nil || one.Len() != 1 || one.String() != name {
			return strings.Join(names, ",")
		}
		// Add grows the set in place; a Union of each name would copy the
		// whole set every time, which takes seconds for thousands of names.
		_ = set.Add(name)
	}
	// A node set holds a name once, so names that repeat are listed as
	// given, as many as the count says.
	if set.Len() != len(names) {
		return strings.Join(names, ",")
	}
	return set.String()
}

// failedItems is the summary of a fan-out that did not succeed everywhere,
// with the error of each item that failed underneath it.
type failedItems struct {
	message string
	errs    []error
	class   progress.Class
}

func (e *failedItems) Error() string { return e.message }

func (e *failedItems) Unwrap() []error { return e.errs }

// ProgressClass says why the fan-out failed as a whole, rather than as
// whichever of its items' errors says a class first.
func (e *failedItems) ProgressClass() progress.Class { return e.class }
