// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package launch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// privateDir returns a new directory of mode 0700: t.TempDir makes one
// that the umask leaves open to others.
func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// env returns a Getenv of the variables vars.
func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestSkip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		goos string
		vars map[string]string
		want string
	}{
		{name: "Linux on a display", goos: "linux", vars: map[string]string{"DISPLAY": ":0"}},
		{name: "Linux on Wayland", goos: "linux", vars: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}},
		{name: "Linux without a display", goos: "linux", want: "neither DISPLAY nor WAYLAND_DISPLAY is set"},
		{name: "Linux under SSH", goos: "linux", vars: map[string]string{"DISPLAY": ":0", "SSH_CONNECTION": "10.0.0.1 22 10.0.0.2 22"},
			want: "the session runs under SSH"},
		{name: "macOS", goos: "darwin"},
		{name: "macOS under SSH", goos: "darwin", vars: map[string]string{"SSH_CONNECTION": "x"}, want: "the session runs under SSH"},
		{name: "Windows", goos: "windows", vars: map[string]string{"DISPLAY": ":0"}, want: "no opener is known for windows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := Opener{GOOS: tt.goos, Getenv: env(tt.vars)}
			if got := o.Skip(); got != tt.want {
				t.Errorf("Skip() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("no Getenv", func(t *testing.T) {
		t.Parallel()

		o := Opener{GOOS: "linux"}
		if got, want := o.Skip(), "neither DISPLAY nor WAYLAND_DISPLAY is set"; got != want {
			t.Errorf("Skip() = %q, want %q", got, want)
		}
	})
}

// started records the commands an Opener starts.
type started struct{ calls [][]string }

func (s *started) start(name string, args ...string) error {
	s.calls = append(s.calls, append([]string{name}, args...))
	return nil
}

func TestOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		goos    string
		target  string
		command string
		file    string
	}{
		{name: "Linux", goos: "linux", target: "http://127.0.0.1:7374/?token=T", command: "xdg-open", file: "open-7374.html"},
		{name: "macOS", goos: "darwin", target: "http://[::1]:7375/?token=T", command: "open", file: "open-7375.html"},
		{name: "no port", goos: "linux", target: "https://localhost/?token=T", command: "xdg-open", file: "open.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var s started
			o := Opener{Name: "prog ui", GOOS: tt.goos, Getenv: env(nil), Dir: privateDir(t), Start: s.start}
			file, err := o.Open(tt.target)
			if err != nil {
				t.Fatal(err)
			}

			if want := filepath.Join(o.Dir, tt.file); file != want {
				t.Errorf("Open = %q, want %q", file, want)
			}
			if len(s.calls) != 1 || len(s.calls[0]) != 2 || s.calls[0][0] != tt.command || s.calls[0][1] != file {
				t.Errorf("started %q, want [[%s %s]]", s.calls, tt.command, file)
			}
			info, err := os.Lstat(file)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
				t.Errorf("the redirect file has mode %v, want a regular file of mode 0600", info.Mode())
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			escaped := strings.ReplaceAll(tt.target, "&", "&amp;")
			for _, want := range []string{
				`<meta http-equiv="refresh" content="0; url=` + escaped + `">`,
				`<a href="` + escaped + `">Open prog ui</a>`,
				`<title>Opening prog ui</title>`,
			} {
				if !strings.Contains(string(data), want) {
					t.Errorf("the redirect file lacks %q:\n%s", want, data)
				}
			}
			if strings.Contains(string(data), "<script") || strings.Contains(string(data), "style") {
				t.Errorf("the redirect file holds a script or a style:\n%s", data)
			}
		})
	}
}

func TestOpenEscapes(t *testing.T) {
	t.Parallel()

	var s started
	o := Opener{Name: `<b>"prog"</b>`, GOOS: "linux", Dir: privateDir(t), Start: s.start}
	file, err := o.Open(`http://127.0.0.1:7374/?token=T&next="><script>x</script>`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "<script>") || strings.Contains(string(data), "<b>") {
		t.Errorf("the redirect file does not escape the address or the name:\n%s", data)
	}
}

func TestOpenReplacesALink(t *testing.T) {
	t.Parallel()

	dir := privateDir(t)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, "open-7374.html")); err != nil {
		t.Fatal(err)
	}

	var s started
	o := Opener{GOOS: "linux", Dir: dir, Start: s.start}
	file, err := o.Open("http://127.0.0.1:7374/")
	if err != nil {
		t.Fatal(err)
	}

	if data, _ := os.ReadFile(victim); string(data) != "keep" {
		t.Errorf("Open wrote through a link: the target holds %q", data)
	}
	if info, err := os.Lstat(file); err != nil || !info.Mode().IsRegular() {
		t.Errorf("the redirect file is %v, %v, want a regular file", info, err)
	}
}

func TestOpenRefuses(t *testing.T) {
	t.Parallel()

	open := t.TempDir()
	if err := os.Chmod(open, 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		goos   string
		dir    string
		target string
		err    string
	}{
		{name: "an unknown system", goos: "plan9", target: "http://127.0.0.1:1/", err: "no opener is known for plan9"},
		{name: "a malformed address", goos: "linux", target: "http://[::1", err: "parse the address to open"},
		{name: "a file address", goos: "linux", target: "file:///etc/passwd", err: `no http address: "file:///etc/passwd"`},
		{name: "no host", goos: "linux", target: "http:///x", err: "no http address"},
		{name: "a password", goos: "linux", target: "ftp://u:secret@h/", err: `"ftp://u:xxxxx@h/"`},
		{name: "a directory others can enter", goos: "linux", dir: open, target: "http://127.0.0.1:1/", err: "has mode 0755; it must be 0700"},
		{name: "a missing directory", goos: "linux", dir: filepath.Join(open, "missing"), target: "http://127.0.0.1:1/", err: "no such file or directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var s started
			dir := tt.dir
			if dir == "" {
				dir = privateDir(t)
			}
			o := Opener{GOOS: tt.goos, Dir: dir, Start: s.start}
			file, err := o.Open(tt.target)

			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("Open = %q, %v, want an error with %q", file, err, tt.err)
			}
			if len(s.calls) != 0 {
				t.Errorf("started %q, want nothing", s.calls)
			}
		})
	}
}

func TestOpenRemovesTheFileWhenTheOpenerFails(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	o := Opener{GOOS: "linux", Dir: privateDir(t), Start: func(string, ...string) error { return boom }}
	file, err := o.Open("http://127.0.0.1:7374/?token=T")

	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "start xdg-open: ") || file != "" {
		t.Errorf("Open = %q, %v, want boom", file, err)
	}
	if entries, _ := os.ReadDir(o.Dir); len(entries) != 0 {
		t.Errorf("Open left %v behind", entries)
	}
}

func TestOpenFailsWhenTheFileCannotBeWritten(t *testing.T) {
	t.Parallel()

	var s started
	o := Opener{GOOS: "linux", Dir: privateDir(t), Start: s.start}
	// A directory of the name cannot be replaced by a file.
	if err := os.Mkdir(filepath.Join(o.Dir, "open-7374.html"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Dir, "open-7374.html", "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	file, err := o.Open("http://127.0.0.1:7374/")

	if err == nil || !strings.HasPrefix(err.Error(), "write the redirect file: ") || file != "" {
		t.Errorf("Open = %q, %v, want an error", file, err)
	}
	if entries, _ := os.ReadDir(o.Dir); len(entries) != 1 {
		t.Errorf("Open left %v behind, want the directory alone", entries)
	}
}

// TestOpenStartsDetachedByDefault does not run in parallel: it stands in
// for defaultStart.
func TestOpenStartsDetachedByDefault(t *testing.T) {
	var s started
	defaultStart = s.start
	defer func() { defaultStart = Detached }()

	o := Opener{GOOS: "linux", Dir: privateDir(t)}
	if _, err := o.Open("http://127.0.0.1:7374/"); err != nil {
		t.Fatal(err)
	}

	if len(s.calls) != 1 || s.calls[0][0] != "xdg-open" {
		t.Errorf("started %q, want xdg-open", s.calls)
	}
}

func TestWritePrivateInAMissingDirectory(t *testing.T) {
	t.Parallel()

	err := writePrivate(filepath.Join(t.TempDir(), "missing", "f"), []byte("x"))

	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("writePrivate = %v, want os.ErrNotExist", err)
	}
}

func TestDetached(t *testing.T) {
	t.Parallel()

	if err := Detached("true"); err != nil {
		t.Errorf("Detached(true) = %v", err)
	}
	if err := Detached(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Detached of a missing program = %v, want os.ErrNotExist", err)
	}
}
