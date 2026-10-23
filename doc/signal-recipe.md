<!-- SPDX-FileCopyrightText: 2026 GSI Helmholtz Centre for Heavy Ion Research GmbH <http://www.gsi.de> -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# The signal context of main

The kit handles no signals
([decision 21](decisions.md#21-the-command-lines-glue-for-progress-is-the-kits)):
`forbidigo` refuses `signal.Notify` and its relatives in every package. The
program's `main` owns them. Four programs carry the same twenty lines; this
is the shape.

```go
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := run(ctx)
	stop() // a second Ctrl-C now ends the process the usual way
	os.Exit(exitCode(err))
}
```

## Rules

- **One context.** `run` gets `ctx` and hands it down. Every pool, every
  child process and every request is made under it, so one interrupt ends
  them all, and `fanout` reports the items it did not start as skipped.
- **Interrupt once.** The first signal cancels `ctx`. A program that starts
  child commands sends them `SIGINT` once, starts none during the shutdown,
  and uses `cmd.WaitDelay` so that a child that ignores it is killed
  rather than waited for.
- **The second signal is the default.** `stop()` after `run` returns, or a
  second `NotifyContext` that is released at the first signal, gives back
  the default action so that a person can end a stuck shutdown.
- **Exit code in the program.** `exitCode` maps the error to the program's
  numbers. `cobratree.IsUsage` marks a usage error, and the program's
  `Classify` hook gives `progress` the skips and failures
  ([decision 21](decisions.md#21-the-command-lines-glue-for-progress-is-the-kits),
  [decision 24](decisions.md#24-the-strict-command-tree-is-the-kits-in-cobratree)).
  An interrupt exits with the number the program documents, `130` by
  convention.
- **Deferred work runs.** `os.Exit` skips deferred calls, so `run` returns
  first and closes the display and the event log, as `cliprogress.Session`
  does, and only then does `main` exit.
- **A server shuts down gracefully.** A program with a local web server
  calls `Shutdown` with a short deadline when `ctx` is done, then closes
  the SSE streams, then removes its runtime files.
