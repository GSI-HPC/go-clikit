// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// The limits Serve sets when ServeOptions leaves them zero. The only
// client meant is a browser on the same machine, but every local process
// can connect, so nothing is left unbounded.
const (
	// DefaultReadHeaderTimeout ends a connection that sends its request
	// line and headers too slowly, or not at all.
	DefaultReadHeaderTimeout = 10 * time.Second
	// DefaultReadTimeout bounds the reading of a whole request; the forms
	// and documents a page sends are small.
	DefaultReadTimeout = 30 * time.Second
	// DefaultWriteTimeout bounds the time from the end of a request's
	// headers to the end of its answer. A handler that has reason to take
	// longer, such as an event stream, moves its own deadline with
	// http.ResponseController, as localweb/sse does.
	DefaultWriteTimeout = time.Minute
	// DefaultIdleTimeout closes the kept-alive connections of a tab that
	// has stopped asking.
	DefaultIdleTimeout = 2 * time.Minute
	// DefaultMaxHeaderBytes is far above what a browser sends to this
	// origin, a cookie included, and far below net/http's 1 MiB.
	DefaultMaxHeaderBytes = 64 << 10
	// DefaultGrace is how long the requests in flight get to finish when
	// the server stops.
	DefaultGrace = 5 * time.Second
)

// ServeOptions configures Serve. A zero duration or size is its default;
// a negative one is no limit.
type ServeOptions struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	// Grace is how long the requests in flight get to finish once ctx
	// ends. A negative Grace stops at once.
	Grace time.Duration
	// ErrorLog receives what net/http reports, such as a handler's panic
	// and a broken connection; nil is net/http's default, the standard
	// logger of package log.
	ErrorLog *log.Logger
}

// Serve serves h on every listener in ls until ctx ends, then stops: the
// listeners and the connections that have not sent a request close at
// once, a Unix socket that Listen bound leaves the file system, and the
// requests in flight get the grace period to finish. It returns nil after
// such a stop. It returns an error when a listener fails, after it has
// stopped the others at once, and when requests had to be cut off at the
// end of the grace period.
//
// A connection that has sent no request is closed at once on stop, where
// http.Server.Shutdown would wait for it as for a request: a browser opens
// such connections ahead of need, and without this, stopping shortly after
// a page was loaded would take the whole grace period.
func Serve(ctx context.Context, ls []net.Listener, h http.Handler, o ServeOptions) error {
	spare := &spareConns{conns: map[net.Conn]struct{}{}}
	s := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: limit(o.ReadHeaderTimeout, DefaultReadHeaderTimeout),
		ReadTimeout:       limit(o.ReadTimeout, DefaultReadTimeout),
		WriteTimeout:      limit(o.WriteTimeout, DefaultWriteTimeout),
		IdleTimeout:       limit(o.IdleTimeout, DefaultIdleTimeout),
		MaxHeaderBytes:    limit(o.MaxHeaderBytes, DefaultMaxHeaderBytes),
		ConnState:         spare.track,
		ErrorLog:          o.ErrorLog,
	}
	// Shutdown calls this once the listeners are closed.
	s.RegisterOnShutdown(spare.closeAll)

	// Serve returns http.ErrServerClosed once Close or Shutdown has
	// closed its listener, and the first error of another kind alone
	// counts.
	var wg sync.WaitGroup
	served := make(chan error, len(ls))
	for _, l := range ls {
		wg.Go(func() { served <- s.Serve(l) })
	}
	select {
	case err := <-served:
		// One listener failed. Another would go on answering under half
		// of the names the user was told, so it stops too, at once.
		_ = s.Close()
		wg.Wait()
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	grace, cancel := context.WithTimeout(context.WithoutCancel(ctx), limit(o.Grace, DefaultGrace))
	defer cancel()
	err := s.Shutdown(grace)
	if err != nil {
		_ = s.Close()
		err = fmt.Errorf("stop within the grace period: %w", err)
	}
	wg.Wait()
	return err
}

// limit returns v, def when v is zero, and 0, no limit for net/http, when
// v is negative.
func limit[T time.Duration | int](v, def T) T {
	switch {
	case v == 0:
		return def
	case v < 0:
		return 0
	}
	return v
}

// spareConns are the connections on which no request has arrived in full
// yet, http.StateNew.
type spareConns struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
	// closing is set once the server stops; a connection that a listener
	// handed out just before it closed is then closed on arrival.
	closing bool
}

// track follows the state of a connection; it is the server's ConnState.
func (s *spareConns) track(c net.Conn, state http.ConnState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case state != http.StateNew:
		delete(s.conns, c)
	case s.closing:
		// The server notices when it reads.
		_ = c.Close()
	default:
		s.conns[c] = struct{}{}
	}
}

// closeAll closes the connections that have sent nothing, and every one
// that turns up later.
func (s *spareConns) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closing = true
	for c := range s.conns {
		_ = c.Close()
	}
	clear(s.conns)
}
