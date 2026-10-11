# herdr-ui — notes for agents

Self-hosted web/phone client for **herdr** (0.9+). Go server + embedded React
SPA. See README.md for the user story.

## Layout

- `cmd/herdr-ui/` — main; flags `-addr` (default `127.0.0.1:7317`), `-socket`,
  `-herdr-bin`, `-print-token`, `-version`, `-tailscale` /
  `-tailscale-hostname`, `-no-version-check`, `-code-port`; subcommands
  `search` and `usage` for the deployed binary on a remote machine, and
  `setup` (see herdr below).
- `internal/state/` — the one state dir (`<user config dir>/herdr-ui`, 0700,
  or `HERDR_UI_STATE_DIR` for a second instance beside the real one)
  and the one atomic 0600 writer; token, machines.json, vapid.json and
  push-subscriptions.json all live there.
- `internal/herdr/` — everything herdr. `client.go`: newline-JSON RPC over
  the Unix socket, **one request = one connection** (herdr closes the socket
  after each response; never pool). `attach.go`: ref-counted
  `herdr terminal attach` PTYs (creack/pty), one per terminal shared by all
  viewers, 45s linger after the last viewer leaves. **No byte replay** for a
  viewer joining a live attach: TUIs draw with relative cursor moves, so a
  tail from an arbitrary byte misplaces everything after it (the footer
  and prompt box of Claude Code vanish). A joiner gets a repaint instead —
  the PTY is resized by a row and back, herdr and the app redraw.
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
  Manually added ones live in `machines.json`. A name of the user's own
  for any machine (the local one too) lives in `labels.json` and shows
  through `Machine.Label()` everywhere (`Cfg.Label` stays what the
  registry said); `PATCH /api/machines/{id} {label}` sets it, empty goes
  back. The sidebar's per-machine ⋯ menu (`MachineMenu`) has Rename, Copy
  SSH host and, for a manually added one, Remove. Each remote connects in the
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
  **Compaction moves the conversation to a new file** (its first message's
  `parentUuid` is a record `uuid` in the old one) while herdr keeps naming
  the old id, so the chat follows `Machine.FollowContinuation(path)` —
  `transcript.Continuation` locally, `ContinuationScript` over ssh —
  re-checked a minute at a time per file. The chat lens follows the bottom
  only when the reader is there and not selecting or dragging; an upward
  wheel or finger lets go at once, and `TurnView` re-renders only when a
  turn's content changed, so a selection survives the polls.
- `internal/machines/files.go` — the file viewer's reads, local or over
  SSH, plus the one write: `SaveUpload`. `ResolvePath` accepts absolute, `~/`, or pane-relative
  and, on a relative miss, searches the pane folder by suffix (agents say
  `demo.mp4` for `docs/demo.mp4`); several hits come back as `candidates`.
  `GitDiff` = `git diff HEAD -- file` in the file's repo (`untracked` for new
  files, no `repo` outside git). `ListDirs(path, hidden)` lists a folder's
  subfolders (repositories marked by a `.git`; `""`/`~` is home) for the
  folder picker behind the New workspace dialog's browse button
  (`web/src/FolderPicker.tsx`: crumbs, subfolders, the machine's known
  workspaces as quick picks, hidden on request). Routes: `/api/fs/dirs`,
  `/api/fs/stat` (resolve + kind),
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
- Terminal mouse and selection (`Terminal.tsx`): Claude Code's full-screen
  mode keeps its own scrollback and tracks the mouse; its DECSETs are
  intercepted so xterm never tracks (a drag selects), and the wheel / a
  finger on a phone become mouse reports — the pixels add up, one report
  per cell height, as a native terminal does. Because the app repaints the
  screen on every scroll, the **selection is kept in content rows** and
  every row seen while it exists is remembered by text: each settled
  repaint is located in that memory (the offset most unique rows agree on,
  and at least 30% of the screen must be recognised — the app's own header
  and status rows always vote for the old place), the selection is drawn
  there or hidden when the screen is somewhere unseen, a drag past the
  top/bottom edge scrolls the app and grows the selection, and copy reads
  the remembered rows (trailing spaces trimmed) wherever the screen went.
  xterm drops its own selection on any user input or a buffer switch; the
  content selection survives that and ends only on a new mousedown or
  typing. A drag is ours from its first move (xterm reports a selection
  only on mouseup, so a wheel under a held button would go unseen): the
  anchor is set on mousedown, the head follows the pointer, our select()
  ends xterm's own drag; a click that never moves is left to xterm. The ⬚ selection layer (phone long-press) is screen-only.
- **Nothing in a bar moves when state changes.** The header is
  `[burger][modes][title — takes the slack][usage pill][lens switch][search][bell]`:
  the lens group is anchored right, the pill grows leftwards into the
  title's slack, the three lens buttons have equal widths so a badge
  widens nothing, and the Changes button is disabled (not removed) when
  there is no folder. A lens's own controls (the terminal's size, width,
  selection, text size, key bar) live in the tab strip's right slot
  (`TabStrip tools=`), never in the header. Transient status — offline,
  a newer server, errors, "Copied N" — is a toast in the bottom corner
  (`Toasts` in ui.tsx), never a pill in a bar. Lenses stay mounted once the
  selected pane has shown them (`.lens-pane[hidden]`, scoped to that pane
  — a lens never mounts hidden, since a terminal opened unseen would
  attach at 80×24 and resize the shared pane): the terminal keeps its
  attach, the chat its scroll and no refetch, Changes its draft; a lens
  gets `active` and polls or refits only when on screen.
- `internal/health` — each machine's vitals (Claude Code / herdr /
  herdr-ui versions, how Claude Code was installed, disk, load, uptime)
  from one POSIX script per machine through `Machine.Shell`, every 15
  minutes and on demand (`POST /api/machines/{id}/health`); the newest
  Claude Code, Codex and Gemini CLI (npm registry) and herdr (GitHub
  releases) every 6 hours unless `-no-version-check` — the only outbound
  requests herdr-ui makes.
  The roster carries `health` per machine and `latest`; the sidebar's ⓘ
  (`HealthButton` in Sidebar.tsx) turns amber on an update, a lagging
  remote herdr-ui, or a nearly full disk.
- **Peers** (`internal/server/peer.go`): herdr allows one attached client
  per terminal ("already has an attached client; retry with --takeover"),
  so two herdr-ui reaching the same pane — the machine's own and this one
  over SSH — collide. The health script also probes `127.0.0.1:7317/api/
  health` on the machine and reads its token file; a running herdr-ui
  becomes `Machine.Peer()` (port, token, version; stale after an hour).
  With a peer, `/ws/pane` is relayed to the peer's `/ws/pane` (frames
  piped both ways, its close reason passed on) and the HTTP transport's
  stream is opened there (`sid = <machine>~<peer sid>`, every later
  request forwarded), over `Machine.ForwardAddr(port)` — the same port
  forward as code-server. herdr prints its refusal *into the PTY* and
  exits, so `attach.go` watches a fresh attach's first chunks
  (`watchStart`: a refusal, or 300 ms / 512 bytes of real output) and
  `Subscribe` returns a `*RefusedError` (`herdr.Refused(err)`) instead
  of painting it; a refusal triggers one more probe (`peerAfterRefusal`,
  at most every 30 s); still no peer → the refusal is the WebSocket close
  reason (≤120 bytes; the client writes it on the screen at every retry)
  or a 409 on the HTTP transport. A plain GET of `/ws/pane` (the client's
  proxy probe) is 426 before anything attaches. A peer's requests carry
  the bearer token and no Origin, so csrfGuard lets them through. Never
  `--takeover`.
- **A throwaway herdr server for a test needs `HERDR_SOCKET_PATH`, never a
  changed `HOME`.** herdr ignores `HOME` for its socket: `HOME=/tmp/x herdr
  server stop` stops the user's real server (it happened). The env var is
  what `herdr server`, `status server` and `server stop` all honour;
  `internal/herdr/server_test.go` is the pattern.
- **herdr is herdr-ui's to run.** `herdr.EnsureServer(bin, socket)` pings
  and, when nothing answers, starts `herdr server` (the headless server)
  detached, at most once per two minutes — at startup and whenever the
  watcher's local poll fails (`Watcher.LocalDown`); a remote machine's
  `connect()` does the same over ssh (`herdr.StartServerScript`) before
  giving up. `herdr-ui setup [-host H] [-tailscale]` (`internal/setup`,
  two embedded POSIX scripts: `setup.sh` installs herdr with brew or its
  installer, starts the server, installs `herdr integration` hooks for
  the agents present; `install-service.sh` is the service installer that
  used to live in deploy/) is the one-command install, locally or over
  ssh. `internal/jobs` runs a shell script on a machine in the background
  with its output streamed (`Machine.Stream`, `sshx.Conn.Stream`) for
  the page to poll (`GET /api/jobs/{id}`); `health.Plan(what, report)`
  is the update command for herdr (brew or `herdr update --handoff`),
  claude / codex / gemini (brew, npm or the native updater, by where the
  binary lives), or every agent; `GET /api/machines/{id}/updates` lists
  them, `POST …/update {what}` starts one; `what=herdr-ui` copies this
  very binary to a remote of the same OS/CPU and restarts its service.
  `Updates.tsx` shows the list, the command before "Run", the live log.
  Nothing uses sudo; everything runs as the user through the shell
  herdr-ui already has on the machine.
- `internal/tsnetx` — `-tailscale` joins the tailnet as a tsnet node
  (userspace, no tailscaled) named `-tailscale-hostname`, serves the same
  handler over HTTPS on :443 with the tailnet's certificate (:80
  redirects), state under `<state dir>/tailscale/`. The status
  (needs_login with the URL, running with the URL, error) rides on the
  roster as `tailscale`; the App shows a login toast. `TS_AUTHKEY` skips
  the login. The loopback listener stays.
- The build's version is injected into the bundle (`__HERDR_UI_VERSION__`,
  from `HERDR_UI_VERSION` set by `make web`) and the roster answer carries
  the server's in `X-Herdr-UI-Version`; a page older than its server shows
  a "new version — reload" pill. After a deploy, a stale tab is told.
- `web/src/markdown.tsx` — dependency-free Markdown → React nodes (no HTML
  strings, nothing to sanitize). `parseBlocks` is pure (task-list state is
  decided there, not in render) and memoized per text; testable via
  esbuild + node.
- `web/src/prefs.ts` — `usePref` (localStorage-backed state; `readPref`
  keeps a stored value only in the fallback's shape), `isTouch`, `errMsg`,
  `basename`/`dirname`, `useDebounced`; `web/src/api.ts` — `check`/`sendJSON` for every
  request. The App derives the shown pane from a *requested* one (deep link,
  notification, click) falling back to the first agent pane; nothing fixes
  selection up in effects.
- `web/src/host.ts` — `copyText` (Clipboard API, else execCommand) and
  `openUrl` (a new tab). Nothing embeds herdr-ui any more: the JetBrains
  plugin is gone, and with it the `window.herdrHost` bridge and `?cwd=`.
- `internal/review/` — the **Changes lens**: what moved in a workspace's
  folder since the reviewer last looked. A baseline per `machine|folder`
  (`review.json`: HEAD then + blob hashes of the files dirty then) is set
  by `POST /api/review/seen`; `GET /api/review?dir=` runs one shell (local
  `sh -c` or over ssh via `Machine.Shell`) for branch, HEAD, commits
  `baseline..HEAD`, `git diff --numstat baseline`, untracked files, the
  current blob hashes and each commit's resulting blobs — a file is
  *fresh* when its blob moved since the baseline (or there is none); a
  commit is *seen* when every blob it brought is the one reviewed (a
  commit of reviewed work, an amend, a rebase). "Mark reviewed" records
  the blob of every file the review showed — changed since the previous
  baseline's commit, committed meanwhile or not, and untracked — so that
  history rewritten under the reviewer brings nothing new. `GET
  /api/review/diff?dir=&path=` is one file's diff from the blob the
  baseline recorded for it (only the edits since the look, not the whole
  commit), else against the baseline commit (an untracked file whole).
  The lens shows fresh files first and folds the rest behind "N files
  unchanged since you looked"; seen commits are dimmed. The
  watcher refreshes `review.Badges` a minute apart, one shell per machine
  over `server.WorkspaceDirs`, and the roster carries `session.dir` and
  `session.review {changed, fresh}` for the sidebar badge. `Review.tsx`
  draws it: commits, files (fresh first), hunks with a comment box whose
  text goes to the pane's agent through `/api/submit` as
  "Review feedback on path:line". Nothing is written into a repository.
  **Review comments**: in the lens a tap on a diff row opens a comment box
  under it; comments gather in a draft (`usePref("review.draft:<machine>|
  <dir>")`, this device, per folder), sit under their rows (`.review-note`,
  matched by new-line number, or old-line number for a removed row) and a
  bar at the foot sends them as one numbered message (`reviewMessage`:
  `file:line`, the row quoted, the text) to the pane's agent, or through
  `SendToAgent` to any agent. Nothing goes until that button.
- `internal/code/` — the **Code mode**: each machine runs its own
  code-server (VS Code in the browser) on `127.0.0.1:7320`, `auth: none`,
  installed by `deploy/install-code.sh` (`make code [HOST=]`: a pinned
  release, checksum verified, launchd / systemd user service). herdr-ui is
  the only door: `/code/<machine>/…` is a `httputil.ReverseProxy` behind the
  token guard — prefix stripped, Host kept, `X-Frame-Options` dropped,
  absolute `Location`s re-prefixed, WebSocket upgrades passed; the target
  is `Machine.CodeAddr(port)`: the port itself locally, a loopback port
  forwarded over the ssh master (`sshx.Conn.ForwardTCP`) remotely, remembered
  per master. `GET /api/code/{machine}` probes `/healthz` (a good answer is
  cached 15s; `?check=1` re-probes). `web/src/Code.tsx` is the picker
  (machine, then a folder from the roster's workspaces) and the iframe,
  kept mounted behind the other modes once opened. The flag `-code-port`
  changes the port on every machine at once.
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
  chat, or `POST /api/seen`) — and a seen finish stays seen until the agent works again, because herdr itself keeps reporting `done` for hours, until the pane is focused in herdr; **a question is held until the agent works
  again** (unless it never had worked — then the dialog simply closed);
  `last_active` is the last moment herdr said working, recovered for panes
  older than herdr-ui from the session file's mtime (`ResolveSession` +
  `TranscriptVersion`, off the poller), persisted in `<state>/activity.json`
  together with whether the tick is still up (`held`), so a restart keeps
  the ticks as they stood and raises none for finishes herdr is still
  reporting from before; a pane first seen already finished (there before
  herdr-ui ran) counts as looked at.
  The idle tiers (fresh ≤15 min, stale ≥120 min, dimmed) are the UI's, from
  `last_active` and the clock. `web/src/radar.ts` is the pure model (vendor
  hues, tiers, marks, row labels, ordering: families by their liveliest
  agent, parent before worktrees, tabs by their liveliest pane, no-stamp
  last; a flat `recent` order; herdr's own), `web/src/Sidebar.tsx` draws it
  (one spinner clock for the braille frames and the pulsing question).
  Machines and workspace groups fold (prefs `collapsed` / `wsFolded`); a
  folded group's head carries its agents' states (working / ? / ✓ counts).
  `web/marks/*.svg` are the vendor logos (MIT packaging, see NOTICE.md),
  `npm run marks` regenerates `src/marks.ts`; drawn inline in currentColor.
- `internal/watchdog/` — notices agents that need a person: **stuck**
  (working ≥ 20 min with no transcript write for 20 min), **loop** (the
  last 6 tool calls of the current turn identical), **waiting** (blocked ≥
  15 min). A sweep a minute over `radar.Episodes()`; the inputs are
  functions wired in `main.go` (session file + mtime shared with the
  radar's recovery, the transcript's last turn for the loop check), so
  `watchdog_test.go` drives the rules without herdr. A note goes on the
  pane (`radar.SetAlert` → `pane.radar.alert`, an amber `!` in the
  sidebar, cleared when the state moves on) and one push per episode
  (`push.Notify`, pref `watchdog`, default on) with a reminder every 30
  min; the notification carries `actions` — Open, and **Send Esc**, which
  the service worker posts to `/api/keys` itself.
- `internal/schedule/` (cron as in Vixie: a restricted day-of-month and day-of-week fire on either) — prompts to agents on a timetable. A schedule names
  a machine and a **folder** (panes come and go; the project stays), a
  five-field cron in the server's local time (`Parse`/`Matches`/`Next`,
  tested), the prompt, and a switch. `Tick` every 30s fires what matches
  the minute and has not run this minute; delivery is
  `Server.SubmitToFolder` → the agent pane in that folder → `submitTo`,
  the chat's own path (an agent busy with a question refuses: noted, never
  queued). A failed run is pushed (`Kind: "schedule"`). `schedules.json`
  in the state dir; routes `GET/PUT /api/schedules`, `DELETE
  /api/schedules/{id}`, `POST /api/schedules/{id}/run`. `Schedules.tsx` is
  the panel behind the calendar button in the sidebar head.
- `internal/search/` — **full-text search** over every transcript on every
  machine (`GET /api/search?q=&machine=&dir=&days=`; ⌘K / Ctrl+K, the 🔍
  in the header). No index: `Scan` walks `~/.claude/projects` newest-first,
  sieves lines by four spellings of the query (as typed, lower, UPPER,
  Capitalized — a byte search folds nothing), then `transcript.MatchLine`
  parses the candidates: only user/assistant texts count, never tool
  output, thinking or sidechains. Locally Scan runs in-process; on a
  remote the script runs `~/.local/bin/herdr-ui search …` (the same Scan,
  a hidden subcommand — `make deploy` puts the binary there; 400 MB scan
  in well under a second) and falls back to a four-pattern grep when the
  binary is missing (BSD grep: seconds per hundred MB). Both print
  `./<project>/<session>.jsonl:<raw line>`, one parser reads either. A
  hit carries `project` (the project dir name = `EncodePath` of the
  folder the agent started in; a `cd` in the session moves the record's
  `cwd` but not the file) — the UI matches it to a pane by
  `encodePath(session.dir|pane.cwd)` and jumps (`jump` prop on Chat →
  `data-ids~=uuid` → scroll + `.turn-hit`), else opens the transcript on
  its own: `GET /api/chat?dir=&session=` without a pane is read-only
  (`Turn.IDs` carry the record uuids; `sessionIDRe` guards the id).
- `internal/usage/` — the **cost and context meter**. `prices.go` is Claude
  Code's own baked model catalog (read from the installed 2.1.285: nine
  price tiers, 21 models, 200k vs 1M windows) and its cost formula (the 1h
  cache tier, fast mode (`usage.speed`) from its own table, ×1.1 when
  `inference_geo` is "us" — not on web search); `transcript.UsageOf` reads
  an assistant record's `message.usage` (`<synthetic>` records and
  `isApiErrorMessage` ones are skipped: no API call behind them). One API response is written as
  several records sharing `message.id` with the same usage — `Accumulator`
  counts an id once (the records sit together; the last wins), holding the
  pending one until the next id. **Live meter**: `Tracker` keeps a sum per
  transcript file, reading only its growth (`Machine.ReadFrom`: a seek
  locally, `tail -c +N` over ssh), at most every 20 s per file, whole lines
  only; `watcher.Usage` hands it every snapshot (main.go: every agent pane's
  session file, and the file it continues after a compaction, read once and
  summed in — the whole `ContinuationChain`, so a second compaction loses
  no file; the hook runs off the poll's path with one in-flight read per
  machine, and a first read over 64 MB takes the tail only, `partial`);
  the roster pane carries `usage{model,name,context,window,cost,turns,
  tokens}` — context = the last response's input + cache tokens.
  The live session's `<session>/subagents/*.jsonl` (listed in one round
  trip, `Machine.DirVersions`) and sidechain records count toward the pane
  too — the number should agree with Claude Code's own. **Report**: `Scan` (`GET /api/usage?days=&machine=
  &tz=`, since = a local midnight) walks `~/.claude/projects` for files
  written since, subagent files count toward their session, per day in
  the viewer's zone, per project/model/session; remotely the deployed
  binary runs `herdr-ui usage -since -tz` (gated on the version, like
  search). UI: `Usage.tsx` (📊 in the sidebar head, or the header pill),
  the pill in the header (fill bar · cost) and on radar rows from 50% up
  (amber 70%, red 90%), Chat's compaction notice from 80% with "Compact
  now" (`/compact` through submit). Prices are API-equivalent: on a
  subscription a measure, not a bill.
- `internal/quick/` — saved prompts (`quick.json`, `GET/PUT /api/quick`,
  the list replaced whole), each for every folder or one `dir`; the chat
  composer offers them under ⚡ and edits them in a dialog. The composer
  also dictates through the browser's own `SpeechRecognition` (nothing
  passes through herdr-ui) and every assistant turn has **forward** — the
  turn's text, headed "From <workspace on machine>:", to another agent
  through `SendToAgent.tsx`, the one dialog Tasks and Chat share
  (`send.targets` pref remembers the last target per key).
- `internal/tasks/` — the **Tasks mode**: herdr-ui's own list in
  `tasks.db` (SQLite, WAL, `modernc.org/sqlite` — pure Go, so the four
  static builds keep `CGO_ENABLED=0`) in the state dir of **the herdr-ui
  that serves the page** (each instance has its own list). Projects (a
  tree: `parent_project_id`, archived, favorite, position), tasks (HTML
  description, done/done_at, due_date, priority 0–5, labels), labels.
  Dates are RFC 3339 UTC strings, "" when unset. `GET /api/tasks?view=
  today|upcoming|open|project&project=&done=1&q=&page=&tz=` — the smart
  views cut by the viewer's local day (`tz`), the search case-folds in Go
  (SQLite's `lower()` is ASCII only), 50 a page; `POST /api/tasks`,
  `GET/PATCH/DELETE /api/tasks/{id}` (PATCH is partial: nil keeps, `""`
  clears a date), `PUT /api/tasks/{id}/labels {label_id}`, `DELETE
  …/labels/{label}`, `/api/tasks/projects[/{id}]`, `/api/tasks/labels`.
  An empty database gets an "Inbox". `web/src/Tasks.tsx` + `tasksApi.ts`;
  descriptions are rendered through an allow-list into React nodes (never
  innerHTML) and edited as plain text that goes back as paragraphs. The
  top-bar `mode-switch` (Agents | Tasks | Code) is a per-device pref.
  **Send to agent** (a split button in the task; "for grooming" prefixes
  `/grill-me`) composes `taskPrompt` (title, description as text, the task
  id and project) and submits it through the existing `/api/submit` to an
  agent pane from the roster, remembering the last target per project.
  **Start an agent on it** (`POST /api/tasks/{id}/start`): a worktree off
  a workspace's repo (`WorktreeCreate`, branch `task-<id>-<slug>`), the
  agent in it (`startAgent` retries herdr's `agent_pane_busy` while the
  new pane's shell comes up — the generic create path uses it too), a
  `task_runs` row and the label **in progress**; then `deliverTaskPrompt`
  in a goroutine: `AgentPrompt` every 2 s until the agent takes it. Claude
  Code's "trust this folder" question for that very worktree is answered
  yes (`PaneRead` → `isTrustPrompt` → keys down+enter, noted as a comment);
  any other question of the agent's gets one push and waits; five minutes
  → `failed`. `tasks.RunWatch` (wired in main, every 15 s, injected
  radar/transcript/push) turns every finish — the pane not working and the
  agent's latest turn ending in text, a record not reported yet (a fresh
  session has no turn before the prompt) — into a comment by "agent" plus
  a push; a pane gone two
  minutes → `gone`. Done takes the label off. `comments` are markdown;
  the UI re-reads an open task every 10 s while a run is live.
- `internal/auth/` — single bearer token; HttpOnly cookie; no accounts.
- `internal/server/` — HTTP routes + the pane WebSocket. `/api/health` and
  `/api/login` are mounted outside the token guard; everything else behind
  it. Every route resolves its machine through `machineFor` (query →
  `X-Herdr-UI-Machine` header → POST body `machine`) and `ready()`. The three
  create routes share `create()` (agent kind validated *before* any RPC,
  container made, agent started, watcher refreshed); the two close routes
  share `closeHandler`, the two renames (`PATCH /api/tabs/{id}`,
  `PATCH /api/sessions/{id}`, body `{label}`) share `renameHandler` —
  herdr's `tab.rename` / `workspace.rename`. In the UI a tab is renamed in
  place by double-click, or from the workspace's ⋯ menu (touch). The roster carries an ETag (304 when unchanged) and
  the agent list. Local files stream through `http.ServeContent` (Range for
  video); the WS uses per-message compression and coalesces queued chunks.
- `web/src/styles.css` — one design system, dark and warm: four surface
  layers (`--bg` → `--bg-raised` → `--bg-input` → `--bg-overlay`, `--bg-active`
  / `--bg-elev` for a raised chip), hairlines as white at low alpha (`--line`,
  `--line-strong`, `--line-hover`), every colour with a `-soft` fill and a
  `-ring`, a radius scale (`--r-xs` 4 … `--r-xl` 16 — never a px radius),
  a type scale (11 / 12 / 13 / 13.5 body / 15 / 18 / 20 — never another
  size), control heights `--ctl-lg`/`--ctl`/`--ctl-sm`/`--ctl-xs`, a layer
  ladder (`--z-drawer` < `--z-dialog` < `--z-menu` < `--z-select`), three
  shadows, `--ease`/`--fast`/`--base` for motion (and
  `prefers-reduced-motion`). **Every `<button>` carries one family class**:
  `btn` (quiet), `btn primary` (the one filled button of a surface), `btn
  danger`, `btn ghost`, `icon-btn` (+ `sm`/`lg`), or `linkish`; `.on` marks a
  toggled one. Fields have one base rule (every input, textarea and `.sel`;
  `.sm`, `.bare`), one placeholder, one focus ring. Rows share a radius,
  hover and focus ring; cards are `.card`; counts are `.badge` (`.quiet`);
  state dots are `.dot.dot-<status>` / `.dot.machine-state.is-<state>`. On
  a coarse pointer the small controls grow and hover-revealed ones stay
  visible. A button is styled by its family class, not by where it sits;
  the only context rules left are the components whose children *are* the
  component — segmented controls (`.mode-switch`, `.lens-switch`,
  `.fv-tabs`), menus (`.row-menu`), the key bar (`.keybar .key`). Reuse
  these before writing new rules.
- `web/src/ui.tsx` — the shared pieces: `Dialog` (one scrim over the
  drawer, a head with icon/title/controls/X, Escape, focus in and back out,
  `#root` inert while open, `as="form"` + `onSubmit`, sizes sm/md/lg),
  `Notice` (tones error/success/warn/info/hint, optional icon, action and
  dismiss), `useTimedNotice`, `useDismiss` (popovers), `useSubmit`,
  `useAsync`, `StatusDot`. `time.ts` (`ago`, `whenShort`, `fmtDate`,
  `fmtDay`) and `options.tsx` (`machineOptions`, `folderOptions`,
  `machineLabel`) are the one way to say a time or build a roster select.
  Icons are `web/src/icons.tsx` (inline 24px stroke paths, `<Icon name
  size={12|14|16}>`): never an emoji or a font glyph as an icon.
  `Select.tsx` replaces every native `<select>` (an icon per option, a
  portal so nothing clips it).
- `web/` — React + Vite + xterm.js SPA, embedded via `web/embed.go`
  (`go:embed all:dist`). Build with `make web` before `go build`. **Paste**: xterm brackets a paste only once it has seen the app's
  DECSET 2004 — and a viewer that attaches later gets no replay, so it
  never sees Claude Code's "bracketed paste on" from startup and would
  paste a multi-line text raw (every newline an Enter: the prompt goes
  off line by line). `Terminal.tsx` tracks whether 2004 ever passed and,
  when nobody knows, wraps a text with line breaks in `ESC[200~ … ESC[201~`
  itself (CRLF/LF → CR, as xterm does); a single line stays raw. herdr's
  attach forwards the markers only to a pane that enabled the mode, so a
  plain `cat` sees none.

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
  a repaint (see attach.go), never a byte replay.
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
