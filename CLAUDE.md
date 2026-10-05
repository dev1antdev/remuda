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
- `web/src/paths.ts` — one path-matching regex shared by the terminal link
  provider (`term.registerLinkProvider`) and the chat linkifier; keeps a
  `:line` suffix, drops trailing punctuation and URL paths.
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
