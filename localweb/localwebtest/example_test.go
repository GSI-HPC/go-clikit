// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localwebtest_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GSI-HPC/go-clikit/localweb/localwebtest"
)

// A test signs in with the launch token, reads the CSRF token from a page,
// and sends a form as the page would.
func ExampleLogin() {
	_ = func(t *testing.T, handler http.Handler, token string) {
		srv := httptest.NewServer(handler)
		defer srv.Close()
		c := localwebtest.Login(t, srv.URL, token)
		resp := c.Get(t, "/csrf")
		csrf, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		c.Header.Set("X-Prog-CSRF", string(csrf))
		resp = c.PostForm(t, "/runs/1/stop", url.Values{"signal": {"INT"}})
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("stop: %s, want 303 See Other", resp.Status)
		}
	}
}

// A test reads the events of a stream one by one.
func ExampleReader() {
	r := localwebtest.NewReader(strings.NewReader("retry: 2000\n\nevent: runs\ndata: <ul>\ndata: </ul>\n\n: keep-alive\n\n"))
	for {
		e, err := r.Next()
		if err != nil {
			fmt.Println(err)
			break
		}
		fmt.Printf("%s %q\n", e.Name, e.Data)
	}
	fmt.Println("retry", r.Retry)
	// Output:
	// runs "<ul>\n</ul>"
	// EOF
	// retry 2000
}
