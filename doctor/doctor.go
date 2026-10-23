// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package doctor writes the checks of a doctor command: what a program
// looked at on the machine, whether it is ok, a warning or failed, and what
// a person can do about it. Check holds one check, Worst and Count sum up a
// list of them, and Write writes them for a person:
//
//	✓ Docker Engine: 28.1.1 (>= 25.0)
//	✗ cgroupv2: not mounted (sind requires cgroupv2)
//	! inotify: max_user_instances 128 (optional)
//
//	Boot with the unified cgroup hierarchy.
//
//	Raise the limit: sudo sysctl fs.inotify.max_user_instances=1024
//
// A Check encodes as JSON with the keys name, status, detail and
// remediation, the last two left out when empty, so that a program writes
// the same list for a script, with cliout.WriteJSON or encoding/json.
//
// The status words ok, warning and failed, the JSON keys and the text Write
// writes are interfaces of the kit, which a script or a test of a program
// may read; changing any of them is a breaking change (decision 38).
//
// What stays in the program: the checks, their names and texts, the
// command and its flags, and the exit status of a failed check, which a
// program can pick from Worst. The package has no exit code (decision 21).
package doctor

import (
	"io"
	"strings"

	"github.com/GSI-HPC/go-clikit/termtext"
)

// Status is how a check came out. Its value is one of the words of
// StatusOK, StatusWarning and StatusFailed, as it is in JSON.
//
// A Status that is none of the three counts as failed in Worst and Count,
// and Write marks it as failed, so that a mistake in a program, such as a
// misspelt or zero Status, fails closed instead of passing for ok.
type Status string

// The statuses of a check.
const (
	// StatusOK is a check that passed.
	StatusOK Status = "ok"
	// StatusWarning is a check that did not pass but that the program can
	// do without, such as an optional feature or a limit that only large
	// work reaches.
	StatusWarning Status = "warning"
	// StatusFailed is a check that did not pass and that the program
	// needs.
	StatusFailed Status = "failed"
)

// Check is one thing a doctor command looked at. It encodes as JSON with
// the keys name, status, detail and remediation.
type Check struct {
	// Name says what was checked, such as "Docker Engine". Write writes it
	// escaped with termtext.Escape, on one line.
	Name string `json:"name"`
	// Status is how the check came out.
	Status Status `json:"status"`
	// Detail says what the check found, such as a version or the reason
	// it failed. It is optional. Write writes it escaped with
	// termtext.Escape, on the line of the check.
	Detail string `json:"detail,omitempty"`
	// Remediation says what a person can do to make the check pass, such
	// as the commands to run. It is optional and may have more than one
	// line. Write writes it once after all the checks, however many
	// checks have it, escaped with termtext.EscapeLines.
	Remediation string `json:"remediation,omitempty"`
}

// Worst returns the worst status of checks: StatusFailed when a check
// failed, else StatusWarning when one is a warning, else StatusOK, which
// it returns for no checks too. A status that is none of the three counts
// as failed.
func Worst(checks []Check) Status {
	failed, warned := Count(checks)
	switch {
	case failed > 0:
		return StatusFailed
	case warned > 0:
		return StatusWarning
	}
	return StatusOK
}

// Count returns how many checks failed and how many are warnings, as a
// caption such as "5 checks, 1 failed, 2 warnings" needs. A status that is
// none of the three counts as failed.
func Count(checks []Check) (failed, warned int) {
	for _, c := range checks {
		switch c.Status {
		case StatusOK:
		case StatusWarning:
			warned++
		default:
			failed++
		}
	}
	return failed, warned
}

// Write writes checks for a person, in a text that is an interface of the
// kit (decision 38). It writes one line for each check, in the order of
// checks:
//
//	MARK NAME
//	MARK NAME: DETAIL
//
// the second when the detail is not empty. The mark is ✓ for StatusOK, !
// for StatusWarning and ✗ for StatusFailed and any other status. The name
// and the detail are escaped with termtext.Escape, so that neither can
// start a line that seems to be another check.
//
// After the lines it writes each distinct remediation once, in the order
// it first appears, after a blank line, escaped with termtext.EscapeLines.
// The line feeds and carriage returns at its start and its end are left
// out, so that one blank line sets it off, whether its lines end in \n or
// \r\n, and "fix" and "fix\n" are the same remediation. A remediation
// that is empty without them is skipped. Write writes nothing for no
// checks, and returns the error of the writer.
func Write(w io.Writer, checks []Check) error {
	var b strings.Builder
	for _, c := range checks {
		b.WriteString(mark(c.Status))
		b.WriteByte(' ')
		b.WriteString(termtext.Escape(c.Name))
		if c.Detail != "" {
			b.WriteString(": ")
			b.WriteString(termtext.Escape(c.Detail))
		}
		b.WriteByte('\n')
	}
	seen := make(map[string]bool)
	for _, c := range checks {
		r := strings.Trim(c.Remediation, "\r\n")
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		b.WriteByte('\n')
		b.WriteString(termtext.EscapeLines(r))
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// mark returns the mark Write writes before the name of a check.
func mark(s Status) string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusWarning:
		return "!"
	}
	return "✗"
}
