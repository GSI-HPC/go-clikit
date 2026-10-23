// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package doctor_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/doctor"
)

func TestTheStatusWords(t *testing.T) {
	// The words are an interface (decision 38).
	for status, want := range map[doctor.Status]string{
		doctor.StatusOK:      "ok",
		doctor.StatusWarning: "warning",
		doctor.StatusFailed:  "failed",
	} {
		if string(status) != want {
			t.Errorf("status %q, want %q", status, want)
		}
	}
}

func TestAChecksJSONHasTheExactKeys(t *testing.T) {
	tests := []struct {
		name  string
		check doctor.Check
		want  string
	}{
		{
			name: "every field",
			check: doctor.Check{
				Name:        "cgroupv2",
				Status:      doctor.StatusFailed,
				Detail:      "not mounted",
				Remediation: "Boot with the unified hierarchy.",
			},
			want: `{"name":"cgroupv2","status":"failed","detail":"not mounted","remediation":"Boot with the unified hierarchy."}`,
		},
		{
			name:  "the empty ones left out",
			check: doctor.Check{Name: "scp client", Status: doctor.StatusOK},
			want:  `{"name":"scp client","status":"ok"}`,
		},
		{
			name:  "a warning",
			check: doctor.Check{Name: "inotify", Status: doctor.StatusWarning, Detail: "128"},
			want:  `{"name":"inotify","status":"warning","detail":"128"}`,
		},
		{
			name:  "the zero check",
			check: doctor.Check{},
			want:  `{"name":"","status":""}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.check)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("JSON\n%s\nwant\n%s", got, tt.want)
			}
			var back doctor.Check
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatal(err)
			}
			if back != tt.check {
				t.Errorf("decoded %+v, want %+v", back, tt.check)
			}
		})
	}
}

func TestAListOfChecksDecodesFromItsJSON(t *testing.T) {
	in := `[{"name":"a","status":"ok"},{"name":"b","status":"warning","detail":"d","remediation":"r"}]`
	var got []doctor.Check
	if err := json.Unmarshal([]byte(in), &got); err != nil {
		t.Fatal(err)
	}
	want := []doctor.Check{
		{Name: "a", Status: doctor.StatusOK},
		{Name: "b", Status: doctor.StatusWarning, Detail: "d", Remediation: "r"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decoded %+v, want %+v", got, want)
	}
}

func TestWorstAndCount(t *testing.T) {
	ok := doctor.Check{Name: "ok", Status: doctor.StatusOK}
	warning := doctor.Check{Name: "warning", Status: doctor.StatusWarning}
	failed := doctor.Check{Name: "failed", Status: doctor.StatusFailed}
	tests := []struct {
		name           string
		checks         []doctor.Check
		worst          doctor.Status
		failed, warned int
	}{
		{name: "no checks", checks: nil, worst: doctor.StatusOK},
		{name: "ok", checks: []doctor.Check{ok, ok}, worst: doctor.StatusOK},
		{name: "a warning", checks: []doctor.Check{ok, warning, ok}, worst: doctor.StatusWarning, warned: 1},
		{name: "a failure", checks: []doctor.Check{ok, failed}, worst: doctor.StatusFailed, failed: 1},
		{name: "failed beats warning", checks: []doctor.Check{warning, failed, warning}, worst: doctor.StatusFailed, failed: 1, warned: 2},
		{name: "failed beats warning after it", checks: []doctor.Check{failed, warning}, worst: doctor.StatusFailed, failed: 1, warned: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := doctor.Worst(tt.checks); got != tt.worst {
				t.Errorf("Worst = %q, want %q", got, tt.worst)
			}
			f, w := doctor.Count(tt.checks)
			if f != tt.failed || w != tt.warned {
				t.Errorf("Count = %d failed, %d warned, want %d, %d", f, w, tt.failed, tt.warned)
			}
		})
	}
}

func TestAStatusThatIsNoneOfTheThreeCountsAsFailed(t *testing.T) {
	for _, status := range []doctor.Status{"", "OK", "warn", "fail", "unknown"} {
		t.Run(string(status), func(t *testing.T) {
			checks := []doctor.Check{
				{Name: "ok", Status: doctor.StatusOK},
				{Name: "warning", Status: doctor.StatusWarning},
				{Name: "odd", Status: status},
			}
			if got := doctor.Worst(checks); got != doctor.StatusFailed {
				t.Errorf("Worst = %q, want %q", got, doctor.StatusFailed)
			}
			f, w := doctor.Count(checks)
			if f != 1 || w != 1 {
				t.Errorf("Count = %d failed, %d warned, want 1, 1", f, w)
			}
			var b strings.Builder
			if err := doctor.Write(&b, checks[2:]); err != nil {
				t.Fatal(err)
			}
			if want := "✗ odd\n"; b.String() != want {
				t.Errorf("Write wrote %q, want %q", b.String(), want)
			}
		})
	}
}

// TestTheTextWriteWrites holds the text of Write byte for byte, as it is
// an interface (decision 38).
func TestTheTextWriteWrites(t *testing.T) {
	checks := []doctor.Check{
		{Name: "Docker Engine", Status: doctor.StatusOK, Detail: "28.1.1 (>= 25.0)"},
		{Name: "Docker daemon", Status: doctor.StatusFailed, Detail: "rootless mode", Remediation: "Use a rootful daemon:\n\ndocker context use default\n"},
		{Name: "scp client", Status: doctor.StatusOK},
		{Name: "inotify", Status: doctor.StatusWarning, Detail: "max_user_instances 128 (optional)", Remediation: "Raise the limit:\n\tsudo sysctl fs.inotify.max_user_instances=1024"},
		{Name: "sshuttle", Status: doctor.StatusWarning, Detail: "not in PATH"},
		{Name: "Docker host", Status: doctor.StatusFailed, Detail: "unknown", Remediation: "Use a rootful daemon:\n\ndocker context use default"},
		{Name: "cgroupv2", Status: doctor.StatusFailed, Remediation: "\n\n"},
		{Name: "systemd", Status: doctor.StatusWarning, Remediation: "\r\n\nRun under systemd.\r\n\n"},
		{Name: "cgroup driver", Status: doctor.StatusWarning, Remediation: "Run under systemd."},
	}
	const want = "" +
		"✓ Docker Engine: 28.1.1 (>= 25.0)\n" +
		"✗ Docker daemon: rootless mode\n" +
		"✓ scp client\n" +
		"! inotify: max_user_instances 128 (optional)\n" +
		"! sshuttle: not in PATH\n" +
		"✗ Docker host: unknown\n" +
		"✗ cgroupv2\n" +
		"! systemd\n" +
		"! cgroup driver\n" +
		"\n" +
		"Use a rootful daemon:\n" +
		"\n" +
		"docker context use default\n" +
		"\n" +
		"Raise the limit:\n" +
		"\tsudo sysctl fs.inotify.max_user_instances=1024\n" +
		"\n" +
		"Run under systemd.\n"
	var b strings.Builder
	if err := doctor.Write(&b, checks); err != nil {
		t.Fatal(err)
	}
	if b.String() != want {
		t.Errorf("Write wrote\n%s\nwant\n%s", b.String(), want)
	}
}

func TestWriteWritesNothingForNoChecks(t *testing.T) {
	var b strings.Builder
	if err := doctor.Write(&b, nil); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 0 {
		t.Errorf("Write wrote %q, want nothing", b.String())
	}
}

func TestWriteEscapesWhatItWrites(t *testing.T) {
	tests := []struct {
		name  string
		check doctor.Check
		want  string
	}{
		{
			name:  "a detail that would start a line",
			check: doctor.Check{Name: "node", Status: doctor.StatusFailed, Detail: "down\n✓ node: up"},
			want:  "✗ node: down\\n✓ node: up\n",
		},
		{
			name:  "a detail that would rewrite the terminal",
			check: doctor.Check{Name: "node", Status: doctor.StatusOK, Detail: "up\x1b]0;title\x07\r"},
			want:  "✓ node: up\\x1b]0;title\\x07\\r\n",
		},
		{
			name:  "a name with control characters",
			check: doctor.Check{Name: "a\tb\u009bc", Status: doctor.StatusWarning},
			want:  "! a\\tb\\u009bc\n",
		},
		{
			name:  "a remediation keeps its lines and tabs only",
			check: doctor.Check{Name: "x", Status: doctor.StatusFailed, Remediation: "run:\n\tfix \x1b[2J\xff"},
			want:  "✗ x\n\nrun:\n\tfix \\x1b[2J\\xff\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			if err := doctor.Write(&b, []doctor.Check{tt.check}); err != nil {
				t.Fatal(err)
			}
			if b.String() != tt.want {
				t.Errorf("Write wrote %q, want %q", b.String(), tt.want)
			}
		})
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWriteReturnsTheErrorOfTheWriter(t *testing.T) {
	want := errors.New("disk full")
	err := doctor.Write(failingWriter{want}, []doctor.Check{{Name: "a", Status: doctor.StatusOK}})
	if !errors.Is(err, want) {
		t.Errorf("Write returned %v, want %v", err, want)
	}
}
