// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !aix

package launch_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GSI-HPC/go-clikit/localweb/launch"
)

// A program opens the address of its server in the user's browser,
// unless it runs where no browser of the user's can open: then the user
// opens the printed address.
func ExampleOpener() {
	dir, _ := os.MkdirTemp("", "prog-ui-") // the Dir of an Instance in a program
	defer func() { _ = os.RemoveAll(dir) }()
	o := launch.Opener{
		Name: "prog ui", GOOS: "linux", Dir: dir,
		Getenv: func(name string) string { return map[string]string{"DISPLAY": ":0"}[name] }, // os.Getenv
		Start: func(name string, args ...string) error { // nil, which is launch.Detached, in a program
			fmt.Println(name, filepath.Base(args[0]))
			return nil
		},
	}

	if why := o.Skip(); why != "" {
		fmt.Println("Open the address above yourself:", why)
		return
	}
	file, err := o.Open("http://127.0.0.1:7374/?token=T")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.Remove(file) }() // when the server stops
	// Output:
	// xdg-open open-7374.html
}

// A second start of a program finds the server of the first, and opens
// it instead of starting another.
func ExampleAcquire() {
	runtimeDir, _ := os.MkdirTemp("", "run-") // $XDG_RUNTIME_DIR in a program
	defer func() { _ = os.RemoveAll(runtimeDir) }()

	inst, err := launch.Acquire(runtimeDir, "prog")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer inst.Release()
	if r, ok := inst.Running(); ok {
		fmt.Println("prog ui is already running:", r.URL)
		return
	}
	// Listen, then publish the address with the token.
	if err := inst.Publish("http://127.0.0.1:7374/?token=T"); err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = inst.Retire() }()
	fmt.Println("prog ui: http://127.0.0.1:7374/?token=T")
	// Output:
	// prog ui: http://127.0.0.1:7374/?token=T
}
