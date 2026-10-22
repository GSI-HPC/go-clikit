// SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de>
// SPDX-License-Identifier: Apache-2.0

package localweb_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"

	"github.com/GSI-HPC/go-clikit/localweb"
)

// A program binds a loopback address, puts the checks in front of its
// handler and serves until its context ends.
func ExampleListen() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b, err := localweb.Listen(ctx, "127.0.0.1:0", localweb.ListenOptions{})
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "Hello from the user's machine.")
	})
	h := localweb.Headers("")(localweb.Hosts(b.Answers, nil)(
		localweb.SameOrigin(localweb.Origin{Navigate: true})(mux)))
	done := make(chan error, 1)
	go func() { done <- localweb.Serve(ctx, b.Listeners, h, localweb.ServeOptions{}) }()

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	for _, host := range []string{b.Host(), "rebind.example"} {
		req, err := http.NewRequest(http.MethodGet, "http://"+b.Host()+"/", nil)
		if err != nil {
			log.Fatal(err)
		}
		req.Host = host
		res, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		fmt.Printf("%s: %s %s", res.Status, res.Header.Get("X-Frame-Options"), body)
	}

	cancel()
	fmt.Println(<-done)
	// Output:
	// 200 OK: DENY Hello from the user's machine.
	// 421 Misdirected Request: DENY This server answers to its loopback address only.
	// <nil>
}

// CrossSite gives the reason a request came from another site, for the
// program's log.
func ExampleCrossSite() {
	page := localweb.Origin{Navigate: true}
	for _, r := range []struct {
		method string
		header map[string]string
	}{
		{"GET", nil},
		{"POST", nil},
		{"POST", map[string]string{"Origin": "http://127.0.0.1:7374"}},
		{"POST", map[string]string{"Origin": "http://evil.example"}},
		{"GET", map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"GET", map[string]string{
			"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document",
		}},
	} {
		req := httptest.NewRequest(r.method, "http://127.0.0.1:7374/", nil)
		for k, v := range r.header {
			req.Header.Set(k, v)
		}
		reason := localweb.CrossSite(req, page)
		if reason == "" {
			reason = "let in"
		}
		fmt.Printf("%s %v: %s\n", r.method, r.header, reason)
	}
	// Output:
	// GET map[]: let in
	// POST map[]: no Origin on a POST
	// POST map[Origin:http://127.0.0.1:7374]: let in
	// POST map[Origin:http://evil.example]: Origin is http://evil.example
	// GET map[Sec-Fetch-Site:cross-site]: Sec-Fetch-Site is cross-site
	// GET map[Sec-Fetch-Dest:document Sec-Fetch-Mode:navigate Sec-Fetch-Site:cross-site]: let in
}

// The Host header of a request has to name the server by one of its
// addresses. The name localhost counts only while both loopback addresses
// are held on one port.
func ExampleBound_Answers() {
	b := &localweb.Bound{Addrs: []netip.AddrPort{
		netip.MustParseAddrPort("127.0.0.1:7374"),
		netip.MustParseAddrPort("[::1]:7374"),
	}}
	for _, host := range []string{
		"127.0.0.1:7374", "[::1]:7374", "localhost:7374", "LocalHost:7374",
		"localhost:7375", "localhost", "rebind.example:7374",
	} {
		fmt.Printf("%s: %t\n", host, b.Answers(host))
	}
	// Output:
	// 127.0.0.1:7374: true
	// [::1]:7374: true
	// localhost:7374: true
	// LocalHost:7374: true
	// localhost:7375: false
	// localhost: false
	// rebind.example:7374: false
}

// Hosts and SameOrigin take a Refuser to answer in the program's own form.
func ExampleHosts() {
	refuse := func(w http.ResponseWriter, _ *http.Request, f localweb.Refusal) {
		log.Printf("refused: %s", f.Reason)
		http.Error(w, `{"error":"`+strings.TrimSuffix(f.Message, ".")+`"}`, f.Status)
	}
	h := localweb.Hosts(func(host string) bool { return host == "127.0.0.1:7374" }, refuse)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") }))

	req := httptest.NewRequest(http.MethodGet, "http://rebind.example/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	fmt.Println(w.Code, strings.TrimSpace(w.Body.String()))
	// Output:
	// 421 {"error":"This server answers to its loopback address only"}
}
