# herdr-ui — notes for agents

Self-hosted web/phone client for **herdr** (0.9+). Go server + embedded React
SPA. See README.md for the user story.

## Layout

- `cmd/herdr-ui/` — main; flags `-addr` (default `127.0.0.1:7317`), `-socket`,
  `-herdr-bin`, `-print-token`, `-version`.
- `internal/state/` — the one state dir (`<user config dir>/herdr-ui`, 0700)
  and the one atomic 0600 writer; token, machines.json, vapid.json and
  push-subscriptions.json all live there.
- `internal/herdr/` — everything herdr. `client.go`: newline-JSON RPC over
  the Unix socket, **one request = one connection** (herdr closes the socket
  after each response; never pool). `attach.go`: ref-counted
  `herdr terminal attach` PTYs (creack/pty), one per terminal shared by all
  viewers, 256KB replay tail, 45s linger after the last viewer leaves.
  `AttachManager.NewCommand` builds the attach process, so the same manager
  serves a local herdr exec or an `ssh -tt` to a remote one.
  **Never pass `--takeover`** — stealing herdr's attach slot is the user's
  call only. `Client.OnTransportError` fires for failures to *reach* herdr
  (not for herdr's own `*Error` refusals); a remote machine hooks it to
  mark itself dead, so handlers never call MarkDead. `PaneInfo` has
  `Dir()`, `AgentName()`, `DisplayTitle()`; status vocabularies are the
  `Status*`/`UI*` constants with `UIStatus(status, hasAgent)` as the one
  mapping.
- `internal/sshx/` — one OpenSSH `ControlMaster` per host (BatchMode: fail,
  never prompt), with socket forwarding (`-O forward -L`), remote exec, and
  the `ssh -tt … herdr terminal attach` command builder. herdr-ui implements
  no SSH itself.
- `internal/machines/` — the multi-PC registry. `local` is always present.
  **Remotes come from herdr itself**: `SyncHerdr` reads `herdr machine list
  --json` (the watcher calls it every 30s, with a 5s timeout) and adds
  enabled machines under **herdr's own id** with `Source: "herdr"` (not
  persisted, not removable here — `herdr machine remove` is the way).
  Manually added ones live in `machines.json`. Each remote connects in the
  background: dial master → one script reads `$HOME` and finds the herdr
  bin (incl. `~/.local/bin`) → RPC transport → `Ping`. **Two RPC
  transports**: a forwarded unix socket when the sshd allows streamlocal
  forwarding (gog), else one `ssh host nc -U <sock>` exec per request
  (roofr's sshd on :2222 answers "unknown channel type"). Remote
  transcripts are fetched by `stat` + `tail -c` delta into a small
  per-session byte cache (`Machine.Transcript` hides local vs remote);
  `ResolveSession` finds `<uuid>.jsonl` and caches the hit on both sides.
  `Machine.Ready()` reconnects a dead remote at most every 15s; a removed
  machine never reconnects and its watcher loop exits.
- `internal/transcript/` — read-only parser for Claude Code JSONL session
  files. Never write to `~/.claude`. Session resolution: herdr's
  `agent_session` (exact UUID) first, cwd-encoded dir scan as fallback.
- `internal/machines/files.go` — the file viewer's reads, local or over
  SSH, plus the one write: `SaveUpload`. `ResolvePath` accepts absolute, `~/`, or pane-relative
  and, on a relative miss, searches the pane folder by suffix (agents say
  `demo.mp4` for `docs/demo.mp4`); several hits come back as `candidates`.
  `GitDiff` = `git diff HEAD -- file` in the file's repo (`untracked` for new
  files, no `repo` outside git). Routes: `/api/fs/stat` (resolve + kind),
  `/api/fs/file` (bytes; text is always `text/plain`, CSP-sandboxed, 512KB
  text / 24MB raw caps), `/api/fs/diff`, `POST /api/fs/upload` (multipart
  `file` ×≤4, 25MB each → `<pane dir>/.herdr-ui/uploads/<time>-<tag>-<name>`,
  stem and extension sanitized separately, remote via `sshx.Conn.Put`).
  `.herdr-ui/.gitignore` (`*`) is written alongside so uploads never reach
  a commit. Uploads are the only thing herdr-ui writes into a project.
- `web/src/attachments.ts` — `useAttachments` (upload at once, chips with
  thumbnails), `filesFrom` (drop/paste → files; a pasted screenshot gets a
  name), `shellQuote`. The chat composer inserts `@<rel>` at the caret and
  strips it when the chip is removed; the terminal lens types the absolute
  paths shell-quoted. Nothing is sent until the user sends.
- Terminal size modes (`web/src/Terminal.tsx`): **fit** resizes the shared
  pane to this screen (herdr follows the smallest attached client — a phone
  would squeeze the desktop's view), **keep** attaches at the pane's own
  grid (`cols`/`rows` from the snapshot layouts, in the roster) and shrinks
  the font to fit the width, scrolling when it can't. Touch defaults to
  keep. Agent status lines (ccstatusline etc.) only fit in keep mode on a
  phone.
- `web/src/markdown.tsx` — dependency-free Markdown → React nodes (no HTML
  strings, nothing to sanitize). `parseBlocks` is pure (task-list state is
  decided there, not in render) and memoized per text; testable via
  esbuild + node.
- `web/src/prefs.ts` — `usePref` (localStorage-backed state), `isTouch`,
  `errMsg`, `basename`; `web/src/api.ts` — `check`/`sendJSON` for every
  request. The App derives the shown pane from a *requested* one (deep link,
  notification, click) falling back to the first agent pane; nothing fixes
  selection up in effects.
- `web/src/host.ts` — the host bridge for an embedding IDE: `?cwd=` opens
  the project's pane; `window.herdrHost.openFile(path, line, machine)` lets
  the host open files itself (herdr-ui falls back to its viewer when it
  returns false or the file is on another machine); `window.herdrUI.insert`
  / `selectFolder` let the host push text into the composer or pick a pane.
- `plugin/` — the JetBrains plugin (Kotlin, IntelliJ Platform Gradle Plugin
  2.19, `intellijIdea("2026.2.x")` — the Community/Ultimate split is gone
  since 2025.3 — plus `bundledPlugin("com.intellij.modules.jcef")`: in 2026
  JCEF is its own bundled plugin and `<depends>` on it is required, or
  `JBCefApp` is a ClassNotFoundException at runtime). Views are built on the
  EDT only; the probe runs on a pooled thread and every failure becomes a
  FallbackView (never a stuck "Connecting…"). `HerdrUiService` finds/starts the `herdr-ui`
  binary on the host and reads its token via `-print-token`;
  `EmbeddedView` is a JBCefBrowser with the `herdrHost` bridge injected on
  load; `FallbackView` is shown when JCEF is unavailable — in Remote
  Development the backend's JCEF needs `REMOTE_DEV_SERVER_JCEF_ENABLED=true`.
  Build: `make plugin`; the zip installs on the backend from disk.
- `web/src/paths.ts` — `findPaths` / `findUrls`, shared by the terminal link
  provider (`term.registerLinkProvider`) and the chat linkifier; keeps a
  `:line` suffix, drops trailing punctuation, never a path inside a URL.
  **File links only in an agent's pane** (`agent` prop); URLs everywhere,
  but Cmd/Ctrl+click in a shell pane — its clicks belong to the app. URLs
  open through `openUrl` in `host.ts` (host bridge, else a new tab).
- **The mouse model** (`Terminal.tsx`): herdr enables mouse tracking in
  every attach stream, so the DECSET for the tracking modes is intercepted
  and xterm never tracks — a plain drag selects. What the app asked for is
  synthesized instead: the wheel as scroll reports, and a click (press and
  release within 4px) as a press+release report at its cell, sent after
  xterm's mouseup so a click that opened a link is not also an app click.
  Drags never reach the app.
- `internal/watch/` — **the only poller**: every machine's
  `session.snapshot` (2s local, 4s remote) whether or not a browser is
  open. Handlers read its cache (`Snapshot` returns the last good snapshot
  plus the last error) and never ask herdr inline; mutations call `Refresh`.
  It emits a `push.Event` per pane status transition; the first snapshot of
  a machine only seeds state — a reconnect must not replay a day of alerts.
- `internal/push/` — Web Push (RFC 8291/8292 via webpush-go). VAPID keys at
  `<config>/herdr-ui/vapid.json` (a damaged file is an error, never a silent
  rotation — rotating orphans every device); subscriptions in
  `push-subscriptions.json`, 404/410 from the push service drops one. The
  scheduler is the point: `blocked` alerts after 10s, `done` (only from
  `working`) after 60s for turns ≥ 60s or 10s with `done:"always"`; **any
  status change on the pane cancels its pending alerts** — answered at the
  desk means no push. Delays are vars so `push_test.go` can shrink them.
  Routes: `GET /api/push` (public key), `POST/DELETE /api/push/subscribe`,
  `POST /api/push/test`.
- `web/public/sw.js` shows every push (Safari revokes silent subscriptions),
  silent while a window is visible; click → `select-pane` message to an open
  window or `openWindow(/?machine=&pane=)`; `pushsubscriptionchange`
  re-subscribes. `web/src/push.ts` is single-flight and re-registers on every
  load with permission granted. Push needs a secure context: localhost or
  `tailscale serve` HTTPS; iPhone needs the home-screen app.
- `internal/radar/` — the sidebar's reading of every agent, after
  hhdebb/herdr-radar's design and written here (`radar_test.go` covers the
  rules). The watcher feeds every snapshot into the `Tracker`; the roster
  carries `pane.radar = {state, last_active}` for agent panes. Rules: a
  blink through idle within 2.5s of working is still working; **done is held
  until seen** (herdr focused the pane, or a client opened its terminal /
  chat, or `POST /api/seen`); **a question is held until the agent works
  again** (unless it never had worked — then the dialog simply closed);
  `last_active` is the last moment herdr said working, recovered for panes
  older than herdr-ui from the session file's mtime (`ResolveSession` +
  `TranscriptVersion`, off the poller), persisted in `<state>/activity.json`.
  The idle tiers (fresh ≤15 min, stale ≥120 min, dimmed) are the UI's, from
  `last_active` and the clock. `web/src/radar.ts` is the pure model (vendor
  hues, tiers, marks, row labels, ordering: families by their liveliest
  agent, parent before worktrees, tabs by their liveliest pane, no-stamp
  last; a flat `recent` order; herdr's own), `web/src/Sidebar.tsx` draws it
  (one spinner clock for the braille frames and the pulsing question).
  `web/marks/*.svg` are the vendor logos (MIT packaging, see NOTICE.md),
  `npm run marks` regenerates `src/marks.ts`; drawn inline in currentColor.
- `internal/tasks/` — the **Tasks mode**: the user's own Vikunja (2.x) behind
  one proxy. `vikunja.json` (URL + API token, 0600) in the state dir;
  `Configure` probes `/api/v1/info` and `/projects` before keeping it.
  `/api/tasks/v1/<path>` forwards to `<url>/api/v1/<path>` with the token
  added — the token never reaches a browser. **PATCH is ours**: Vikunja's
  `POST /tasks/{id}` replaces the whole task (a body of `{done:true}` wipes
  the due date), so a PATCH fetches, merges and POSTs. Vikunja facts the UI
  relies on (verified on 2.7): `GET /tasks` is the cross-project list
  (`filter=done = false && project = 3`, `due_date < now/d+1d`,
  `labels in 1`, `s=` search, repeated `sort_by`/`order_by`, 50 per page,
  `X-Pagination-Total-Pages`); create is `PUT /projects/{id}/tasks`; labels
  go through `/tasks/{id}/labels`; saved filters arrive in `/projects` as
  pseudo-projects with **negative ids** and list through their list view
  (`/projects/{id}/views/{view}/tasks`). `web/src/Tasks.tsx` +
  `tasksApi.ts`; descriptions are Vikunja's HTML, rendered through an
  allow-list into React nodes (never innerHTML) and edited as plain text.
  The top-bar `mode-switch` (Agents | Tasks) is a per-device pref. **Send to
  agent** (a split button in the task; "for grooming" prefixes `/grill-me`)
  composes `taskPrompt` (title, description as text, the Vikunja link) and
  submits it through the existing `/api/submit` to an agent pane from the
  roster, remembering the last target per Vikunja project.
- `internal/auth/` — single bearer token; HttpOnly cookie; no accounts.
- `internal/server/` — HTTP routes + the pane WebSocket. `/api/health` and
  `/api/login` are mounted outside the token guard; everything else behind
  it. Every route resolves its machine through `machineFor` (query →
  `X-Herdr-UI-Machine` header → POST body `machine`) and `ready()`. The three
  create routes share `create()` (agent kind validated *before* any RPC,
  container made, agent started, watcher refreshed); the two close routes
  share `closeHandler`. The roster carries an ETag (304 when unchanged) and
  the agent list. Local files stream through `http.ServeContent` (Range for
  video); the WS uses per-message compression and coalesces queued chunks.
- `web/` — React + Vite + xterm.js SPA, embedded via `web/embed.go`
  (`go:embed all:dist`). Build with `make web` before `go build`.

## Invariants

- **herdr owns every process and status.** herdr-ui holds no session state;
  it must keep working after a restart while agents run.
- **The transcript is read-only**; input goes through herdr: `agent.prompt`
  preferred (refuses with `agent_blocked` while a menu is open — surface
  that as 409, never type into the menu), `pane.send_text` + 150ms +
  `pane.send_keys ["Enter"]` as the shell fallback. Everything per-pane is
  serialized via `paneLock`. Never queue or auto-send user input.
- **Terminal bytes go over WS binary frames**, control messages over JSON
  text frames. The attach PTY paints the screen on start; late joiners get
  the replay tail.
- herdr `pane.send_keys` grammar is lowercase with `+` modifiers: `enter`,
  `esc`, `up`, `down`, `tab`, `shift+tab`, `backspace`, `ctrl+c`… (verified
  live on 0.9.1; the server lowercases and allowlists). The REST `/api/keys`
  path is for the chat lens only. The terminal lens key bar never uses it:
  it types raw sequences through the live terminal's own input path
  (`TermApi.input`), so ⏎ = `ESC CR` (Alt+Enter → newline in Claude Code,
  verified), arrows follow DECCKM, and Ctrl/Alt are one-shot modifiers.
- Roster keeps herdr's hierarchy: workspace → tabs → panes, plus `worktree`
  (linked worktrees nest under their repo workspace in the sidebar by
  `repo_key`). Tabs render in the strip above the pane, never in the sidebar.
- `xterm` keeps a small local scrollback but its viewport scrollbar is hidden
  in CSS: an overlay scrollbar paints over the last column. `color-scheme:
  dark` + thin custom scrollbars everywhere else.
- Auth: every route except `/api/health` and `/api/login` requires the
  token. Keep it that way.

## Testing

`go vet ./...` + `make typecheck`. Live check against real herdr: create a
scratch workspace via `POST /api/sessions {"dir":"/tmp/x","agent":"claude"}`,
drive it via `/api/keys` / `/api/submit` / `/api/chat?pane=...`, then
`DELETE /api/sessions/{ws}`. Never type into panes you did not create — the
user's own sessions live in the same roster.
