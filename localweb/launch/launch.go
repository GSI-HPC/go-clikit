// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package launch starts a local web server the way its user expects: one
// server per user, which a second start opens instead of starting another,
// and the user's browser opened on its address, without the launch token
// in a command line.
//
// The address a program prints holds the launch token (localweb/session),
// and the argument list of a process is readable by every user of the
// machine. So an Opener never hands the address to the desktop's opener:
// it writes a file that only the user can read, which sends the browser on
// to the address, and opens that file.
//
// An Instance is the record of the server that runs, in a private
// directory of the user's, such as $XDG_RUNTIME_DIR/prog, and the lock
// that makes two starts take turns. A start waits for the lock no longer
// than its context:
//
//	inst, err := launch.Acquire(ctx, runtimeDir, "prog")
//	if err != nil {
//		return err
//	}
//	defer inst.Release()
//	if r, ok := inst.Running(); ok {
//		// Open r.URL, and return.
//	}
//	// Listen, and make the address with the token.
//	if err := inst.Publish(url); err != nil {
//		return err
//	}
//	defer inst.Retire(ctx)
//
// The kit reads no environment: the program passes the runtime directory,
// its name, the operating system and a lookup of the environment.
package launch

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
)

// Opener opens an address in the browser of the user's desktop. Its fields
// are what it takes from the process, so that a test can stand in for each.
type Opener struct {
	// Name is the program's name as the redirect page shows it, such as
	// "prog ui".
	Name string
	// GOOS names the operating system, as runtime.GOOS does.
	GOOS string
	// Getenv reads a variable of the environment, as os.Getenv does; nil
	// reads every variable as unset.
	Getenv func(string) string
	// Dir is the directory the redirect file is written to. It must be a
	// directory of the user's that no other user can enter, such as the
	// Dir of an Instance or one from os.MkdirTemp.
	Dir string
	// Start runs the desktop's opener with its arguments and returns once
	// it has started; nil is Detached.
	Start func(name string, args ...string) error
}

// Skip says why no browser can be opened from this process, and returns ""
// when one can. Under SSH, the browser that would start is one on the
// remote machine, which is not where the user sits; on Linux, a process
// without a display has no desktop to open anything in. In either case the
// user opens the printed address instead.
func (o Opener) Skip() string {
	if o.command() == "" {
		return "no opener is known for " + o.GOOS
	}
	getenv := o.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if getenv("SSH_CONNECTION") != "" {
		return "the session runs under SSH"
	}
	if o.GOOS == "linux" && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
		return "neither DISPLAY nor WAYLAND_DISPLAY is set"
	}
	return ""
}

// command names the program that opens a file in the user's default
// application, or "" on a system the kit knows none for.
func (o Opener) command() string {
	switch o.GOOS {
	case "linux":
		return "xdg-open"
	case "darwin":
		return "open"
	default:
		return ""
	}
}

// defaultStart is the Start of an Opener that sets none; a test stands in
// for it, so that no browser opens.
var defaultStart = Detached

// redirectPage is the file the opener is given. It holds no script and no
// style: a page opened from a file has no reason to run either, and the
// link is there for a browser that does not follow the refresh.
const redirectPage = `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta http-equiv="refresh" content="0; url=%[1]s">
<title>Opening %[2]s</title>
<p><a href="%[1]s">Open %[2]s</a></p>
`

// Open writes a file that sends the browser on to target, an http or
// https address, and hands it to the desktop's opener. It returns the path
// of the file, which holds the launch token: the program removes it when
// its server stops, and a later start on the same port replaces it. A
// failure leaves no file behind.
//
// The file is Dir/open-PORT.html, readable by the user alone. Open writes
// it under a temporary name and renames it into place, so it never
// follows a link someone put there.
func (o Opener) Open(target string) (string, error) {
	command := o.command()
	if command == "" {
		return "", errors.New("no opener is known for " + o.GOOS)
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("parse the address to open: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("the address to open is no http address: %q", u.Redacted())
	}
	if err := checkPrivateDir(o.Dir, os.Getuid()); err != nil {
		return "", err
	}
	name := "open.html"
	if port := u.Port(); port != "" {
		name = "open-" + port + ".html"
	}
	page := fmt.Sprintf(redirectPage, html.EscapeString(target), html.EscapeString(o.Name))
	file := filepath.Join(o.Dir, name)
	if err := writePrivate(file, []byte(page)); err != nil {
		return "", fmt.Errorf("write the redirect file: %w", err)
	}
	start := o.Start
	if start == nil {
		start = defaultStart
	}
	if err := start(command, file); err != nil {
		// Nothing will read the file, and it holds the token.
		_ = os.Remove(file)
		return "", fmt.Errorf("start %s: %w", command, err)
	}
	return file, nil
}

// writePrivate writes data to file, readable and writable by its owner
// alone: into a new file of a temporary name in the same directory, which
// it then renames to file. A rename replaces a link of that name, and
// never writes where the link points.
func writePrivate(file string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+".*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	err = errors.Join(err, f.Close())
	if err == nil {
		err = os.Rename(f.Name(), file)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}
