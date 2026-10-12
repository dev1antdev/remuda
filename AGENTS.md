# Working in remuda

remuda is a Go server with an embedded React SPA: a browser and phone
cockpit for the AI coding agents that **herdr** runs on the user's
machines. This file is *how to work here*, for any agent. `CLAUDE.md` is
the record of *what lives where and why* — every package, every
invariant — read its bullet before touching a package and change the
bullet when you change the behaviour. `README.md` is the user story.

## Build and check

Every Go compile (`go build`, `go vet`, `go test`) needs `web/dist`: the
SPA is embedded with `go:embed all:dist` (`web/embed.go`). Run `make web`
once after cloning and after every change under `web/`.

| To | Run |
|---|---|
| Build the SPA (before any Go compile) | `make web` |
| One binary with the SPA inside | `make build` → `bin/remuda` |
| Develop | `make server` (API on :7317) and `make dev` (Vite on :5173, proxies `/api` and `/ws`) |
| Go checks | `gofmt -l ./cmd ./internal` (must print nothing), `go vet ./...`, `go test ./...` |
| TypeScript check | `make typecheck` (`tsc --noEmit` + `go vet`) |
| Everything, as CI does (`.github/workflows/ci.yml`) | `make test && make typecheck`, or `docker build --target test .` |
| A release | `git tag -a vX.Y.Z -m 'remuda X.Y.Z' && make publish` — the Release workflow builds and publishes it |
| Binaries for every host | `make release`; `make deploy HOST=<ssh host>` |

Tests are plain `testing` and need no herdr: the ones that start a
throwaway herdr server skip when `herdr` is not on PATH. The web side has
no test runner; `tsc` is the check, and `web/src/markdown.tsx` is written
so it can be run through esbuild + node by hand.

**Done means**: gofmt clean, `go vet`, `go test` and `tsc` all pass, and
for anything a person sees, `make build && ./bin/remuda` and a look in
the browser. Nothing is done on compile alone. In Claude Code, `/verify`
runs the whole list.

## Hard rules

These protect the person's machine and the agents running on it. They are
not style.

- **herdr owns every process and status.** remuda keeps no session state
  and must keep working across its own restart while agents run. Never
  spawn, kill or track an agent yourself; ask herdr over its RPC.
- **`~/.claude` is read-only.** Transcripts are parsed, never written.
  Input reaches an agent only through herdr — `agent.prompt`, else
  `pane.send_text` + `pane.send_keys` — serialized per pane by
  `paneLock`. Never queue, retry or auto-send what a person typed. The one
  exception is the restart after an update the person ran (CLAUDE.md,
  `internal/restart`).
- **Never `--takeover`.** herdr allows one attached client per terminal;
  stealing the slot is the person's call only. A refusal is surfaced
  (`RefusedError`, 409 on HTTP, the WebSocket close reason), never retried
  with the flag.
- **A test herdr needs `HERDR_SOCKET_PATH`, never a changed `HOME`
  alone.** herdr ignores `HOME` for its socket: `HOME=/tmp/x herdr server
  stop` stopped the person's real server once. `internal/herdr/
  server_test.go` is the pattern. An agent working inside a herdr pane
  already has `HERDR_SOCKET_PATH` and `HERDR_SOCKET` in its environment,
  naming the person's real server: a test sets both itself and never
  counts on them being unset.
- **Never run the uninstall for real on the person's machine.** `remuda
  uninstall` and `scripts/uninstall.sh run` stop the service, and with
  `-herdr` the server and every agent in it. To look, use `-n` (the plan
  only reads). To test, `internal/setup/setup_test.go` is the pattern: a
  temp `HOME` whose `~/.local/bin` holds stand-ins for `launchctl`,
  `systemctl`, `herdr`, `brew` and `ssh`, checked to be what the script
  finds before anything runs; a full run belongs in a container.
- **Live checks touch only panes you created.** The person's own
  sessions are in the same roster. Create a scratch workspace through the
  API, drive that one, delete it when done (`/live-check` in Claude Code).
- **Every route is behind the token** except `/api/health` and
  `/api/login`. A new route goes on the guarded mux and resolves its
  machine through `machineFor` / `ready()`, never by trusting the body.
- **Nothing phones home** but the version checks (npm registry, GitHub
  releases; off with `-no-version-check`). No sudo anywhere. The uploads
  folder `<pane dir>/.remuda/uploads` is the only thing remuda writes into
  a project.
- **One design system** (`web/src/styles.css`): every `<button>` carries a
  family class, radii and type sizes come from the scales, icons are
  `icons.tsx` (never an emoji), transient status is a toast, nothing in a
  bar moves when state changes. Reuse a rule before writing one.

## Conventions

- **Go 1.27, standard library first.** `net/http` method patterns
  (`"GET /api/x"`), no framework. Errors go out through `httpErr` /
  `rpcErr`, JSON through `decode` / `writeJSON`. `go vet` is the linter,
  gofmt the formatter (a hook runs it in Claude Code). Tabs.
- **TypeScript, React 18 function components**, 2-space indent, double
  quotes, semicolons. Dependencies are react, react-dom and xterm; add
  nothing without a reason that survives review. Never `innerHTML`:
  Markdown and task descriptions render to React nodes. Per-device state
  is `usePref` (`prefs.ts`); every request goes through `api.ts`
  (`check` / `sendJSON`); every time string through `time.ts`.
- **Names carry the meaning; comments are rare.** A variable, function or
  type says what it is (`baseline`, `machine`, `summary` — never `b`,
  `mc`, `sum`). A comment is a one-line doc comment on a function or type
  saying what it does, or a note on a genuinely hard spot — a wire
  protocol, an ordering the code cannot show. Nothing above a line that
  restates it, no prose narrating why; if a block needs a paragraph,
  split it into named pieces instead.
- **Commit messages are one plain sentence** about what changed for the
  person, not a list of files: *"Chats follow an agent update: restarted
  with their sessions resumed"*, *"Machines can be renamed, the local one
  too"*.
- **Docs move with the code.** A behaviour change edits the matching
  bullet in `CLAUDE.md` in the same commit; a user-facing one touches
  `README.md` too.

## Where to look

| To change… | Start in |
|---|---|
| An HTTP route, the roster, the pane WebSocket | `internal/server/` (`server.go` holds the mux) |
| herdr RPC, the attach PTY, starting herdr | `internal/herdr/` |
| Remote machines, SSH, remote files | `internal/machines/`, `internal/sshx/` |
| What the sidebar says about an agent | `internal/radar/`, `web/src/radar.ts`, `Sidebar.tsx` |
| Pushes and their timing | `internal/push/`, `web/public/sw.js`, `web/src/push.ts` |
| Transcript parsing, chat turns | `internal/transcript/`, `web/src/Chat.tsx` |
| The terminal lens: keys, selection, mouse | `web/src/Terminal.tsx`, `KeyBar.tsx` |
| The Changes lens (review) | `internal/review/`, `web/src/Review.tsx` |
| Tasks mode | `internal/tasks/`, `web/src/Tasks.tsx`, `tasksApi.ts` |
| Updates and the restarts after them | `internal/health/`, `internal/jobs/`, `internal/restart/`, `web/src/Updates.tsx` |
| Installing and uninstalling: the installer, setup, the service, code-server | `internal/setup/` (the scripts in `scripts/`, embedded), `cmd/remuda/main.go` for the commands |
| The look | `web/src/styles.css`, `ui.tsx`, `icons.tsx`, `Select.tsx` |

## Agent tooling in this repository

- `.claude/settings.json` — Claude Code: the read-only and build commands
  that run without asking, a deny on writes to the transcripts under `~/.claude/projects` (`*.jsonl` only; the memory folder beside them stays writable),
  and two hooks in `.claude/hooks/`: `guard-bash.sh` refuses
  `--takeover`, an unscoped `herdr server stop` and a `HOME=…` herdr;
  `gofmt.sh` formats a Go file after every edit.
- `.claude/skills/` — `/verify` (the full check), `/live-check` (a
  scratch workspace against the real herdr, cleaned up afterwards),
  `/herdr` (reading herdr's state and RPC by hand).
- `GEMINI.md` is this file (a symlink). Codex, Cursor and Copilot read
  `AGENTS.md` as it is. The rules are the same for every agent.
