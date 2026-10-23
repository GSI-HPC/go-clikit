// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package doctor_test

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/GSI-HPC/go-clikit/doctor"
)

// A doctor command makes its checks, writes them for a person with a
// caption of the counts, and picks its exit status from the worst of them.
// A detail is what the program found and may quote another program, so
// it is escaped.
func ExampleWrite() {
	const pull = "Pull the node image:\n\ndocker pull ghcr.io/gsi-hpc/sind-node"
	checks := []doctor.Check{
		{Name: "Docker Engine", Status: doctor.StatusOK, Detail: "28.1.1 (>= 25.0)"},
		{Name: "node image", Status: doctor.StatusFailed, Detail: "not found\x1b[2J", Remediation: pull},
		{Name: "inotify", Status: doctor.StatusWarning, Detail: "max_user_instances 128 (optional)"},
		{Name: "sshuttle", Status: doctor.StatusWarning, Detail: "not in PATH"},
	}

	if err := doctor.Write(os.Stdout, checks); err != nil {
		fmt.Println(err)
	}
	failed, warned := doctor.Count(checks)
	fmt.Printf("\n%d checks, %d failed, %d warnings\n", len(checks), failed, warned)
	if doctor.Worst(checks) == doctor.StatusFailed {
		fmt.Println("exit status 1")
	}
	// Output:
	// ✓ Docker Engine: 28.1.1 (>= 25.0)
	// ✗ node image: not found\x1b[2J
	// ! inotify: max_user_instances 128 (optional)
	// ! sshuttle: not in PATH
	//
	// Pull the node image:
	//
	// docker pull ghcr.io/gsi-hpc/sind-node
	//
	// 4 checks, 1 failed, 2 warnings
	// exit status 1
}

// The same checks as JSON, for a script: the words and keys are fixed.
func ExampleCheck() {
	check := doctor.Check{Name: "inotify", Status: doctor.StatusWarning, Detail: "max_user_instances 128"}
	out, err := json.Marshal(check)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(out))
	// Output:
	// {"name":"inotify","status":"warning","detail":"max_user_instances 128"}
}

// Worst fails closed: a status that is none of the three counts as failed.
func ExampleWorst() {
	fmt.Println(doctor.Worst(nil))
	fmt.Println(doctor.Worst([]doctor.Check{{Status: doctor.StatusOK}, {Status: doctor.StatusWarning}}))
	fmt.Println(doctor.Worst([]doctor.Check{{Status: doctor.StatusOK}, {Status: "fine"}}))
	// Output:
	// ok
	// warning
	// failed
}
