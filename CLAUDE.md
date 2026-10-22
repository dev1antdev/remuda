# remuda — notes for agents

Self-hosted web/phone cockpit for the AI coding agents on the user's
machines, on top of **herdr** (0.9+), which it installs, starts and
updates. Go server + embedded React SPA. See README.md for the user
story. Renamed from herdr-ui on 2026-10-11 (before that: drover): the
state dir, the service and the browser's localStorage keys carry over
by themselves (`state.Dir`, install-service.sh, prefs.ts); the binary,
module path, headers (`X-Remuda-*`), env vars (`REMUDA_*`), the
`.remuda/uploads` folder and the Tailscale hostname are the new name.

How to build, check and work here — the rules every agent follows and the
tooling in `.claude/` (hooks, `/verify`, `/live-check`, `/herdr`): @AGENTS.md
— the same file Codex, Cursor and Copilot read, and Gemini through the
`GEMINI.md` symlink.

## Layout

- `cmd/remuda/` — main; flags `-addr` (default `127.0.0.1:7317`), `-socket`,
  `-herdr-bin`, `-print-token`, `-version`, `-tailscale` /
  `-tailscale-hostname`, `-no-version-check`, `-code-port`; subcommands
  `search` and `usage` for the deployed binary on a remote machine, and
  `setup`, `uninstall` and `code` (see "Installing and uninstalling"
  below); `remuda -h` lists them (`help` in main.go). `main.go` parses the
  flags and hands `app.Options` to `internal/app`; the subcommands are
  `commands.go`.
- `internal/app/` — the composition root: `Run(ctx, Options)` builds every
  service, lists who hears which event (the one place the subscriptions
  live), starts the loops and serves. Nothing here does work of its own.
- `internal/script/` — every POSIX script remuda runs is built with it:
  `Quote`, the `PATH` line (`~/.local/bin`, Homebrew, Kimi Code's own
  `~/.kimi-code/bin`), `InDir` / `InRepo` (cd, or answer nothing),
  the portable `Stat`, `Remuda(home, version)` (a machine's own remuda
  and the guard that it is this version), and the separators `GS` / `RS`
  / `US` with `Sections` / `Lines` / `Fields` for reading an answer.
- `internal/shell/` — the port scripts go through: `Shell` (`Run`,
  `Stream`), `Host` (+ `ID`, `Label`, `Home`, `Local`), `ExitError`,
  `Text`. `shell.Local` runs here through `sh -c`, `sshx.Conn` is the
  adapter over the master, `shell.Fake` answers scripts in tests. review,
  jobs, search and usage take a `shell.Host` and know nothing of ssh or
  herdr; a `*machines.Machine` is one.
- `internal/events/` — the in-process bus: `PaneRef`, `PaneStatus` (To is
  `Gone` for a closed pane), `Snapshot`, `MachineRemoved`, `JobDone`,
  `LocalHerdrDown`. `Subscribe` / `Publish` are typed, synchronous and in
  subscription order; the watcher and the jobs publish, push, the
  restarts, the radar, the review badges, the meter and the vitals
  subscribe — in `internal/app`.
- `internal/state/` — the one state dir (`<user config dir>/remuda`, 0700,
  or `REMUDA_STATE_DIR` for a second instance beside the real one)
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
  the `ssh -tt … herdr terminal attach` command builder. remuda implements
  no SSH itself.
- `internal/machines/` — the multi-PC registry: `manager.go` (the
  registry), `machine.go` (one machine: its state, the ssh master, the
  herdr client and attach manager, the peer, port forwards), `herdr.go`
  (herdr's binary and server on the machine), `files.go` (the file
  viewer's reads and the one write), `shell.go` (a `Machine` is a
  `shell.Host`: `Run` / `Stream` here through `sh -c` or over the master);
  `Machine.Transcripts` is a `transcript.Reader`. `local` is always present.
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
  forwarding, else one `ssh host nc -U <sock>` exec per request
  (one sshd on :2222 answers "unknown channel type"). Remote
  transcripts are fetched by `stat` + `tail -c` delta into a small
  per-session byte cache (`transcript.Reader.Turns` hides local vs remote);
  `Reader.ResolveSession` finds `<uuid>.jsonl` and caches the hit on both sides.
  `Machine.Ready()` reconnects a dead remote at most every 15s; a removed
  machine never reconnects and its watcher loop exits.
- `internal/transcript/` — read-only parser for Claude Code JSONL session
  files. Never write to `~/.claude`. Session resolution: herdr's
  `agent_session` (exact UUID) first, cwd-encoded dir scan as fallback.
  **Compaction moves the conversation to a new file** (its first message's
  `parentUuid` is a record `uuid` in the old one) while herdr keeps naming
  the old id, so the chat follows `Reader.Live(path)` —
  `transcript.Continuation` locally, `ContinuationScript` over ssh —
  re-checked a minute at a time per file. `transcript.Reader` (reader.go)
  is the session files of one machine: `Version`, `DirVersions`,
  `ReadFrom`, `Turns`, `ResolveSession`, `Live` / `Chain` / `Lineage`
  (every file a conversation lived in) and `Predecessor`, in process here
  and by script over a `shell.Host` on a remote. `Turn.Answer`,
  `Turn.ToolCalls` and `Turn.LastID` say what a turn ended with. The chat lens (`features/chat/Chat.tsx`: the transcript through `useChat.ts`, the one fetch hook — every 2 s while the agent works or asks, 10 s otherwise, a 304 = unchanged, `last=N` for a wall tile; `Composer.tsx` (`compact` in a tile), `TurnView.tsx`, `PromptKeys.tsx` (↑ ↓ Enter Esc for a question), `Tail.tsx` (a tile's chat: the last 12 turns, 3 s / 15 s), `FilesRail.tsx` (the **files rail** beside the lenses: every file the agent edited or read in this conversation since its last compaction, from its own tool calls — `footprint.ts` is the pure part, tested: `toolPath` reads `file_path` / `notebook_path` from an object input or from one the server cut to a string at 8 KB, so `TurnView` links a big Write too; edited files first by their last touch, the ones only read folded; its own `useChat` poll at a tile's pace, only while on screen; a row opens the file viewer on its changes, the code button `actions.openInCode`; a change made through a shell command is the Changes lens's to show, not the rail's), `QuickPrompts.tsx`, `dictation.ts`) follows the bottom (`useFollowBottom.ts`)
  only when the reader is there and not selecting or dragging; an upward
  wheel or finger lets go at once, and `TurnView` (`sameTurn`) re-renders only when a
  turn's content changed, so a selection survives the polls.
- `internal/machines/files.go` — the file viewer's reads, local or over
  SSH, plus the one write: `SaveUpload`. `ResolvePath` accepts absolute, `~/`, or pane-relative
  and, on a relative miss, searches the pane folder by suffix (agents say
  `demo.mp4` for `docs/demo.mp4`); several hits come back as `candidates`.
  `GitDiff` = `git diff HEAD -- file` in the file's repo (`untracked` for new
  files, no `repo` outside git). `ListDirs(path, hidden)` lists a folder's
  subfolders (repositories marked by a `.git`; `""`/`~` is home) for the
  folder picker behind the New workspace dialog's browse button
  (`web/src/features/roster/FolderPicker.tsx`: crumbs, subfolders, the machine's known
  workspaces as quick picks, hidden on request). Routes: `/api/fs/dirs`,
  `/api/fs/stat` (resolve + kind; `dir=` stands in for the pane's folder
  when there is no pane — a transcript on its own, a task's comments),
  `/api/fs/file` (bytes; text is always `text/plain`, CSP-sandboxed, 512KB
  text / 24MB raw caps), `/api/fs/diff`, `POST /api/fs/upload` (multipart
  `file` ×≤4, 25MB each → `<pane dir>/.remuda/uploads/<time>-<tag>-<name>`,
  stem and extension sanitized separately, remote via `sshx.Conn.Put`).
  `.remuda/.gitignore` (`*`) is written alongside so uploads never reach
  a commit. Uploads are the only thing remuda writes into a project.
- `web/src/features/chat/attachments.ts` — `useAttachments` (upload at
  once, chips with thumbnails); `lib/transfer.ts` has `filesFrom`
  (drop/paste → files; a pasted screenshot gets a name), `lib/text.ts`
  `shellQuote`. The chat composer (`chat/Composer.tsx`) inserts `@<rel>`
  at the caret and strips it when the chip is removed; the terminal lens
  (`terminal/engine/paste.ts`) types the absolute paths shell-quoted.
  Nothing is sent until the user sends.
- Terminal size modes (`web/src/features/terminal/engine/geometry.ts`): **fit** resizes the shared
  pane to this screen (herdr follows the smallest attached client — a phone
  would squeeze the desktop's view), **keep** attaches at the pane's own
  grid (`cols`/`rows` from the snapshot layouts, in the roster) and shrinks
  the font to fit the width, scrolling when it can't. Touch defaults to
  keep. Agent status lines (ccstatusline etc.) only fit in keep mode on a
  phone.
- Terminal mouse and selection (`features/terminal/engine/`: `modes.ts`
  intercepts, `mouse.ts` encodes the reports, `pointer.ts` listens,
  `selection.ts` keeps the selection, `layer.ts` is the ⬚ layer, `copy.ts`
  reads whichever selection there is): Claude Code's full-screen
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
  (`Toasts` in `ui/Toasts.tsx`, fed by `app/toasts.tsx`), never a pill in a bar. Lenses stay mounted once the
  selected pane has shown them (`.lens-pane[hidden]`, scoped to that pane
  — a lens never mounts hidden, since a terminal opened unseen would
  attach at 80×24 and resize the shared pane): the terminal keeps its
  attach, the chat its scroll and no refetch, Changes its draft; a lens
  gets `active` and polls or refits only when on screen.
- `internal/health` — each machine's vitals (Claude Code / herdr /
  remuda versions, how Claude Code was installed, herdr's agent hooks
  that lag the installed herdr (`OutdatedHooks`, the shell behind
  `Report.OutdatedHooks`) and its plugins from GitHub (`GitHubPlugins` →
  `Report.Plugins`), disk, load, uptime)
  from one POSIX script per machine through `shell.Text`, every 15
  minutes and on demand (`POST /api/machines/{id}/health`); the newest
  Claude Code, Codex, Gemini CLI and Kimi Code (npm registry), herdr and **remuda
  itself** (GitHub releases) every 6 hours unless `-no-version-check` —
  the only outbound requests remuda makes by itself (a read on demand
  asks again when the last look is over 5 minutes old, `RefreshLatest`).
  `release.go`: the newest release with its assets (`Collector.Release`).
  **`internal/updates`** turns all of that into what the page offers:
  `Plan(what, report)` / `Offered(report, latest)` are the commands for
  herdr and the agents, `ReleaseUpdate(have, os, arch, rel, local)` offers
  `what = remuda-release` ("Download & install remuda vX") when the
  release's x.y.z is newer than the machine's (a build past a tag,
  `v0.4.0-24-g…`, is 0.4.0) and it has `remuda-<os>-<arch>` for the
  machine; `ReleaseScript` is what the machine runs on Run: curl the asset
  to `~/.local/bin/remuda.new`, check it against the release's
  `checksums.txt` when there is one (a mismatch keeps the old binary;
  none → installed with a "not verified" line), `InstallScript` (chmod,
  `-version`, mv) and `RestartScript` (launchctl kickstart / systemctl
  restart) — the machine serving the page restarts detached two seconds
  after the job instead, so the job ends first and the stale-page pill
  does the rest. `updates.Service` (`Available`, `Start`, `Recent`, `Job`)
  is what the four update routes call: `Start` refuses a command that is
  not the one shown with a `StaleError` (409), deploys this very binary
  for `what = remuda`, and wraps an agent update in the restart hooks. `make release` writes `bin/checksums.txt`; `make
  publish` (on `main` with `release` fast-forwarded in, a tag on HEAD, a
  clean tree, the checks green) pushes the tag, and
  `.github/workflows/release.yml` builds the binaries and makes the
  GitHub release from it; `.github/workflows/ci.yml` is /verify's list
  plus govulncheck, npm audit and gitleaks on every push to main or
  release and every pull request. Pull requests target `release`; `main`
  is what is out (AGENTS.md, "Build and check").
  The roster carries `health` per machine and `latest`; the sidebar's ⓘ
  (`features/roster/HealthButton.tsx`; the rules in `concerns.ts`) turns amber on an update, a lagging
  remote remuda, or a nearly full disk.
- **Peers** (`internal/server/peer.go`): herdr allows one attached client
  per terminal ("already has an attached client; retry with --takeover"),
  so two remuda reaching the same pane — the machine's own and this one
  over SSH — collide. The health script also probes `127.0.0.1:7317/api/
  health` on the machine and reads its token file; a running remuda
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
- **Installing and uninstalling need no checkout** (`internal/setup`: five
  embedded POSIX scripts, and `setup_test.go` drives them in a temp HOME
  with stand-ins for launchctl, systemctl, herdr, brew and ssh first on
  PATH — never against the real machine). `install.sh` is the one line
  people pipe to sh, a release asset (`releases/latest/download/
  install.sh`, uploaded by release.yml): the binary for the OS and CPU,
  checked against `checksums.txt` (no match, no install), put at
  `~/.local/bin/remuda`, then `remuda setup -open`; its work sits in
  `main`, called on the last line, so a download cut off half way runs
  nothing. `REMUDA_INSTALL_VERSION` / `_ONLY` / `_MISSING` / `_BASE` steer
  it, and `setup -host H` runs it on H with ONLY and MISSING (and this
  build's tag when it is a release, `installEnv`), so a remote without
  remuda fetches the binary for its own OS and CPU. `-open` waits for
  `/api/health` and opens the page on a Mac at the desk, the address
  handed to `osascript` on stdin (`openPage` in local.go: the token never
  goes on a command line; on Linux the link is only printed). `remuda
  uninstall [-host H] [-data] [-code] [-herdr] [-all] [-yes] [-n]` is
  `uninstall.sh` behind `setup.Removal`: `plan` prints what would go and
  what stays (exit 10 when nothing would), the person is asked
  (`confirm`: no terminal means no), `run` removes — the service, the
  binary, the log and the ssh masters always; the state dir, code-server
  and herdr (its hooks first, then its server stopped, the binary through
  brew or from under HOME, its folders) only by their flags. A folder is
  deleted as the state dir only when remuda's files are in it.
  `Removal.Remove` starts the local script in a session of its own
  (`Setsid`): typed into a herdr pane, `-herdr` stops the server that
  pane lives in, and herdr ends the pane's process group with it (seen in
  a container: the script died right after the stop). A remuda the
  service does not run (`by_hand`: pgrep minus this command and the
  service's pid) is only pointed out — and stopped when herdr goes,
  because its `EnsureServer` would start the server again. `remuda
  code [-host H] [-port N] [install|uninstall|status]` is
  `install-code.sh` (`setup.Code`).
- **herdr is remuda's to run.** `herdr.EnsureServer(bin, socket)` pings
  and, when nothing answers, starts `herdr server` (the headless server)
  detached, at most once per two minutes — at startup and whenever the
  watcher's local poll fails (`events.LocalHerdrDown` → `herdr.Keeper.Revive`);
  a remote machine's `connect()` does the same over ssh
  (`herdr.StartServerScript`) before giving up. `remuda setup [-host H] [-tailscale]` (`internal/setup`:
  `setup.sh` installs herdr with brew or its installer, starts the
  server, installs `herdr integration` hooks for the agents present and
  installs again every integration herdr reports outdated;
  `install-service.sh` is the launchd / systemd service) is the
  one-command install, locally or over ssh. `internal/jobs` runs a shell script on a `shell.Host` in the background
  with its output streamed (`Host.Stream`: `sh -c` here, the master for a
  remote) for the page to poll (`GET /api/jobs/{id}`) and publishes
  `events.JobDone`, on which the vitals are re-read; `updates.Plan(what,
  report)` is the update command for herdr (brew or `herdr update --handoff`;
  every brew path runs `brew update` first, once per job),
  claude / codex / gemini / kimi (brew, npm or the agent's own updater, by
  where the binary lives), or every agent (`restart.Kinds("agents")` is the
  one list of them); `what=hooks` is `herdr integration
  install` for every agent whose hooks herdr reports outdated (the herdr
  update runs that step itself, `hooksStep`, before the server restart —
  0.9.5 changed what Codex's hooks are), `what=plugins` is `herdr plugin
  update --yes` for the plugins installed from GitHub, on herdr 0.9.4 and
  up; `GET /api/machines/{id}/updates` lists
  them (`updates.Service.Available`), `POST …/update {what}` starts one
  (`Service.Start`); `what=remuda` copies this very binary to a remote of
  the same OS/CPU and restarts its service; `what=remuda-release` has the
  machine (the local one too) download the newest GitHub release itself
  (`updates.ReleaseUpdate`). The UI: the
  concern "remuda vX is out" in the ⓘ with a Download & install button,
  the row in Updates, and one toast per version in App.tsx (pref
  `release.seen`) whose button opens the local machine's Updates on it.
  `features/updates/Updates.tsx` shows the list, the command before "Run", the live log,
  and the machine's recent jobs. Nothing uses sudo; everything runs as
  the user through the shell remuda already has on the machine.
- **Chats follow an agent update** (`internal/restart`). When an update
  job changed an agent's version (the vitals re-read right after), every
  pane running that agent is restarted onto it: `agent.prompt` with the
  agent's own `/exit` (Ctrl-C twice if it lingers), a wait for the shell
  prompt, then `agent.start` with `--resume <session id>` (`codex resume`,
  `gemini --resume`, `kimi --session`) from the snapshot's `agent_session`. Idle/done chats
  go at once; working/blocked/unknown ones are booked and restarted after
  the watcher sees them settle (`events.PaneStatus` → `Restarter.OnStatus`,
  a 25 s timer, then its own job via `Restarter.Later`). A pane without a session id
  is left alone; a pane that closes with its agent is reported. `what =
  restart-<kind>` runs the restart alone. A restart holds the pane's
  lock (`sessions.Input.Lock`) from the exit to the resumed agent (nothing else lands in
  the bare shell in between), a booking is tied to the session id it was
  made for, and a settled chat waits 25 s more (any change in between
  calls it off) because the person is likely reading or typing right
  then. This is the one place remuda types text the person did not
  compose, and only because they pressed Run on an update or a restart.
  `Restarter.Server` (restart/server.go) is the server onto a new binary
  when no handoff happened: the chats with sessions noted and their panes
  locked, `herdr server stop`, `Machine.StartServer`, a wait for the
  restored session, then every pane that came back as a shell gets
  `agent.start --resume`. The herdr update job ends with `herdrThen`
  (`Machine.ServerVersions`: binary vs running server; equal → nothing,
  else the restart); `what = restart-herdr` runs it alone and the vitals
  probe's `herdr_server` line is what offers it.
- `internal/tsnetx` — `-tailscale` joins the tailnet as a tsnet node
  (userspace, no tailscaled) named `-tailscale-hostname`, serves the same
  handler over HTTPS on :443 with the tailnet's certificate (:80
  redirects), state under `<state dir>/tailscale/`. The status
  (needs_login with the URL, running with the URL, error) rides on the
  roster as `tailscale`; the App shows a login toast. `TS_AUTHKEY` skips
  the login. The loopback listener stays.
- The build's version is injected into the bundle (`__REMUDA_VERSION__`,
  from `REMUDA_VERSION` set by `make web`) and the roster answer carries
  the server's in `X-Remuda-Version`; a page older than its server shows
  a "new version — reload" pill. After a deploy, a stale tab is told.
- `web/src/ui/Markdown.tsx` — dependency-free Markdown → React nodes (no
  HTML strings, nothing to sanitize). `parseBlocks` is pure (task-list
  state is decided there, not in render) and memoized per text;
  `markdown.test.ts` covers it.
- `web/src/lib/` — what every screen uses and no screen owns, React-free
  but for the hooks: `http.ts` (`check` / `getJSON` / `sendJSON`, every
  request), `query.ts` (`useQuery(fn, deps, {every, enabled, keep})` —
  the one way a screen reads the server: a fetch that follows its inputs,
  polls while enabled and the tab is visible, drops an answer that
  arrives after the inputs changed; `undefined` from the fetcher means
  unchanged (an ETag 304) and the data stays; `reload`, `fetching`, and
  `mutate` for what the screen already shows ahead of the server — a
  switch flipped, a card dropped), `store.ts` (`createStore` over
  `useSyncExternalStore`: `get` / `set` / `subscribe` / `use(select)`),
  `prefs.ts` (`usePref`, localStorage-backed and shared: every hook of a
  key follows a write through any other or through `writePref`, over
  `useSyncExternalStore` — the terminal's tools in the tab strip and the
  lens beside them hold the same keys; `readPref` keeps a stored
  value only in the fallback's shape; `isTouch`), `text.ts` (`errMsg`,
  `basename` / `dirname`, `plural`, `shellQuote`), `time.ts`, `paths.ts`,
  `host.ts` (`copyText`: Clipboard API, else execCommand; `openUrl`: a
  new tab — nothing embeds remuda any more, the JetBrains plugin and its
  `window.herdrHost` bridge are gone), `transfer.ts` (`filesFrom`),
  `version.ts` (`newer`).
- `web/src/app/` — the shell. `store.ts` is the one page store: the
  *requested* pane (a deep link, a notification, a click), the lens, the
  mode, the drawer, the open dialog / menu / panel, the notice, the
  confirm, the file viewer, the search viewer, a jump; `actions` are the
  only writes. `current.ts` derives the shown pane from the requested
  one, falling back to the first agent pane — nothing fixes selection up
  in effects. `url.ts`: the address bar follows the store
  (`?machine=&pane=&mode=&lens=`, `history.replaceState`) and is read
  once at load, so a link or a reload lands on the same pane in the same
  lens. `act.ts` (`act(promise)`: refresh the roster, else toast the
  error), `toasts.tsx`, `Shell.tsx` (URL sync, ⌘K, the service worker's
  select-pane, mark-seen, the four modes), `Header.tsx`, `PaneArea.tsx`
  (the lenses, mounted once seen, and the files rail beside them in
  `.pane-body` — store field `footprint` from the pref of that name,
  default on with a mouse, its toggle in the tab strip's tools slot
  whatever the lens so nothing in the strip moves, disabled for a shell
  pane, hidden with the rail under 920 px), `Wall.tsx` (the **Wall mode**: every
  agent pane on a connected machine as a tile — the sidebar row's head
  (mark, logo, label, workspace · machine, context % · cost), the last 12
  turns through `features/chat/Tail`, a compact composer; a click on the
  head opens the pane in Agents on the chat lens. `features/roster/wall.ts`
  says which panes make a tile and in what order — the roster's, stable, so
  no tile jumps on a status change — and leaves out the ones hidden by hand
  (pref `wall.hidden`, the eye on a tile, "Show N hidden" resets) and, with
  "Hide idle", the idle ones past the radar's fresh tier; pref `wall` holds
  the size S/M/L (`TILE_MIN` 360/460/600 px, an auto-fill grid) and the idle
  switch; no sidebar in the mode, "+ Agent" opens the New workspace
  dialog; the tile itself is `Tile.tsx`, `TileView`), **the pop-out**
  (`pip.ts` owns the one small window Chrome's Document
  Picture-in-Picture gives — `popOut(selection)` from the click itself,
  the page's stylesheets copied in, `pagehide` clears the store; one
  window at a time, the browser's rule; `popOutSupported` decides whether
  the buttons exist at all, so nothing in a bar appears or vanishes.
  `PopOut.tsx` draws the popped pane through a `createPortal` into that
  window's body — the same `TileView` as the wall, the roster, the store
  and every handler shared, "That pane is gone" once the roster lost it.
  Store field `popout`, buttons on every wall tile and in the header,
  disabled there without a pane), `Overlays.tsx` (panels, dialogs, the
  menu, the confirm, the file viewer). The roster is read once for the
  page: `features/roster/useRoster.tsx` (`RosterProvider`: `useQuery`
  every 3 s with the ETag, `stale` when the server is newer; `useRoster()`
  hands out machines, agents, latest, tailscale, version, offline).
- `internal/review/` — the **Changes lens**: a workspace folder's
  uncommitted work, what `git diff HEAD` shows — files changed against
  HEAD, staged or not, plus untracked ones — with the ones that moved
  since the reviewer last looked marked fresh, and the commits made since
  that look. Never what a merge, a pull or a rebase brought into HEAD: a
  diff against the commit reviewed last once listed the five hundred
  files a merge of staging carried in. A baseline per `machine|folder`
  (`review.json`: HEAD then, blob hashes of the files dirty then, and the
  verdict the agent was told, if any) is set by `POST /api/review/seen
  {dir, verdict?}`; `GET /api/review?dir=` runs one shell (`shell.Text` on
  the machine: local `sh -c` or over ssh) for branch, HEAD, the commits
  `baseline..HEAD` along the first parent (a merge is one line, not the
  history it brought), `git diff --numstat HEAD`, untracked files, the
  current blob hashes and each listed commit's resulting blobs — a file
  is *fresh* when its blob moved since the baseline (or there is none); a
  commit is *seen* when every blob it brought is the one reviewed (a
  commit of reviewed work, an amend, a rebase). A look records the blob
  of every file the review showed — changed against HEAD or untracked —
  so that a commit of that very content brings nothing new and a file
  committed as reviewed leaves the list. `GET /api/review/diff?dir=&path=`
  is one file's diff from the blob the baseline recorded for it (only the
  edits since the look), else against HEAD (an untracked file whole).
  The lens shows fresh files first and folds the rest behind "N files
  unchanged since you looked"; seen commits are dimmed. Every
  `events.Snapshot` goes to `review.RefreshBadges`, which runs `Badges` a
  minute apart and never two at once per machine, one shell over
  `Snapshot.WorkspaceDirs()`, and the roster carries `session.dir` and
  `session.review {changed, fresh}` for the sidebar badge.
  `features/review/Review.tsx` draws it (`Diff.tsx` the file rows,
  `Hunk.tsx` a hunk's rows in one column or two with a comment box under
  the tapped row, `hunks.ts` the parsing — `splitHunks`, `rowsOf`,
  `pairRows` lays a removed run beside the added run that follows it,
  `partners`, `changedSpan` the words that differ between a removed line
  and its replacement, out to word boundaries, drawn as `.fv-mark`;
  `keys.ts` the keys — `useReviewKeys`, by key position so a Cyrillic
  layout works: j k walk the files, ↑ ↓ the lines of the open one, n p
  its hunks, c comments on the focused line, o opens the file there, s
  flips the two columns (pref `review.split`, on by default with a mouse,
  the toggle in the lens head); the legend in the head, hidden under
  1180 px and on touch; the hook listens only while the lens is on screen
  (`active`) and never over a field, a dialog or the file viewer;
  `VerdictDialog.tsx`, `draft.ts` the comments and the messages they
  become): commits, files (fresh first), hunks with a comment box whose
  text goes to the pane's agent through `/api/submit` as
  "Review feedback on path:line". Nothing is written into a repository.
  **Review comments**: in the lens a tap on a diff row opens a comment box
  under it; comments gather in a draft (`usePref("review.draft:<machine>|
  <dir>")`, this device, per folder), sit under their rows (`.review-note`,
  matched by new-line number, or old-line number for a removed row) and a
  bar at the foot sends them as one numbered message (`commentsMessage`:
  `file:line`, the row quoted, the text) to the pane's agent, or through
  `SendToAgent` to any agent. **Verdicts** end a review with a word to the
  agent: Approve and Reject (`VerdictDialog`) each take a note and send
  `verdictMessage` — the verdict, the note, the draft's comments, and
  what it means (approved: reviewed, carry on, a task can move on;
  rejected: work on it and say when it is ready for another look) —
  through `/api/submit` first, then `markReviewed(verdict)`, so an
  agent's refusal leaves the baseline alone; a rejection needs a comment
  or a note. "Mark reviewed" is the silent one: the baseline moves, the
  agent hears nothing. The subtitle says "approved" / "rejected" / "since
  you looked" from the baseline's verdict. Nothing goes until one of
  those buttons.
- `internal/code/` — the **Code mode**: each machine runs its own
  code-server (VS Code in the browser) on `127.0.0.1:7320`, `auth: none`,
  installed by `remuda code [-host H]` (`internal/setup/scripts/
  install-code.sh`, also `make code [HOST=]`: a pinned release, its
  checksum known for all four platforms, launchd / systemd user service). remuda is
  the only door: `/code/<machine>/…` is a `httputil.ReverseProxy` behind the
  token guard — prefix stripped, Host kept, `X-Frame-Options` dropped,
  absolute `Location`s re-prefixed, WebSocket upgrades passed; the target
  is `Machine.CodeAddr(port)`: the port itself locally, a loopback port
  forwarded over the ssh master (`sshx.Conn.ForwardTCP`) remotely, remembered
  per master. `GET /api/code/{machine}` probes `/healthz` (a good answer is
  cached 15s; `?check=1` re-probes). `web/src/features/code/Code.tsx` is the picker
  (machine, then a folder from the roster's workspaces) and the iframe,
  kept mounted behind the other modes once opened. `actions.openInCode(machine,
  folder, path)` (the files rail's code button; store field `codeFile`)
  switches the mode, moves the picker to that machine and folder and
  remounts the frame with `payload=[["openFile","vscode-remote://<page
  host>/path"]]`, which the workbench opens on load — the authority is
  the page's own host because the proxy keeps `Host`. The flag `-code-port`
  changes the port on every machine at once.
- `web/src/lib/paths.ts` — `findPaths` / `findUrls`, shared by the terminal link
  provider (`terminal/engine/links.ts`) and the chat linkifier; keeps a
  `:line` suffix, drops trailing punctuation, never a path inside a URL.
  A link needs a name a file would have — a dotfile (`.env`, `.gitignore`;
  the dot kept on `.mcp.json`) or a name with an extension, in any
  alphabet — so a folder (`internal/server`) or a slashed phrase
  (`Approve/Reject`) is none.
  **File links only in an agent's pane** (`agent` prop); URLs everywhere,
  but Cmd/Ctrl+click in a shell pane — its clicks belong to the app. URLs
  open through `openUrl` in `host.ts` (host bridge, else a new tab).
- **The mouse model** (`terminal/engine/pointer.ts`, the reports encoded in `mouse.ts`): herdr enables mouse tracking in
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
  It publishes `events.Snapshot` and one `events.PaneStatus` per pane
  status transition (push, the restarts and the radar hear them); the
  first snapshot of a machine only seeds state — a reconnect must not
  replay a day of alerts.
- `internal/push/` — Web Push (RFC 8291/8292 via webpush-go). VAPID keys at
  `<config>/remuda/vapid.json` (a damaged file is an error, never a silent
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
  re-subscribes. `web/src/features/alerts/push.ts` is single-flight and re-registers on every
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
  older than remuda from the session file's mtime (`chat.Service.LastWriteOf`:
  the reader's `ResolveSession` + `Version`, off the poller), persisted in `<state>/activity.json`
  together with whether the tick is still up (`held`), so a restart keeps
  the ticks as they stood and raises none for finishes herdr is still
  reporting from before; a pane first seen already finished (there before
  remuda ran) counts as looked at.
  The idle tiers (fresh ≤15 min, stale ≥120 min, dimmed) are the UI's, from
  `last_active` and the clock. `web/src/features/roster/radar.ts` is the pure model (vendor
  hues, tiers, marks, row labels, ordering: families by their liveliest
  agent, parent before worktrees, tabs by their liveliest pane, no-stamp
  last; a flat `recent` order; herdr's own; `radar.test.ts`),
  `features/roster/Sidebar.tsx` draws it (`rows.tsx` the rows,
  `HealthButton.tsx`, `MachineMenu.tsx`, `RadarSettings.tsx`; one spinner
  clock for the braille frames and the pulsing question).
  Machines and workspace groups fold (prefs `collapsed` / `wsFolded`); a
  folded group's head carries its agents' states (working / ? / ✓ counts).
  `web/marks/*.svg` are the vendor logos (MIT packaging, see NOTICE.md),
  `npm run marks` regenerates `src/ui/marks.ts`; drawn inline in currentColor.
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
  `sessions.Input.SubmitToFolder` → the agent pane in that folder →
  `Input.Submit`, the chat's own path (an agent busy with a question refuses: noted, never
  queued). A failed run is pushed (`Kind: "schedule"`). `schedules.json`
  in the state dir; routes `GET/PUT /api/schedules`, `DELETE
  /api/schedules/{id}`, `POST /api/schedules/{id}/run`. `features/schedules/Schedules.tsx` is
  the panel behind the calendar button in the sidebar head (`cron.ts` the
  presets and their parsing, `ScheduleEditor.tsx` the form).
- `internal/search/` — **full-text search** over every transcript on every
  machine (`GET /api/search?q=&machine=&dir=&days=`; ⌘K / Ctrl+K, the 🔍
  in the header). No index: `Scan` walks `~/.claude/projects` newest-first,
  sieves lines by four spellings of the query (as typed, lower, UPPER,
  Capitalized — a byte search folds nothing), then `transcript.MatchLine`
  parses the candidates: only user/assistant texts count, never tool
  output, thinking or sidechains. Locally Scan runs in-process; on a
  remote the script runs `~/.local/bin/remuda search …` (the same Scan,
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
  transcript file, reading only its growth (`transcript.Reader.ReadFrom`: a
  seek locally, `tail -c +N` over ssh), at most every 20 s per file, whole
  lines only; `Tracker.ObserveSnapshot` takes every `events.Snapshot`
  (every agent pane's session file, and the files it continues after a
  compaction, read once and summed in — the reader's whole `Lineage`, so a
  second compaction loses no file; it runs off the poll's path with one
  in-flight read per machine, and a first read over 64 MB takes the tail
  only, `partial`);
  the roster pane carries `usage{model,name,context,window,cost,turns,
  tokens}` — context = the last response's input + cache tokens.
  The live session's `<session>/subagents/*.jsonl` (listed in one round
  trip, `Reader.DirVersions`) and sidechain records count toward the pane
  too — the number should agree with Claude Code's own. **Report**: `Scan` (`GET /api/usage?days=&machine=
  &tz=`, since = a local midnight) walks `~/.claude/projects` for files
  written since, subagent files count toward their session, per day in
  the viewer's zone, per project/model/session; `usage.Collect` asks every
  connected host at once, and remotely the deployed binary runs `remuda
  usage -since -tz` (gated on the version, like search). UI: `features/usage/Usage.tsx` (📊 in the sidebar head, or the header pill),
  the pill in the header (fill bar · cost) and on radar rows from 50% up
  (amber 70%, red 90%), Chat's compaction notice from 80% with "Compact
  now" (`/compact` through submit). Prices are API-equivalent: on a
  subscription a measure, not a bill.
- `internal/quick/` — saved prompts (`quick.json`, `GET/PUT /api/quick`,
  the list replaced whole), each for every folder or one `dir`; the chat
  composer offers them under ⚡ and edits them in a dialog. The composer
  also dictates through the browser's own `SpeechRecognition` (nothing
  passes through remuda) and every assistant turn has **forward** — the
  turn's text, headed "From <workspace on machine>:", to another agent
  through `features/agents/SendToAgent.tsx`, the one dialog Tasks, Chat and the review share
  (`send.targets` pref remembers the last target per key).
- `internal/tasks/` — the **Tasks mode**: remuda's own list in
  `tasks.db` (SQLite, WAL, `modernc.org/sqlite` — pure Go, so the four
  static builds keep `CGO_ENABLED=0`) in the state dir of **the remuda
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
  An empty database gets an "Inbox". `web/src/features/tasks/` (`Tasks.tsx`
  the mode on `useQuery` — the catalog, the list, the open task — with
  `mutate` for the optimistic moves; `ProjectTree.tsx`, `TaskList.tsx`,
  `TaskDetail.tsx` with `Labels.tsx` and `Comments.tsx`,
  `StartAgentDialog.tsx`, `prompt.ts`, `html.tsx`, `pieces.tsx`, `api.ts`);
  descriptions are rendered through an allow-list into React nodes (never
  innerHTML) and edited as plain text that goes back as paragraphs. The
  top-bar `mode-switch` (Agents | Wall | Tasks | Code) is a per-device pref.
  **Workflow**: every project has its columns (`statuses`: title, colour,
  `kind` open|done, position, the flags `on_new` / `on_start` /
  `on_finish` on at most one column each) — New / In progress / Testing /
  Done by default (`DefaultStatuses`), a copy of the parent's for a
  sub-project (`CreateProject`); a store from before is seeded by
  `seedWorkflows` at every open (done → the done column, the old "in
  progress" label → on_start, else on_new; the label is read once and
  never written again). A task is always in one column (`status_id`) and
  `done`/`done_at` **follow the column's kind**, so the smart lists and
  the checkbox stand: `Patch{Done}` moves to the done column or to where
  new tasks land, `Patch{StatusID}` sets done by kind, a move to another
  project maps the column by name (else by state). `Patch{After}` places a
  task in its column (0 = top; `placeAfter` takes the midpoint between the
  neighbours and renumbers the column when two of them touch); a changed
  column without After goes to the bottom. `PUT /api/tasks/projects/{id}/
  statuses {statuses, moves}` replaces the list whole (`SetStatuses`: id
  0 creates, a missing id is removed and its tasks go to `moves[old]`,
  required while it has any; ≥1 open and ≥1 done; each flag at most
  once). `view=board` lists a project whole in column order, one page,
  done columns capped at `BoardDone` (50 latest). **Fields**
  (`fields.go`): per project, `kind` text|number|select|multi|date|check|
  link, `options`, `on_card`, `in_prompt`; values sit in `task_fields` as
  JSON and only meaningful ones (an empty text, false, nothing chosen is
  no row); a task carries `fields{"<field id>": value}`, `Patch.Fields` is
  partial with null clearing; `PUT …/{id}/fields` replaces whole (a kind
  change drops the values, shrunk options prune them — `fit`); a task
  moved to another project keeps the values whose field there has the
  same name and kind (`carryFields`). `Prompt(task, project, groom)` adds
  a `Title: value` line per `in_prompt` field with a value. UI:
  `Board.tsx` — a column per status, cards, HTML5 drag-and-drop on a
  mouse, a ⋯ "Move to…" menu on touch, swipe-snapped columns a screen
  wide on a phone, quick add per column; the order shown is the array's
  (a drop reorders it at once, `patchTask({status_id, after})` confirms,
  a refusal puts it back) and the board re-reads itself every 15 s while
  on screen — the List | Board switch is the `tasks.view` pref, ⚙ opens
  `ProjectSettings.tsx` (name, colour, columns with their marks, fields;
  saved whole, a removed column asked where its tasks go), `fields.tsx`
  holds `FieldChip` / `FieldInput` / `valueText`.
  **Send to agent** (a split button in the task; "for grooming" prefixes
  `/grill-me`) composes `taskPrompt` (title, description as text, the
  `in_prompt` fields with a value, the task id and project) and submits it
  through the existing `/api/submit` to an
  agent pane from the roster, remembering the last target per project.
  **Start an agent on it** (`POST /api/tasks/{id}/start`): a worktree off
  a workspace's repo (`WorktreeCreate`, branch `task-<id>-<slug>`), the
  agent in it (`sessions.StartAgent` retries herdr's `agent_pane_busy`
  while the new pane's shell comes up — the generic create path uses it
  too), a `task_runs` row and the task moved to its project's `on_start`
  column (`StartRun`) — all of it `tasks.Launcher.Start` (start.go); then
  the delivery in a goroutine: `AgentPrompt` every 2 s until the agent takes it. Claude
  Code's "trust this folder" question for that very worktree is answered
  yes (`PaneRead` → `isTrustPrompt` → keys down+enter, noted as a comment);
  any other question of the agent's gets one push and waits; five minutes
  → `failed`. `tasks.RunWatch` (wired in `internal/app`, every 15 s, its
  inputs the radar's display and `chat.Service.LastAnswer`) turns every finish — the pane not working and the
  agent's latest turn ending in text, a record not reported yet (a fresh
  session has no turn before the prompt) — into a comment by "agent" plus
  a push, and `Store.OnFinish` moves the task to the `on_finish` column
  when it still sits in `on_start` (a card the person moved meanwhile
  stays; the push title says `→ Testing`); a pane gone two minutes →
  `gone`. `comments` are markdown, a path in one opening on the run's
  machine relative to its worktree; the UI re-reads an open task every
  10 s while a run is live.
- `internal/auth/` — single bearer token; HttpOnly cookie; no accounts.
- `internal/server/` — the HTTP adapter: one file per context (`roster.go`,
  `machines.go`, `sessions.go`, `chat.go`, `files.go`, `push.go`,
  `search.go`, `usage.go`, `quick.go`, `schedule.go`, `review.go`,
  `code.go`, `update.go`, `tasks.go`, `terminal.go` with `termhttp.go` and
  `peer.go`), each registering its routes in a `…Routes(mux)` that
  `Handler()` calls; a handler reads the request, asks one service and
  writes the answer, and the logic lives in the context packages.
  `/api/health` and `/api/login` are mounted outside the token guard;
  everything else behind it. Every route resolves its machine through
  `machineFor` (query → `X-Remuda-Machine` header → POST body `machine`)
  and `ready()`. The three create routes share `create()` (agent kind
  validated by `sessions.KnownAgent` *before* any RPC, container made,
  agent started by `sessions.StartAgent`, watcher refreshed); the two
  close routes share `closeHandler`, the two renames (`PATCH
  /api/tabs/{id}`, `PATCH /api/sessions/{id}`, body `{label}`) share
  `renameHandler` — herdr's `tab.rename` / `workspace.rename`.
- `internal/sessions/` — the input side of herdr: `Agents` / `KnownAgent`,
  `StartAgent` (retries `agent_pane_busy` for a few seconds), `Keys`
  (herdr's `send_keys` grammar), `RefOK` (a branch name git gets as an
  argument), and `Input`: one lock per pane, `Submit` (`agent.prompt`, else
  `send_text` + Enter; `ErrAgentBusy` while a menu is open), `SendKeys`,
  `SubmitToFolder` (the scheduler's door).
- `internal/chat/` — the conversation behind a pane: `Service.Open` (which
  file the chat lens shows: the pane's live one followed through
  compactions, a session of its folder by id, or a transcript on its own
  by folder and id — a `NotFoundError` for an unknown one), `SessionFile`,
  `LastWrite` / `LastWriteOfPane`, `LastTurn`, `LastToolCalls`,
  `LastAnswer`: the radar's recovery, the watchdog and the task watch read
  through it. `GET /api/chat?last=N` (1..200) answers the last N turns only
  — `Conversation.LastTurns`, the slice kept per file version in
  `tailCache`, so the wall's tiles and the lens on the same pane parse a
  file once per change; the ETag is the same either way, and every GET with
  a pane still marks it seen. In the UI a tab is renamed in
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
- `web/src/ui/` — the shared pieces, `index.ts` the barrel: `Dialog` (one
  scrim over the drawer, a head with icon/title/controls/X, Escape, focus
  in and back out, `#root` inert while open, `as="form"` + `onSubmit`,
  sizes sm/md/lg), `FormDialog`, `ConfirmDialog`, `Notice` (tones
  error/success/warn/info/hint, optional icon, action and dismiss),
  `Toasts`, `useTimedNotice`, `useDismiss` (popovers), `useSubmit`,
  `useDebounced`, `StatusDot`, `Logo`, `brand.ts`, `Markdown`.
  `lib/time.ts` (`ago`, `whenShort`, `fmtDate`, `fmtDay`) and
  `features/roster/options.tsx` (`machineOptions`, `folderOptions`,
  `machineLabel`) are the one way to say a time or build a roster select.
  Icons are `ui/icons.tsx` (inline 24px stroke paths, `<Icon name
  size={12|14|16}>`): never an emoji or a font glyph as an icon.
  `Select.tsx` replaces every native `<select>` (an icon per option, a
  portal so nothing clips it).
- `web/` — React + Vite + xterm.js SPA, embedded via `web/embed.go`
  (`go:embed all:dist`). Build with `make web` before `go build`; `npm
  test` runs Vitest on the pure modules (`*.test.ts` beside each; node,
  no DOM). Laid out by feature: `lib/` → `ui/` → `features/<name>/` (one
  folder per lens or panel, its server calls in its own `api.ts`) →
  `app/`; a dependency points down that list, and a feature never imports
  another feature's screens (its `api.ts` and pure modules, and the few
  shared pieces `agents/SendToAgent`, `files/target`, are fine).
  `features/terminal/engine/` is the terminal outside React (`engine.ts`
  wires `transport`, `modes`, `mouse`, `pointer`, `selection`, `layer`,
  `copy`, `links`, `paste`, `geometry`, `keys`); `Terminal.tsx` is the
  mount that owns one engine per pane. **Paste**: xterm brackets a paste only once it has seen the app's
  DECSET 2004 — and a viewer that attaches later gets no replay, so it
  never sees Claude Code's "bracketed paste on" from startup and would
  paste a multi-line text raw (every newline an Enter: the prompt goes
  off line by line). `engine/modes.ts` notes whether 2004 ever passed
  and `engine/paste.ts`, when nobody knows, wraps a text with line breaks
  in `ESC[200~ … ESC[201~` itself (CRLF/LF → CR, as xterm does); a single line stays raw. herdr's
  attach forwards the markers only to a pane that enabled the mode, so a
  plain `cat` sees none.

## Invariants

- **herdr owns every process and status.** remuda holds no session state;
  it must keep working after a restart while agents run.
- **The transcript is read-only**; input goes through herdr: `agent.prompt`
  preferred (refuses with `agent_blocked` while a menu is open — surface
  that as 409, never type into the menu), `pane.send_text` + 150ms +
  `pane.send_keys ["Enter"]` as the shell fallback. Everything per-pane is
  serialized by `sessions.Input`, one lock per pane. Never queue or auto-send user input.
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
- **Hardening (the 2026-10 review; keep every one):** `secureHeaders` in
  `Handler()` puts nosniff, `Referrer-Policy: same-origin`, `X-Frame-Options:
  SAMEORIGIN`, HSTS over TLS and the page's CSP (`appCSP`) on every
  response but the code-server proxy's (its own headers; `X-Frame-Options`
  forced to SAMEORIGIN there) and the file viewer's bytes (a stricter
  sandbox CSP). The code proxy strips `Cookie`/`Authorization` before
  forwarding — remuda's token never reaches a machine's code-server.
  `decode` caps a JSON body at 1 MB; the `-addr` server has header and
  idle timeouts. The sign-in URL is logged only on the first run or to a
  terminal; a wrong token is logged with the remote address and answered
  after 500 ms; `REMUDA_TOKEN` must be ≥ 32 characters. Session ids from
  herdr go through `text.SessionID` before naming a file or following
  `--resume` (the pane's own hooks report them: the agent chooses the
  value), and a `path` session must be a `.jsonl` under
  `~/.claude/projects`. Release asset URLs must be GitHub's for this
  repository and tag (`releaseURLOK`) and are shell-quoted. Uploads refuse
  a `.remuda`/`uploads` that is a symlink and write with `O_EXCL|O_NOFOLLOW`.
  The review diff refuses a path starting with `-` and passes `--` to git.
  Branch/base names for a worktree go through `sessions.RefOK`. ssh control sockets
  live under `<state>/ssh` (`sshx.SocketDir`) while the path keeps
  within 86 bytes, a unix sockaddr's 104 less the 17-byte suffix
  OpenSSH binds a master with, else in the temp dir, private on a Mac.
  `TS_AUTHKEY` travels on stdin, never on a command line. Colours are six hex digits or nothing
  (`normColor`). A run of an update must send back the command shown.
  The installer installs nothing whose sha256 is not the release's; the
  sign-in address reaches the browser through `osascript`'s stdin, never
  an argument; `uninstall.sh` asks before anything goes and deletes a
  state folder only when remuda's files are in it.

## Testing

`go vet ./...` + `make typecheck`. Live check against real herdr: create a
scratch workspace via `POST /api/sessions {"dir":"/tmp/x","agent":"claude"}`,
drive it via `/api/keys` / `/api/submit` / `/api/chat?pane=...`, then
`DELETE /api/sessions/{ws}`. Never type into panes you did not create — the
user's own sessions live in the same roster.
