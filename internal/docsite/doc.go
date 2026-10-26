// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

// Package docsite holds the tests that keep the documentation site in
// site/ true to the code (doc/decisions.md, decision 48): the recordings
// of the displays, the map of the packages and their imports, and the
// names the site links to on pkg.go.dev. It has no API.
//
// The site's generated files are rewritten with
//
//	go test ./internal/docsite -update
package docsite
