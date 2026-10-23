// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package htmx holds the scripts a local web page of the kit runs: htmx,
// its extension for Server-Sent Events, and idiomorph with its htmx
// extension, which swaps a fragment by changing only what changed. They
// are the files their projects publish, byte for byte, under the
// Zero-Clause BSD licence (LICENSES/0BSD.txt), and a test holds each to
// its checksum.
//
// A program serves them as the static files of localweb/assets, and puts
// Config into the page, so that htmx keeps to the
// Content-Security-Policy of localweb.Headers:
//
//	vendor, err := assets.New(htmx.Files(), "/htmx/", assets.Options{})
//	mux.Handle("GET /htmx/", vendor)
//
//	<meta name="htmx-config" content='{{.HTMXConfig}}'>
//	<script src="{{vendor .HTMX}}" defer></script>
//
// A new version of a script comes in a minor release of the kit, which
// names it.
package htmx

import (
	"embed"
	"io/fs"
)

// The names of the files in Files.
const (
	// Script is htmx itself.
	Script = "htmx.min.js"
	// SSE is the extension "sse", which swaps the events of a stream
	// into the elements that name them.
	SSE = "htmx-ext-sse.js"
	// Morph is idiomorph with the extension "morph", a swap that keeps
	// what did not change, such as the focus and the scroll position.
	Morph = "idiomorph-ext.min.js"
)

// The versions of the files in Files.
const (
	Version          = "2.0.11"
	SSEVersion       = "2.2.4"
	IdiomorphVersion = "0.7.4"
)

// Config is the configuration of htmx a page sets in its meta element
// htmx-config. htmx evaluates no expression and runs no script it swaps
// in, which the Content-Security-Policy refuses anyway; it adds no style
// of its own, which the policy refuses too; it sends requests to its own
// origin only; and it keeps no copy of a page for its history, as what a
// page shows of the user's machine must not outlive the session. As the
// back and forward buttons then ask the server for the page, htmx does not
// mark that request with HX-Request, so that a handler answers it with the
// whole page (decision 41).
const Config = `{"allowEval":false,"allowScriptTags":false,"includeIndicatorStyles":false,"selfRequestsOnly":true,"historyCacheSize":0,"historyRestoreAsHxRequest":false}`

//go:embed htmx.min.js htmx-ext-sse.js idiomorph-ext.min.js
var files embed.FS

// Files returns the scripts, under the names Script, SSE and Morph.
func Files() fs.FS { return files }
