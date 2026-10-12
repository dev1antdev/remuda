---
name: verify
description: Run remuda's full check before calling work done — gofmt, go vet, go test, tsc and, when the SPA changed, a real build. Use after any code change, before a commit, or when asked whether the tree is green.
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
3. `go vet ./...`
4. `go test ./...` — no herdr needed. The tests that start a throwaway
   herdr server skip when `herdr` is not on PATH; one that reaches the
   person's real socket is a bug in the test (`HERDR_SOCKET_PATH`, see
   `internal/herdr/server_test.go`).
5. `make typecheck` — `tsc --noEmit` in `web/` plus `go vet`.
6. For a change a person can see: build and look.

   ```sh
   make build
   REMUDA_STATE_DIR=$(mktemp -d) ./bin/remuda -addr 127.0.0.1:7417 -no-version-check
   ```

   A second instance beside the running one, with its own token, tasks
   and pushes, sharing the real herdr. `REMUDA_STATE_DIR=<that dir>
   ./bin/remuda -print-token` prints its token; `http://127.0.0.1:7417/
   ?token=<token>` logs the browser in. Do not open the person's panes'
   terminals from it (one attach per terminal, theirs holds it); a
   scratch workspace is what `/live-check` makes. For iterative UI work
   `make server` + `make dev` (Vite on :5173) is faster.

7. Report the commands run and their result, in a line each.

## When it fails

- `pattern all:dist: no matching files found` → step 1.
- A `go vet` complaint is fixed, not suppressed.
- A `tsc` error in a component after an API change → the type in
  `web/src/api.ts` changes first, the components follow.
- A test that passes alone and fails in `./...` shares state through the
  real state dir or the real herdr: scope it (`REMUDA_STATE_DIR`,
  `HERDR_SOCKET_PATH`, `t.TempDir()`).
