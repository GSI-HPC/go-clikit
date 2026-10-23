// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package launch

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// acquireIn takes the instance "prog" in a new runtime directory.
func acquireIn(t *testing.T) *Instance {
	t.Helper()
	i, err := Acquire(t.Context(), privateDir(t), "prog")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(i.Release)
	return i
}

// writeRecord writes r as the record of i, with mode perm.
func writeRecord(t *testing.T, i *Instance, r Record, perm os.FileMode) {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(i.recordPath(), data, perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(i.recordPath(), perm); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireMakesAPrivateDirectory(t *testing.T) {
	t.Parallel()

	runtimeDir := privateDir(t)
	i, err := Acquire(t.Context(), runtimeDir, "prog")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Release()

	if want := filepath.Join(runtimeDir, "prog"); i.Dir() != want {
		t.Errorf("Dir() = %q, want %q", i.Dir(), want)
	}
	for name, want := range map[string]os.FileMode{"prog": os.ModeDir | 0o700, "prog/" + LockFile: 0o600} {
		info, err := os.Lstat(filepath.Join(runtimeDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode() != want {
			t.Errorf("%s has mode %v, want %v", name, info.Mode(), want)
		}
	}
}

func TestAcquireRefuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, runtimeDir string)
		program string
		relDir  bool
		err     string
	}{
		{name: "a relative runtime directory", relDir: true, program: "prog", err: `the runtime directory "run" is no absolute path`},
		{name: "no name", program: "", err: `the program's name "" cannot name a directory`},
		{name: "a path", program: "a/prog", err: `the program's name "a/prog" cannot name a directory`},
		{name: "dot", program: ".", err: `"." cannot name a directory`},
		{name: "dot dot", program: "..", err: `".." cannot name a directory`},
		{name: "a file in the way", program: "prog", prepare: func(t *testing.T, runtimeDir string) {
			if err := os.WriteFile(filepath.Join(runtimeDir, "prog"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}, err: "make the runtime directory: "},
		{name: "a link to a directory", program: "prog", prepare: func(t *testing.T, runtimeDir string) {
			if err := os.Symlink(privateDir(t), filepath.Join(runtimeDir, "prog")); err != nil {
				t.Fatal(err)
			}
		}, err: "is no directory"},
		{name: "a directory others can enter", program: "prog", prepare: func(t *testing.T, runtimeDir string) {
			if err := os.Mkdir(filepath.Join(runtimeDir, "prog"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(runtimeDir, "prog"), 0o750); err != nil {
				t.Fatal(err)
			}
		}, err: "has mode 0750; it must be 0700"},
		{name: "a link as the lock", program: "prog", prepare: func(t *testing.T, runtimeDir string) {
			if err := os.Mkdir(filepath.Join(runtimeDir, "prog"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(runtimeDir, "elsewhere"), filepath.Join(runtimeDir, "prog", LockFile)); err != nil {
				t.Fatal(err)
			}
		}, err: "open the lock of the runtime directory: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtimeDir := privateDir(t)
			if tt.prepare != nil {
				tt.prepare(t, runtimeDir)
			}
			if tt.relDir {
				runtimeDir = "run"
			}
			i, err := Acquire(t.Context(), runtimeDir, tt.program)

			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("Acquire = %v, %v, want an error with %q", i, err, tt.err)
			}
		})
	}
}

func TestAcquireFailsWhenTheLockFails(t *testing.T) {
	t.Parallel()

	i, err := acquire(t.Context(), privateDir(t), "prog", func(int, int) error { return unix.ENOLCK })

	if !errors.Is(err, unix.ENOLCK) || !strings.HasPrefix(err.Error(), "lock the runtime directory: ") || i != nil {
		t.Errorf("acquire = %v, %v, want ENOLCK", i, err)
	}
}

func TestAcquireTriesAgainWhileTheLockIsHeld(t *testing.T) {
	t.Parallel()

	var calls []int
	flock := func(_, how int) error {
		calls = append(calls, how)
		switch len(calls) {
		case 1, 2:
			return unix.EWOULDBLOCK
		case 3:
			return unix.EINTR
		}
		return nil
	}
	i, err := acquire(t.Context(), privateDir(t), "prog", flock)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Release()

	if len(calls) != 4 {
		t.Errorf("flock was called %d times, want 4", len(calls))
	}
	for _, how := range calls {
		if how != unix.LOCK_EX|unix.LOCK_NB {
			t.Errorf("flock(%#x), want LOCK_EX|LOCK_NB", how)
		}
	}
}

// holdLock takes the lock of "prog" in runtimeDir, as another start that
// has not yet published does, until the test ends.
func holdLock(t *testing.T, runtimeDir string) {
	t.Helper()
	other, err := Acquire(t.Context(), runtimeDir, "prog")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(other.Release)
}

// endedContexts are the two ways a context ends while a lock is held: its
// deadline passes, or it is canceled.
var endedContexts = []struct {
	name string
	ctx  func(t *testing.T) context.Context
	want error
}{
	{name: "a deadline", want: context.DeadlineExceeded, ctx: func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(t.Context(), 3*lockPoll)
		t.Cleanup(cancel)
		return ctx
	}},
	{name: "a cancel", want: context.Canceled, ctx: func(t *testing.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		timer := time.AfterFunc(3*lockPoll, cancel)
		t.Cleanup(func() { timer.Stop(); cancel() })
		return ctx
	}},
}

func TestAcquireWaitsNoLongerThanItsContext(t *testing.T) {
	t.Parallel()

	for _, tt := range endedContexts {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runtimeDir := privateDir(t)
			holdLock(t, runtimeDir)

			i, err := Acquire(tt.ctx(t), runtimeDir, "prog")

			if !errors.Is(err, tt.want) || !strings.HasPrefix(err.Error(), "wait for the lock of the runtime directory: ") || i != nil {
				t.Errorf("Acquire = %v, %v, want %v", i, err, tt.want)
			}
		})
	}
}

func TestAcquireTakesAFreeLockUnderAnEndedContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	i, err := Acquire(ctx, privateDir(t), "prog")
	if err != nil {
		t.Fatalf("Acquire = %v, want the lock, which is free", err)
	}
	defer i.Release()
	if i.lockf == nil {
		t.Error("Acquire holds no lock")
	}
}

func TestCheckPrivateDirOfAnotherUser(t *testing.T) {
	t.Parallel()

	dir := privateDir(t)
	if err := checkPrivateDir(dir, os.Getuid()+1); err == nil || !strings.HasSuffix(err.Error(), "belongs to another user") {
		t.Errorf("checkPrivateDir = %v, want an error", err)
	}
}

func TestTwoStartsTakeTurns(t *testing.T) {
	t.Parallel()

	runtimeDir := privateDir(t)
	first, err := Acquire(t.Context(), runtimeDir, "prog")
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan *Instance)
	go func() {
		second, err := Acquire(t.Context(), runtimeDir, "prog")
		if err != nil {
			t.Error(err)
		}
		got <- second
	}()

	select {
	case <-got:
		t.Fatal("a second Acquire took the lock that the first holds")
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Publish("http://127.0.0.1:7374/?token=T"); err != nil {
		t.Fatal(err)
	}
	second := <-got
	defer second.Release()
	first.Release() // released already: nothing to do
}

func TestPublishWritesAPrivateRecord(t *testing.T) {
	t.Parallel()

	i := acquireIn(t)
	if err := i.Publish("http://127.0.0.1:7374/?token=T"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(i.recordPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != 0o600 {
		t.Errorf("the record has mode %v, want 0600", info.Mode())
	}
	data, err := os.ReadFile(i.recordPath())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"url":"http://127.0.0.1:7374/?token=T","pid":` + itoa(os.Getpid()) + "}\n"
	if string(data) != want {
		t.Errorf("the record holds %q, want %q", data, want)
	}
	if i.lockf != nil {
		t.Error("Publish kept the lock")
	}
}

func itoa(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}

func TestPublishRefuses(t *testing.T) {
	t.Parallel()

	i := acquireIn(t)
	for _, u := range []string{"http://example.org:80/", "http://[::1"} {
		if err := i.Publish(u); err == nil {
			t.Errorf("Publish(%q) = nil, want an error", u)
		}
	}
	if err := os.Mkdir(i.recordPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(i.recordPath(), "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := i.Publish("http://127.0.0.1:7374/"); err == nil || !strings.HasPrefix(err.Error(), "record the server: ") {
		t.Errorf("Publish over a directory = %v, want an error", err)
	}
	if i.lockf == nil {
		t.Error("a failed Publish dropped the lock")
	}
}

func TestRecordAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
		err  string
	}{
		{in: "http://127.0.0.1:7374/?token=T", want: "127.0.0.1:7374"},
		{in: "http://127.0.0.2:7374/", want: "127.0.0.2:7374"},
		{in: "http://[::1]:7374/", want: "[::1]:7374"},
		{in: "http://[::ffff:127.0.0.1]:7374/", want: "127.0.0.1:7374"},
		{in: "http://[::1", err: "parse the server's address: "},
		{in: "https://127.0.0.1:7374/", err: "no http address"},
		{in: "http://localhost:7374/", err: "no http address"},
		{in: "http://10.0.0.1:7374/", err: "no http address"},
		{in: "http://127.0.0.1/", err: "no http address"},
		{in: "http://127.0.0.1:0/", err: "no http address"},
		{in: "http://u:secret@127.0.0.1:7374/", err: `"http://u:xxxxx@127.0.0.1:7374/"`},
		{in: "http://127.0.0.1:7374/#x", err: "no http address"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := recordAddr(tt.in)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Errorf("recordAddr = %v, %v, want an error with %q", got, err, tt.err)
				}
				return
			}
			if err != nil || got.String() != tt.want {
				t.Errorf("recordAddr = %v, %v, want %s", got, err, tt.want)
			}
		})
	}
}

// listen returns the address of a new listener on 127.0.0.1.
func listen(t *testing.T) netip.AddrPort {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l.Addr().(*net.TCPAddr).AddrPort()
}

// fake makes the checks of i pass, or fail where a field says so.
type fake struct {
	dead, refused bool
	owner         int
	ownerErr      error
}

func (f fake) apply(i *Instance) {
	i.alive = func(int) bool { return !f.dead }
	i.dial = func(netip.AddrPort) bool { return !f.refused }
	i.owner = func(netip.AddrPort) (int, error) { return f.owner, f.ownerErr }
}

func TestRunning(t *testing.T) {
	t.Parallel()

	const url = "http://127.0.0.1:7374/?token=T"
	other := os.Getpid() + 1
	tests := []struct {
		name   string
		record any
		perm   os.FileMode
		fake   fake
		uid    int
		want   bool
	}{
		{name: "running", record: Record{URL: url, PID: other}, want: true},
		{name: "this process's", record: Record{URL: url, PID: os.Getpid()}},
		{name: "a process gone", record: Record{URL: url, PID: other}, fake: fake{dead: true}},
		{name: "nothing listens", record: Record{URL: url, PID: other}, fake: fake{refused: true}},
		{name: "another user listens", record: Record{URL: url, PID: other}, fake: fake{owner: os.Getuid() + 1}},
		{name: "the listener's owner unknown", record: Record{URL: url, PID: other}, fake: fake{ownerErr: errors.ErrUnsupported}},
		{name: "another host", record: Record{URL: "http://example.org:7374/", PID: other}},
		{name: "a record others can read", record: Record{URL: url, PID: other}, perm: 0o644},
		{name: "a record of another user", record: Record{URL: url, PID: other}, uid: os.Getuid() + 1},
		{name: "no URL", record: Record{PID: other}},
		{name: "no PID", record: Record{URL: url}},
		{name: "a negative PID", record: Record{URL: url, PID: -1}},
		{name: "a PID that is no number", record: map[string]string{"url": url, "pid": "1"}},
		{name: "no record"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			i := acquireIn(t)
			if tt.fake.owner == 0 {
				tt.fake.owner = os.Getuid()
			}
			tt.fake.apply(i)
			if tt.record != nil {
				perm := tt.perm
				if perm == 0 {
					perm = 0o600
				}
				data, err := json.Marshal(tt.record)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(i.recordPath(), data, perm); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(i.recordPath(), perm); err != nil {
					t.Fatal(err)
				}
			}
			if tt.uid != 0 {
				i.uid = tt.uid
			}

			r, ok := i.Running()

			if ok != tt.want {
				t.Fatalf("Running = %v, %v, want %v", r, ok, tt.want)
			}
			if ok && r != tt.record {
				t.Errorf("Running = %v, want %v", r, tt.record)
			}
			if !ok && r != (Record{}) {
				t.Errorf("Running = %v, false, want the zero Record", r)
			}
		})
	}
}

func TestRunningReadsNoOddRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, path string)
	}{
		{name: "a link", prepare: func(t *testing.T, path string) {
			target := filepath.Join(filepath.Dir(path), "target")
			writeFile(t, target, `{"url":"http://127.0.0.1:7374/","pid":1}`)
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a named pipe", prepare: func(t *testing.T, path string) {
			if err := unix.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "a directory", prepare: func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "too long", prepare: func(t *testing.T, path string) {
			writeFile(t, path, `{"url":"http://127.0.0.1:7374/`+strings.Repeat("x", maxRecord)+`","pid":1}`)
		}},
		{name: "no JSON", prepare: func(t *testing.T, path string) {
			writeFile(t, path, "url=http://127.0.0.1:7374/")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			i := acquireIn(t)
			fake{}.apply(i)
			tt.prepare(t, i.recordPath())

			if r, ok := i.Running(); ok {
				t.Errorf("Running = %v, true, want false", r)
			}
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRunningOnAHost checks a server of this user that listens, with the
// system's own checks: another process of the user, and the owner of the
// listener.
func TestRunningOnAHost(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("the owner of a listener is told on Linux only")
	}

	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	addr := listen(t)
	i := acquireIn(t)
	want := Record{URL: "http://" + addr.String() + "/?token=T", PID: cmd.Process.Pid}
	writeRecord(t, i, want, 0o600)

	if r, ok := i.Running(); !ok || r != want {
		t.Errorf("Running = %v, %v, want %v, true", r, ok, want)
	}
}

func TestAlive(t *testing.T) {
	t.Parallel()

	if !alive(os.Getpid()) {
		t.Error("alive(this process) = false")
	}
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if alive(cmd.Process.Pid) {
		t.Errorf("alive(%d), a process that ended, = true", cmd.Process.Pid)
	}
}

func TestDial(t *testing.T) {
	t.Parallel()

	addr := listen(t)
	if !dial(addr) {
		t.Errorf("dial(%s), a listener, = false", addr)
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := l.Addr().(*net.TCPAddr).AddrPort()
	_ = l.Close()
	if dial(closed) {
		t.Errorf("dial(%s), a closed port, = true", closed)
	}
}

func TestRetire(t *testing.T) {
	t.Parallel()

	t.Run("its own record", func(t *testing.T) {
		t.Parallel()

		i := acquireIn(t)
		if err := i.Publish("http://127.0.0.1:7374/"); err != nil {
			t.Fatal(err)
		}
		if err := i.Retire(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(i.recordPath()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the record is there after Retire: %v", err)
		}
		if i.lockf != nil {
			t.Error("Retire kept the lock")
		}
	})

	t.Run("the record of another process", func(t *testing.T) {
		t.Parallel()

		i := acquireIn(t)
		writeRecord(t, i, Record{URL: "http://127.0.0.1:7374/", PID: os.Getpid() + 1}, 0o600)
		i.Release()
		if err := i.Retire(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(i.recordPath()); err != nil {
			t.Errorf("Retire removed the record of another process: %v", err)
		}
	})

	t.Run("no record, the lock held", func(t *testing.T) {
		t.Parallel()

		i := acquireIn(t)
		if err := i.Retire(t.Context()); err != nil {
			t.Fatal(err)
		}
		if i.lockf != nil {
			t.Error("Retire kept the lock")
		}
	})

	t.Run("an ended context, the lock free", func(t *testing.T) {
		t.Parallel()

		i := acquireIn(t)
		if err := i.Publish("http://127.0.0.1:7374/"); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := i.Retire(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(i.recordPath()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the record is there after Retire: %v", err)
		}
	})

	for _, tt := range endedContexts {
		t.Run("the lock held, "+tt.name, func(t *testing.T) {
			t.Parallel()

			runtimeDir := privateDir(t)
			i, err := Acquire(t.Context(), runtimeDir, "prog")
			if err != nil {
				t.Fatal(err)
			}
			if err := i.Publish("http://127.0.0.1:7374/"); err != nil {
				t.Fatal(err)
			}
			holdLock(t, runtimeDir)

			err = i.Retire(tt.ctx(t))

			if !errors.Is(err, tt.want) || !strings.HasPrefix(err.Error(), "wait for the lock of the runtime directory: ") {
				t.Errorf("Retire = %v, want %v", err, tt.want)
			}
			if _, err := os.Lstat(i.recordPath()); err != nil {
				t.Errorf("Retire removed the record without the lock: %v", err)
			}
			if i.lockf != nil {
				t.Error("a failed Retire holds the lock")
			}
		})
	}

	t.Run("a lock that fails", func(t *testing.T) {
		t.Parallel()

		i := acquireIn(t)
		i.Release()
		i.flock = func(int, int) error { return unix.ENOLCK }
		if err := i.Retire(t.Context()); !errors.Is(err, unix.ENOLCK) {
			t.Errorf("Retire = %v, want ENOLCK", err)
		}
	})
}
