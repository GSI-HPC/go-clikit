// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package page

import "net/http"

// IsHTMX reports whether htmx sent r, which it marks with the header
// HX-Request: true. A handler answers htmx with a fragment, and a browser
// that sent a plain form or followed a link with a whole page.
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// Redirect sends the browser to the path to of this server after a
// request that changed something. htmx gets 204 No Content with the
// header HX-Redirect, on which it loads to as a whole page; a plain form
// gets 303 See Other, on which the browser loads to with a GET.
func Redirect(w http.ResponseWriter, r *http.Request, to string) {
	if IsHTMX(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// Retarget has htmx swap the answer into the element target, a CSS
// selector, in the way swap, such as "innerHTML", in place of the target
// and swap of the element that sent the request: a dialog shows why a
// request was refused in the place of its preview. It sets the headers
// HX-Retarget and HX-Reswap, before the handler writes the answer.
func Retarget(w http.ResponseWriter, target, swap string) {
	w.Header().Set("HX-Retarget", target)
	w.Header().Set("HX-Reswap", swap)
}
