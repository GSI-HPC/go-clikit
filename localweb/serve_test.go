// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const patience = 10 * time.Second

// loopback listens on a free port of 127.0.0.1.
func loopback(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

// serving runs Serve in the background.
func serving(ctx context.Context, ls []net.Listener, h http.Handler, o ServeOptions) <-chan error {
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ls, h, o) }()
	return done
}

// result waits for what Serve returns.
func result(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(patience):
		t.Fatal("Serve did not return")
		return nil
	}
}

func get(t *testing.T, url string) string {
	t.Helper()
	c := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: patience}
	res, err := c.Get(url)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	return string(body)
}

func hello(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello")) }

func TestServeStopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()

	a, b := loopback(t), loopback(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := serving(ctx, []net.Listener{a, b}, http.HandlerFunc(hello), ServeOptions{})

	for _, l := range []net.Listener{a, b} {
		if got := get(t, "http://"+l.Addr().String()+"/"); got != "hello" {
			t.Errorf("answer = %q, want hello", got)
		}
	}
	cancel()

	if err := result(t, done); err != nil {
		t.Errorf("Serve = %v, want nil", err)
	}
	for _, l := range []net.Listener{a, b} {
		if c, err := net.DialTimeout("tcp", l.Addr().String(), time.Second); err == nil {
			_ = c.Close()
			t.Errorf("%s still accepts", l.Addr())
		}
	}
}

func TestServeStopsAtOnceWhenTheContextHasEnded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := result(t, serving(ctx, []net.Listener{loopback(t)}, http.HandlerFunc(hello), ServeOptions{}))

	if err != nil {
		t.Errorf("Serve = %v, want nil", err)
	}
}

func TestServeLetsARequestInFlightFinish(t *testing.T) {
	t.Parallel()

	l := loopback(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("late"))
	})
	done := serving(ctx, []net.Listener{l}, h, ServeOptions{Grace: patience})
	answer := make(chan string, 1)
	go func() { answer <- get(t, "http://"+l.Addr().String()+"/") }()
	<-started

	cancel()
	select {
	case err := <-done:
		t.Fatalf("Serve returned %v while a request was in flight", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	if err := result(t, done); err != nil {
		t.Errorf("Serve = %v, want nil", err)
	}
	if got := <-answer; got != "late" {
		t.Errorf("answer = %q, want late", got)
	}
}

// A handler that waits for its client to go ends on the stop signal, and
// does not hold the stop for the grace period.
func TestStoppingEndsAHandlerThatWaits(t *testing.T) {
	t.Parallel()

	l := loopback(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-Stopping(r.Context()):
			_, _ = w.Write([]byte("stopping"))
		case <-r.Context().Done():
		}
	})
	done := serving(ctx, []net.Listener{l}, h, ServeOptions{Grace: time.Hour})
	answer := make(chan string, 1)
	go func() { answer <- get(t, "http://"+l.Addr().String()+"/") }()
	<-started

	begin := time.Now()
	cancel()

	if err := result(t, done); err != nil {
		t.Errorf("Serve = %v, want nil", err)
	}
	if took := time.Since(begin); took > patience {
		t.Errorf("the stop took %v", took)
	}
	if got := <-answer; got != "stopping" {
		t.Errorf("answer = %q, want stopping", got)
	}
}

// Outside Serve there is no stop signal.
func TestStoppingOutsideServe(t *testing.T) {
	t.Parallel()

	if Stopping(context.Background()) != nil {
		t.Error("Stopping of a bare context is not nil")
	}
}

func TestServeCutsOffARequestAtTheEndOfTheGrace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		grace time.Duration
	}{
		{name: "a short grace", grace: 50 * time.Millisecond},
		{name: "a negative grace stops at once", grace: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := loopback(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			started, release := make(chan struct{}), make(chan struct{})
			defer close(release)
			h := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				close(started)
				<-release
			})
			done := serving(ctx, []net.Listener{l}, h, ServeOptions{Grace: tt.grace})
			go func() {
				c := &http.Client{Timeout: patience}
				if res, err := c.Get("http://" + l.Addr().String() + "/"); err == nil {
					_ = res.Body.Close()
				}
			}()
			<-started

			cancel()

			err := result(t, done)
			if err == nil || !strings.HasPrefix(err.Error(), "stop within the grace period: ") {
				t.Errorf("Serve = %v, want the grace period error", err)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("Serve = %v, want it to wrap context.DeadlineExceeded", err)
			}
		})
	}
}

// failingListener fails to accept.
type failingListener struct {
	net.Listener
	err    error
	closed chan struct{}
	once   sync.Once
}

func (f *failingListener) Accept() (net.Conn, error) { return nil, f.err }

func (f *failingListener) Close() error {
	f.once.Do(func() { close(f.closed) })
	return f.Listener.Close()
}

func TestServeStopsTheOthersWhenAListenerFails(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	bad := &failingListener{Listener: loopback(t), err: boom, closed: make(chan struct{})}
	good := loopback(t)
	// The bad listener fails at once, and that ends Serve.
	done := serving(t.Context(), []net.Listener{good, bad}, http.HandlerFunc(hello), ServeOptions{})

	err := result(t, done)

	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "serve: ") {
		t.Errorf("Serve = %v, want serve: boom", err)
	}
	if c, err := net.DialTimeout("tcp", good.Addr().String(), time.Second); err == nil {
		_ = c.Close()
		t.Error("the other listener still accepts")
	}
	select {
	case <-bad.closed:
	default:
		t.Error("the failed listener is not closed")
	}
}

// acceptSignal tells when a connection was accepted.
type acceptSignal struct {
	net.Listener
	accepted chan struct{}
}

func (a *acceptSignal) Accept() (net.Conn, error) {
	c, err := a.Listener.Accept()
	if err == nil {
		a.accepted <- struct{}{}
	}
	return c, err
}

func TestAConnectionThatSentNothingDoesNotDelayTheStop(t *testing.T) {
	t.Parallel()

	l := &acceptSignal{Listener: loopback(t), accepted: make(chan struct{}, 4)}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// A grace period that the test would not outlast.
	done := serving(ctx, []net.Listener{l}, http.HandlerFunc(hello), ServeOptions{Grace: time.Hour})
	quiet, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = quiet.Close() }()
	<-l.accepted

	cancel()

	if err := result(t, done); err != nil {
		t.Errorf("Serve = %v, want nil", err)
	}
	_ = quiet.SetReadDeadline(time.Now().Add(patience))
	if _, err := quiet.Read(make([]byte, 1)); err == nil {
		t.Error("the connection delivered data")
	} else if isTimeout(err) {
		t.Error("the quiet connection is left open")
	}
}

func TestServeWithZeroAndNegativeOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		o    ServeOptions
	}{
		{name: "zero"},
		// A negative Grace cuts off a connection that has not yet gone
		// idle after its answer, which TestServeCutsOffARequestAtTheEndOfTheGrace
		// covers; here it would make the result depend on timing.
		{name: "negative", o: ServeOptions{
			ReadHeaderTimeout: -1, ReadTimeout: -1, WriteTimeout: -1, IdleTimeout: -1, MaxHeaderBytes: -1,
		}},
		{name: "set", o: ServeOptions{
			ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second,
			MaxHeaderBytes: 1 << 12, Grace: time.Second,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := loopback(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := serving(ctx, []net.Listener{l}, http.HandlerFunc(hello), tt.o)

			if got := get(t, "http://"+l.Addr().String()+"/"); got != "hello" {
				t.Errorf("answer = %q, want hello", got)
			}
			cancel()

			if err := result(t, done); err != nil {
				t.Errorf("Serve = %v, want nil", err)
			}
		})
	}
}

func TestLimit(t *testing.T) {
	t.Parallel()

	durations := []struct{ v, want time.Duration }{
		{0, time.Minute}, {-1, 0}, {-time.Hour, 0}, {time.Second, time.Second},
	}
	for _, tt := range durations {
		if got := limit(tt.v, time.Minute); got != tt.want {
			t.Errorf("limit(%v, 1m) = %v, want %v", tt.v, got, tt.want)
		}
	}
	sizes := []struct{ v, want int }{{0, 64}, {-1, 0}, {100, 100}}
	for _, tt := range sizes {
		if got := limit(tt.v, 64); got != tt.want {
			t.Errorf("limit(%d, 64) = %d, want %d", tt.v, got, tt.want)
		}
	}
}

func TestSpareConns(t *testing.T) {
	t.Parallel()

	t.Run("a connection that has sent nothing is closed at once", func(t *testing.T) {
		t.Parallel()

		s := &spareConns{conns: map[net.Conn]struct{}{}}
		c, peer := net.Pipe()
		s.track(c, http.StateNew)

		s.closeAll()

		if !isClosedBy(peer) {
			t.Error("the connection is left open")
		}
		if len(s.conns) != 0 || !s.closing {
			t.Errorf("conns = %v, closing = %v", s.conns, s.closing)
		}
	})

	t.Run("a connection that sent a request is left", func(t *testing.T) {
		t.Parallel()

		s := &spareConns{conns: map[net.Conn]struct{}{}}
		c, peer := net.Pipe()
		defer func() { _ = c.Close(); _ = peer.Close() }()
		s.track(c, http.StateNew)
		s.track(c, http.StateActive)
		if len(s.conns) != 0 {
			t.Errorf("conns = %v, want none", s.conns)
		}

		s.closeAll()

		_ = peer.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		if _, err := peer.Read(make([]byte, 1)); err == nil || !isTimeout(err) {
			t.Errorf("read = %v, want a timeout: the connection was closed", err)
		}
	})

	t.Run("a connection that arrives after closing started is closed", func(t *testing.T) {
		t.Parallel()

		s := &spareConns{conns: map[net.Conn]struct{}{}}
		s.closeAll()
		c, peer := net.Pipe()

		s.track(c, http.StateNew)

		if !isClosedBy(peer) {
			t.Error("the late connection is left open")
		}
		if len(s.conns) != 0 {
			t.Errorf("conns = %v, want none", s.conns)
		}
		// A state change of a connection that is gone is no error.
		s.track(c, http.StateClosed)
	})
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
