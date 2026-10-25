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
| The page's unit tests (Vitest on the pure modules) | `make webtest` (`npm test` in `web/`; `make test` runs it too) |
| Everything, as CI does (`.github/workflows/ci.yml`) | `make test && make typecheck`, or `docker build --target test .` |
| The end-to-end smoke: a throwaway herdr, a remuda of its own, headless Chrome (`scripts/e2e.sh`, `e2e/smoke.mjs`) | `make e2e` (needs herdr, node and Chrome; nothing of yours is touched) |
| A pull request | against `release`: `gh pr create --base release` |
| A release | `git checkout main && git merge --ff-only release && git tag -a vX.Y.Z -m 'remuda X.Y.Z' && make publish` — the Release workflow builds and publishes it |
| Binaries for every host | `make release`; `make deploy HOST=<ssh host>` |

Two long-lived branches. **`release`** is where the next release comes
together: every pull request targets it (Dependabot's too), CI runs on
every push to it, and before a tag it is built and tried as a whole —
the checks, `make build`, a real session in the browser, and the
end-to-end smoke — `make e2e`, the `e2e` job in ci.yml, gated on
`github.ref == 'refs/heads/release'` and `workflow_dispatch`, so it
runs on every push to `release` and by hand (`gh workflow run ci.yml
--ref release`) before a tag. **`main`** is what is out: at a
release it receives `release` by fast-forward and nothing else, so a
clone of `main`, the README on GitHub and the binary on every machine
agree. `make publish` refuses any branch but `main`
(`PUBLISH_BRANCH=<branch>` overrides). A hotfix is made on `main` and
merged into `release` the same day, or the next fast-forward fails.

Tests are plain `testing` and need no herdr: the ones that start a
throwaway herdr server skip when `herdr` is not on PATH, and a package
that runs scripts is tested on a `shell.Fake` with a recorded answer
(`internal/review/wire_test.go`, `internal/jobs/jobs_test.go`). The web
side runs Vitest on its pure modules, a `*.test.ts` beside each (`lib/`,
`features/roster/radar.ts`, `features/review/hunks.ts`,
`features/terminal/engine/selection.ts`, …) in a plain node environment:
nothing under test touches the DOM, and a piece that would is split so
the logic sits in a module without React.

**Done means**: gofmt clean, `go vet`, `go test`, `tsc` and `npm test` all pass, and
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
  `sessions.Input`. Never queue, retry or auto-send what a person typed. The one
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
- **TypeScript, React 19 function components**, 2-space indent, double
  quotes, semicolons. Dependencies are react, react-dom, xterm and
  radix-ui (menus, tooltips, popovers: the primitives whose keyboard and
  focus behaviour is not worth writing twice; `ui/Menu.tsx` and
  `ui/Tip.tsx` are the only places that import it); add nothing without
  a reason that survives review. Never `innerHTML`:
  Markdown and task descriptions render to React nodes. Per-device state
  is `usePref` (`lib/prefs.ts`); every request goes through `lib/http.ts`
  (`check` / `getJSON` / `sendJSON`) from a feature's own `api.ts`; a
  screen reads the server through `useQuery` (`lib/query.ts`) and the
  page's state through the store (`app/store.ts`); every time string
  through `lib/time.ts`. A file stays under about three hundred lines: a
  lens is a folder of pieces, not one component.
- **Names carry the meaning; comments are rare.** A variable, function or
  type says what it is (`baseline`, `machine`, `summary` — never `b`,
  `mc`, `sum`). A comment is a one-line doc comment on a function or type
  saying what it does, or a note on a genuinely hard spot — a wire
  protocol, an ordering the code cannot show. Nothing above a line that
  restates it, no prose narrating why; if a block needs a paragraph,
  split it into named pieces instead.
- **Contexts, one port, one adapter.** `internal/server` is the HTTP
  adapter and holds no logic: a handler reads the request, asks one
  service and writes the answer; the use case lives in its context
  package (`sessions`, `chat`, `review`, `updates`, `tasks`, …). A package
  that only runs scripts on a machine takes a `shell.Host`, never a
  `*machines.Machine`, and builds its scripts with `internal/script`.
  Packages hear each other through `internal/events`; the subscriptions
  are listed once, in `internal/app`. `docs/architecture.md` draws the
  map; a new feature is a package with its use cases, a file in
  `internal/server` with its routes, and a line in `app.go`.
- **The page is laid out by feature.** `web/src/lib/` (what every screen
  uses and no screen owns), `ui/` (the design system's pieces),
  `features/<name>/` (one folder per lens or panel: its server calls in
  its own `api.ts`, its pure logic in modules with tests beside them, its
  screen in pieces), `app/` (the shell: the store, the URL, the header,
  the pane area, the overlays). A dependency points down that list; a
  feature never imports another feature's screens; `app/` is the only
  place that composes them. The terminal is an engine outside React
  (`features/terminal/engine/`) with `Terminal.tsx` as its mount.
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
| How it all fits, where a new feature goes | `docs/architecture.md`, `internal/app/app.go` |
| An HTTP route, the roster, the pane WebSocket | `internal/server/` (one file per context; `server.go` holds the mux and the helpers) |
| What people type into panes, the agents remuda starts | `internal/sessions/` |
| A script a machine runs, local or over ssh | `internal/script/` (building it), `internal/shell/` (the port, the fake for tests) |
| Who hears what | `internal/events/`, the subscriptions in `internal/app/app.go` |
| The page's own state: the selected pane, the mode, the lens, the URL | `web/src/app/store.ts`, `url.ts`, `current.ts` |
| How a screen reads the server | `web/src/lib/query.ts` (`useQuery`), the feature's `api.ts`; the roster through `web/src/features/roster/useRoster.tsx` |
| herdr RPC, the attach PTY, starting herdr | `internal/herdr/` |
| Remote machines, SSH, remote files | `internal/machines/`, `internal/sshx/` |
| What the sidebar says about an agent | `internal/radar/`, `web/src/features/roster/` (`radar.ts` the model, `Sidebar.tsx`, `rows.tsx`) |
| Pushes and their timing | `internal/push/`, `web/public/sw.js`, `web/src/features/alerts/push.ts` |
| Transcript parsing, the conversation behind a pane | `internal/transcript/`, `internal/chat/`, `web/src/features/chat/` |
| The terminal lens: keys, selection, mouse | `web/src/features/terminal/engine/` (the terminal outside React), `Terminal.tsx`, `KeyBar.tsx` |
| The Changes lens (review) | `internal/review/`, `web/src/features/review/` |
| Tasks mode | `internal/tasks/` (`start.go` starts an agent on a task), `web/src/features/tasks/` |
| Updates and the restarts after them | `internal/updates/`, `internal/health/`, `internal/jobs/`, `internal/restart/`, `web/src/features/updates/Updates.tsx` |
| Installing and uninstalling: the installer, setup, the service, code-server | `internal/setup/` (the scripts in `scripts/`, embedded), `cmd/remuda/main.go` for the commands |
| The look | `web/src/styles.css`, `web/src/ui/` |

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
