---
name: verify
description: Run remuda's full check before calling work done — gofmt, go vet, go test, tsc, the page's unit tests and, when the SPA changed, a real build. Use after any code change, before a commit, or when asked whether the tree is green.
---

# Verify

From the repository root. Stop at the first failure and fix it; a step
you skipped is reported as skipped, never as passed.

1. `make web` — if anything under `web/` changed or `web/dist` is
   missing. Every Go compile embeds that folder (`web/embed.go`), so
   without it `go vet` fails with `pattern all:dist: no matching files`.
2. `gofmt -l ./cmd ./internal` — must print nothing. Run `gofmt -w` on
   what it names. (In Claude Code a hook formats a Go file after each
   edit; this catches the rest.)
3. `go vet ./...`, and `GOOS=windows GOARCH=amd64 go vet ./...`: remuda.exe is the
   helper a cockpit reaches on a Windows PC, so a syscall of one system
   goes in a `_unix.go` / `_windows.go` pair (`internal/proc`, `internal/state`).
4. `go test ./...` — no core needs to be running first. The tests that
   start a throwaway remuda-core (`internal/core/coretest`) skip without
   `RC_BIN` or `remuda-core` on PATH; one that reaches the person's real
   core is a bug in the test (`RC_SESSION`, `RC_SOCKET` and the XDG
   folders are the test's own, as `coretest.Server` sets them).
5. `make typecheck` — `tsc --noEmit` in `web/` plus `go vet`.
6. `make webtest` — Vitest on the page's pure modules (`npm test` in
   `web/`; `make test` runs it too).
7. For a change a person can see: build and look.

   ```sh
   make build
   REMUDA_STATE_DIR=$(mktemp -d) ./bin/remuda -addr 127.0.0.1:7417 -no-version-check
   ```

   A second instance beside the running one, with its own token, tasks
   and pushes, sharing the real remuda-core; with `REMUDA_STATE_DIR` set it
   places no agent door, so the real service's `agent-key` / `agent-url`
   on every machine stay (`-door` would open one). `REMUDA_STATE_DIR=<that dir>
   ./bin/remuda -print-token` prints its token; `http://127.0.0.1:7417/
   ?token=<token>` logs the browser in. Do not type into the person's
   panes from it: a terminal is live and shared, so a key or a resize
   reaches the agent they are working with; a scratch workspace is what
   `/live-check` makes. As it connects, the scratch instance marks the
   panes it places to be kept (`terminal.keep`) so a reboot restores
   them — the one write it makes to the person's core. For iterative UI work
   `make server` + `make dev` (Vite on :5173) is faster.

8. Report the commands run and their result, in a line each.

## When it fails

- `pattern all:dist: no matching files found` → step 1.
- A `go vet` complaint is fixed, not suppressed.
- A `tsc` error in a component after an API change → the type in the
  feature's `api.ts` (`web/src/features/<name>/api.ts`; the roster's in
  `features/roster/api.ts`) changes first, the components follow.
- A test that passes alone and fails in `./...` shares state through the
  real state dir or the person's core: scope it (`REMUDA_STATE_DIR`,
  `RC_SESSION` / `RC_SOCKET` / the XDG folders as `coretest.Server` sets
  them, `t.TempDir()`).
