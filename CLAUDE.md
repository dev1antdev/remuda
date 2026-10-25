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
  mark itself dead, so handlers never call MarkDead. `Client.PaneScreen` is `pane.read` with the
  colours kept (the wall's screen view). `PaneInfo` has
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
  and by script over a `shell.Host` on a remote. **A session's subagents**
  (`subagents.go`, tested): the agents it spawned through its Agent tool
  live at `<session>/subagents/agent-<id>.jsonl`, a `.meta.json` beside
  each (`toolUseId`, `agentType`, `description`, `finished`);
  `Reader.Subagents(path, now)` lists the folder in one stat round trip
  (kept 5 s on a remote) and re-reads only a file that moved, summing
  each up as a `Subagent` — its tool calls, the one still running
  (`Doing`), the start of its last answer (`Report`), its span, and
  `working` / `quiet` (nothing written for `QuietAfter`) / `done` (the
  meta's finish, or an answer that stood for `ReportedAfter`: the finish
  is not written for every kind of agent) — with a stamp of the folder's versions for the chat's
  ETag; `Reader.SubagentTurns(path, id)` is one agent's own conversation
  (`SubagentID` guards the id). Their records are sidechain ones, which
  `Parse` drops from the parent file (`parseRecords(r, keepSidechain)`). `Turn.Answer`,
  `Turn.ToolCalls` and `Turn.LastID` say what a turn ended with; a tool
  result that is a picture alone reads `[image]` (`resultText`), so the
  call counts as answered and the chat draws the picture under it; a
  tool part carries `tool_id` (the tool_use id), `at` (when it was
  called) and `took`, the seconds to its result (`secondsBetween`), 0
  while it runs. `Plan(turns)` (plan.go, tested) is **the agent's plan**,
  the task list its own calls keep — TodoWrite replaces the list,
  TaskCreate adds an item numbered by the tool's answer ("Task #3
  created"), TaskUpdate moves, edits or deletes one — as `PlanItem{ID,
  Text, Active, Status}`, pending / in_progress / completed; read from
  the transcript alone, so a remote conversation has its plan too. **An answer says what it spent** (`spent.go`, tested): `Took`, the seconds
  from the person's prompt to the turn's last record, and `Tokens`, the
  input, output and cache tokens of its API responses — one per
  `message.id`, the last record of each, as `usage.Accumulator` counts —
  with the responses kept on the turn (`Responses`, not serialized) for
  `usage.PriceTurns`, which `chat.Conversation.Turns` runs over every
  parse (so the tail cache holds priced turns) to set `Cost` at API
  prices and `Model`; `TurnView` draws them as a quiet foot line, "4 min
  12 s · 38k tokens · $0.42" (`spent` in `lib/time.ts`, `tok` / `usd`),
  the model and what the numbers mean in its title. The chat lens (`features/chat/Chat.tsx`: the transcript through `useChat.ts`, the one fetch hook — every 2 s while the agent works or asks, 10 s otherwise, a 304 = unchanged, `last=N` for a wall tile; `Composer.tsx` (`compact` in a tile; ↑ on an empty line brings back what was sent from this device to that folder, newest first, ↓ walks forward and back to the line being written — pref `prompt.history:<machine>|<dir>`, the 50 newest, a repeat moved up; `history.ts` the pure part, tested, `useHistory.ts` the hook), `export.ts` (**Export as Markdown**: the conversation shown — the pane's live session or the older one picked in the lens, store field `chatSession` set by Chat — fetched whole and saved as `<workspace>-<day>-<session>.md` through `saveText` in `lib/host.ts`; text as it is, thinking and tool calls folded in `<details>`, outputs fenced past any backticks; `conversationMarkdown` / `exportFileName` tested; Export as Markdown is a row of the View menu (`useChatViewItems` in `ChatTools.tsx`) while the chat lens is on screen, and the palette has it for the pane and for a transcript open from the search; the row beside it is **Recap**, dialog kind `recap` — `features/standup/RecapDialog.tsx` over `GET /api/recap`, Copy as Markdown — in the palette too, for the pane and for the viewer); `FindBar.tsx` (**Find in this conversation**: ⌘F / Ctrl+F by key code while the chat lens or the transcript viewer is on screen and no overlay is open — from the composer too, since no field there has a find of its own — the 🔍 toggle in the tab strip's tools slot (`ChatTools.tsx`; the viewer bar's action for a transcript on its own), whose bar — the field, "3 of 12", ↑ ↓, × — opens to its left so nothing else in the strip moves, and a palette command; Enter / Shift+Enter step, Esc closes; store field `find` — the query, the current match, the count, the ⌘F presses so each puts the focus back in the field — written by the bar and by `useFind.ts` in Chat, which walks the log's text nodes (a folded turn renders none, so only what is on screen counts) and marks every match through the CSS Custom Highlight API, `::highlight(remuda-find)` and `remuda-find-current` — no node changes, so a selection and the memoised turns stay — brings the current one into the middle when it is chosen, not on every poll, and outlines its turn with `.turn-hit` where the API is missing; `find.ts` the pure part, tested: `findRanges` case-folded on the text's own offsets, `step` / `clamp` / `countWords`, at most `MAX_MATCHES` 2000); `TimelineBar.tsx` (**the session's timeline**: a thin bar over the log of the chat lens and of the transcript viewer — one block per prompt and its answer laid out in time, a gap over 10 minutes drawn as 10 so a break does not flatten the day — with the moments worth a look marked, the first failed call, the longest call, a loop of three or more identical calls in a row, every question the agent asked, a compaction, a `linkish` chip for each kind under the bar, and every block, mark and chip a jump to its turn through the search's own `jumpTo`; pref `chat.timeline`, on with a mouse, a checked row of the View menu in the pane and in the viewer, a palette command; `timeline.ts` the pure part, tested: `timeline`, `squeezer`, `chips`, `summary`); an older session picked in the lens's session picker gets **Resume in a new tab** beside it — `POST /api/tabs` with `resume`, the agent of the pane started on that conversation in the same workspace, the new pane selected, `PlanBar.tsx` (**the agent's plan** over the log — "Plan 3/7 · Building the page", the agent's own active form, unfolding to its list with a mark per item, nothing when it keeps none; from the answer's `plan`, so the viewer has it too; `plan.ts` the pure part, tested: `counts`, `current`, `progressWords`, `planLine`, `planCallWords`), `TurnView.tsx` (a tool call's summary ends with how long it took — `took` in `lib/time.ts`, a tenth under ten seconds — a failed one in red, and the answer's foot counts its failed and repeated calls, `calls.ts` tested; a picture a finished Read or Write was about — `imagePath` in `footprint.ts`, tested — is drawn under the call as a thumbnail from the machine's own file through `/api/fs/file`, a click opens the file viewer; a file gone since draws nothing; under an Agent call, a `SubagentRow.tsx` per agent it spawned — a state dot, the description, the call it is on or the first line of its report, its tool calls, Open → the transcript viewer on that agent's own conversation (store `viewer.subagent`, `GET /api/chat?…&subagent=<id>`); `subagents.ts` the pure part, tested: `subagentsOf`, `sameSubagents` so a poll redraws no row that stands, `subagentLine`, `subagentWords`; a TodoWrite / TaskCreate / TaskUpdate call is one line, the item it added or moved (`planCallWords`), never its JSON), `QuestionCard.tsx` (**the question itself** while the agent waits: `GET /api/question?machine&pane` — `internal/server/question.go` → `chat.ParseQuestion`, tested — reads the dialog off the pane's visible screen every 2 s: Claude Code's permission prompts, its trust prompt and an AskUserQuestion, the cursor ❯ on one line of a list, numbered or aligned inside a box, the dialog's words above it, a wrapped choice or the line describing one glued to it; the card shows the first line, the last line and, in the lens, the detail between clamped to three lines, the whole in the title, and a `btn` per choice — a click walks the cursor there with ↑ ↓ and presses Enter, one key a request, Esc beside them; `question.ts` the pure part, tested; a screen with no dialog on it leaves `PromptKeys.tsx`, the blind ↑ ↓ Enter Esc), `Tail.tsx` (a tile's chat: the last 12 turns, 3 s / 15 s, the state line's "3/7" from the tail's `plan`; `TailAnswer` carries the question card in both views of a tile and in the pop-out), `FilesRail.tsx` (the **files rail** beside the lenses: every file the agent edited or read in this conversation since its last compaction, from its own tool calls — `footprint.ts` is the pure part, tested: `toolPath` reads `file_path` / `notebook_path` from an object input or from one the server cut to a string at 8 KB, so `TurnView` links a big Write too (a link only for a tool that read or wrote the file: Grep's or Glob's path is a folder, named only); edited files first by their last touch, the ones only read folded; its own `useChat` poll at a tile's pace, only while on screen; a row opens the file viewer on its changes, the code button `actions.openInCode`; a change made through a shell command is the Changes lens's to show, not the rail's), `QuickPrompts.tsx`, `dictation.ts`) follows the bottom (`useFollowBottom.ts`)
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
- **Nothing in a bar moves when state changes.** The page's places are
  **the rail** (`app/Rail.tsx`) down the left edge: the five modes, then
  Standup, Usage, Schedules, Activity (a dot for news) and Settings,
  each an icon with its name under it and a tooltip; under 920 px it
  lies along the bottom with Agents, Wall, Tasks, Review and More (a
  menu with Code, the panels and Settings), and yields while the
  composer has the keyboard. The header is `[burger][title — takes the
  slack][usage pill][lens switch][pop-out][search]`: the lens group is
  anchored right, the pill grows leftwards into the title's slack, the
  three lens buttons have equal widths so a badge widens nothing, and
  the Changes button is disabled (not removed) when there is no folder. The tab strip's right slot (`TabStrip tools=`) holds the lens's one
  control — Find for the chat, the key bar switch for the terminal — and
  **the View menu** (`app/ViewMenu.tsx`): the lens's own rows
  (`useChatViewItems` in `chat/ChatTools.tsx`: the timeline, Recap…,
  Export as Markdown; `useTerminalViewItems` in
  `terminal/TerminalLens.tsx`: the size mode, the minimum width,
  selection mode, smaller / larger text that keep the menu open, the key
  bar) and then the page's: the files rail and full screen; the viewer
  bar has the same pair for a transcript on its own. Never a row of icons
  in the header. Transient status — offline,
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
  release and every pull request, and on `release` (and by hand,
  `workflow_dispatch`) the end-to-end smoke too: `make e2e` is
  `scripts/e2e.sh` — a throwaway herdr server on a socket and HOME of its
  own (`HERDR_SOCKET_PATH`, stopped the same way), a remuda on it with its
  own state dir, headless Chrome — driving `e2e/smoke.mjs` over the
  DevTools protocol with no dependency: sign in, a scratch shell
  workspace through the API, a command typed into the terminal lens and
  read back through `/api/screen`, the chat, the Wall, Review, Activity,
  the palette, the workspace closed, no console errors; the temp folder
  stays on a failure, with the screenshots (an artifact in CI). Pull
  requests target `release`; `main` is what is out (AGENTS.md, "Build and
  check").
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
  probe's `herdr_server` line is what offers it. **One chat by hand**:
  `Restarter.Pane(mc, paneID, say)` is the same leave-and-resume for a
  single pane, behind `POST /api/panes/restart {machine, pane}` →
  `updates.Service.RestartPane` → a job (`GET /api/jobs/{id}`); the
  workspace's ⋯ menu ("Restart Claude Code on its session…", the active
  tab's agent pane) and the palette offer it behind a confirm, and
  `features/updates/restartPane.ts` follows the job with toasts.
  `restart.Resumable(pane)` is the one check — refused for a shell, an
  agent the table does not know, no session id, a question open
  (`/exit` typed into a menu is wrong) or an agent still working (a
  restart by hand is never booked for later: the person asks again once
  it has settled, or sends Esc) — run on the watcher's snapshot before
  the job starts (a 400 with the reason) and on a fresh look inside it;
  a pane restarted within the last two minutes is refused too.
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
  state is decided there, not in render) and memoized per text; quotes
  and lists nest at most `MAX_NESTING` (16) deep and then read as text (a
  line of three thousand `>` recursed past the stack), and a line over
  `LONG_LINE` (10 000 characters) is drawn plain, with no marks or links
  (the inline regex costs its square on a run of one character);
  `markdown.test.ts` covers it. `ui/ErrorBoundary.tsx` is the one class
  component: React unmounts the whole tree on a render error, so every
  turn (`TurnView`) draws inside one — a turn the parser cannot draw is a
  note, the rest of the chat stands — and `App.tsx` wraps the page in one
  with a Reload button, never a blank page.
- `web/src/lib/` — what every screen uses and no screen owns, React-free
  but for the hooks: `http.ts` (`check` / `getJSON` / `sendJSON`, every
  request), `query.ts` (`useQuery(fn, deps, {every, enabled, keep})` —
  the one way a screen reads the server: a fetch that follows its inputs,
  polls while enabled and the page is visible — `visibility.ts`:
  this document or the pop-out's — drops an answer that
  arrives after the inputs changed; `undefined` from the fetcher means
  unchanged (an ETag 304) and the data stays; `reload`, `fetching`, and
  `mutate` for what the screen already shows ahead of the server — a
  switch flipped, a card dropped), `store.ts` (`createStore` over
  `useSyncExternalStore`: `get` / `set` / `subscribe` / `use(select)`),
  `prefs.ts` (`usePref`, localStorage-backed and shared: every hook of a
  key follows a write through any other or through `writePref`, over
  `useSyncExternalStore` — the terminal's tools in the tab strip and the
  lens beside them hold the same keys; a `storage` event from another
  tab drops the cached value and wakes the hooks; `readPref` keeps a stored
  value only in the fallback's shape; `isTouch`), `media.ts` (`useMedia`:
  a media query as a hook, for a piece the stylesheet hides that would
  otherwise poll unseen), `clock.ts` (`useNow`: the time, refreshed every
  so many ms, for a counter on screen), `text.ts` (`errMsg`,
  `basename` / `dirname`, `plural`, `shellQuote`), `time.ts`, `paths.ts`,
  `host.ts` (`copyText`: Clipboard API, else execCommand; `saveText`: a
  file for the browser to keep; `openUrl`: a
  new tab — nothing embeds remuda any more, the JetBrains plugin and its
  `window.herdrHost` bridge are gone), `transfer.ts` (`filesFrom`),
  `version.ts` (`newer`), `theme.ts` (the two Catppuccin flavours,
  `resolveTheme`, `ansiPalette` / `terminalTheme` / `pageColor`),
  `perf.ts` (`watchStalls`: the browser's long
  tasks from 250 ms up, each reported with where the page was;
  `stallWords`, `toastDue` tested).
- `web/src/app/` — the shell. `store.ts` is the one page store: the
  *requested* pane (a deep link, a notification, a click), the lens, the
  mode, the drawer, the open dialog / menu / panel (the panels: the
  palette, search, alerts, activity, schedules, usage, standup, the
  release install), the notice, the
  confirm, the file viewer, the search viewer, a jump, the find bar; `actions` are the
  only writes. `current.ts` derives the shown pane from the requested
  one, falling back to the first agent pane — nothing fixes selection up
  in effects. `url.ts`: the address bar follows the store
  (`?machine=&pane=&mode=&lens=`, `history.replaceState`) and is read
  once at load, so a link or a reload lands on the same pane in the same
  lens. `act.ts` (`act(promise)`: refresh the roster, else toast the
  error), `toasts.tsx`, `diag.ts` (`startDiagnostics`, from App: every
  stall of the main thread goes to the console as `[remuda] The page
  stalled 4.2 s on the Wall (13 tiles)` with the mode, the lens and the
  pane — what the Wall freeze hunt lacked — and one from 2 s up becomes
  a toast, a minute apart at most), `Shell.tsx` (URL sync, ⌘K → the palette, ⌘F → the find bar, the service worker's
  select-pane, mark-seen, the four modes), `Rail.tsx` (the places),
  `Header.tsx`, `PaneArea.tsx`
  (the lenses, mounted once seen, and the files rail beside them in
  `.pane-body` — store field `footprint` from the pref of that name,
  default on with a mouse, a row of the View menu whatever the lens,
  disabled for a shell pane, hidden with the rail under 920 px and not polling there; **full screen** is the View menu's last row, `app/full.tsx` — store field `full`: the header, the sidebar and the rail go, the tab strip stays with its tools, the browser's own full screen is asked for where it exists; Esc outside a field or the terminal leaves, so does the browser's exit, and a mode change clears it; the palette has it), `Wall.tsx` (the **Wall mode**: every
  agent pane on a connected machine as a tile — the sidebar row's head
  (mark, logo, label, workspace · machine, the pull request the agent named as a link with its checks and review beside it (`internal/prstatus`), context % · cost), the last 12
  turns through `features/chat/Tail`, a compact composer; a click on the
  head opens the pane in Agents on the chat lens. `features/roster/wall.ts`
  says which panes make a tile and in what order — the roster's, stable, so
  no tile jumps on a status change — and leaves out the ones hidden by hand
  (pref `wall.hidden`, the eye on a tile, "Show N hidden" resets) and, with
  "Hide idle", the idle ones past the radar's fresh tier, and with **Needs me** (pref `needsMe`, `needsMe` in wall.ts) everyone but the agents waiting for the person — a question open, a watchdog note, a finish not looked at — "Nobody needs you right now" when that leaves none; a tile that needs one wears a pulsing ring (red for a question, amber for a note), a finished one a green edge; the two filters and the view are palette commands too, and **Alt+N** (`app/next.ts`, by key code; the palette row "Go to the next agent that needs you") walks the agents that need the person in the roster's order, round and round (`nextNeedingMe` in wall.ts, tested): on the Wall the tile takes the wall, elsewhere the pane opens on its chat, a toast when nobody needs one; pref `wall` holds
  the size S/M/L (`TILE_MIN` 360/460/600 px, an auto-fill grid) and the idle
  switch; no sidebar in the mode, "+ Agent" opens the New workspace
  dialog; the tile itself is `Tile.tsx`, `TileView`; a tile shows the chat or, in the **Screen** view (`view` in pref `wall`: the Chat | Screen pair in the wall's bar, "Wall of screens" in the palette, the pop-out follows it), the pane's terminal as it is — `features/terminal/Screen.tsx` reads `GET /api/screen?machine&pane` (`internal/server/screen.go` → `herdr.Client.PaneScreen`, `pane.read` with `format: ansi`, read from herdr on demand at the tiles' pace, 3 s / 15 s, an ETag over the text so an unchanged screen is a 304; no attach, so no slot is taken and the shared pane is never resized) and draws the rows through `features/terminal/ansi.ts` (`parseAnsi`, tested: SGR colours in xterm's palette, 256 and true colour, bold / dim / italic / underline / inverse, every other escape dropped) at the font that fits the pane's columns in the tile's width (`thumbFont`, `engine/geometry.ts`), the bottom rows kept when the height runs out; a click opens the terminal lens, the head the lens of the view; `TailAnswer` (chat/Tail.tsx) is the foot under either view — the keys for a question and the compact composer; under the head a **state line** that is always there, `TileState` in `chat/Tail.tsx` — `stateWords` with how long the state has lasted (`radar.since`, `elapsed` in `lib/time.ts`) and, while the agent works in the chat view, what it does this second from the end of its tail (`features/chat/doing.ts`, tested: the tool call still without a result through `toolLabel`, now in `footprint.ts`, and how long it has run — `Running` in `Tail.tsx`, a second at a time through `useNow` in `lib/clock.ts`, amber past `LONG_CALL` 60 s — else thinking or writing; an Agent call without its result reads as waiting on its subagents, and the line says how many of the conversation's still work, "2 subagents working", from the answer's `subagents`), while its plan has open items how far it is, "3/7" (`progressWords` in `chat/plan.ts`, from the tail's `plan`), and once it has finished or sits idle, **what it answered**: the first line of the text its last turn ended with, in the quiet colour — the chat view reads it from the tail (`features/chat/said.ts`, tested: marks stripped, cut at a word by 120 characters), the screen view and the strip from the roster's `pane.said` (`internal/said`, the same cut)), **the pop-out**
  (`pip.ts` owns the one small window Chrome's Document
  Picture-in-Picture gives — `popOut(selection)` from the click itself,
  the page's stylesheets copied in, `pagehide` clears the store; one
  window at a time, the browser's rule; `popOutSupported` decides whether
  the buttons exist at all, so nothing in a bar appears or vanishes; the
  window's document is registered with `lib/visibility.ts`, so the polls
  behind it go on while the tab behind it is hidden, and the follow-bottom
  hook uses the log's own window and document for its frames, selection
  and mouse-up.
  `PopOut.tsx` draws the popped pane through a `createPortal` into that
  window's body — the same `TileView` as the wall, the roster, the store
  and every handler shared, "That pane is gone" once the roster lost it.
  Store field `popout`, buttons on every wall tile and in the header,
  disabled there without a pane), `Overlays.tsx` (panels, dialogs, the
  menu, the confirm, the file viewer; the keyboard sheet behind **?** — Shift and the slash key by position, never over a field or another dialog, the same key closes it — is `features/help/KeysDialog.tsx` over `help/keys.ts`, the one list of every key the page answers, tested; the palette has it as "Keyboard shortcuts"), `commands.tsx` (the **command
  palette** on ⌘K / Ctrl+K, by key code so any layout works: every pane
  as a row to jump to — each machine's liveliest first, shells at its
  end, the current one marked — and every command the page has a button
  or a menu item for: the other modes, the other lenses of the pane, new
  workspace / tab / worktree, the renames, close workspace, pop out, the
  files rail, the panels' widths back to usual, alerts, schedules,
  usage, add a PC, and last, whatever was
  typed, "Search every conversation…", which opens the search panel with
  the query (`actions.openSearch`, store field `search`). `commands.tsx`
  is the one list; `features/palette/Palette.tsx` shows it — ↑ ↓ ↵ Esc,
  the mouse — and `features/palette/rank.ts` is the pure part, tested:
  `paneEntries` builds the rows from the roster through `radar.ts`
  (`rowLabel`, `stateWords`, `markOf`), `rank` keeps the entries every
  typed word hits and orders them — the whole title, its start, a word's
  start, anywhere, a title's initials ("gw" → Go to the Wall); a title
  counts double, a shell half — panes before commands, ties in the given
  order). The roster is read once for the
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
  edits since the look; a file deleted since diffs as its removal), else
  against HEAD (an untracked file whole) — always `--no-ext-diff`, so a
  difftastic set as the external driver stays out of it.
  The lens shows fresh files first and folds the rest behind "N files
  unchanged since you looked"; seen commits are dimmed. Every
  `events.Snapshot` goes to `review.RefreshBadges`, which runs `Badges` a
  minute apart and never two at once per machine, one shell over
  `Snapshot.WorkspaceDirs()`, and the roster carries `session.dir` and
  `session.review {changed, fresh}` for the sidebar badge.
  `features/review/Review.tsx` draws it (`Diff.tsx` the file rows,
  `Hunk.tsx` a hunk's rows in one column or two with a comment box under
  the tapped row, `hunks.ts` the parsing — `splitHunks`, `rowsOf`,
  `pairRows` lays a removed run beside the added run that follows it, a
  "\ No newline at end of file" note staying with its side,
  `partners`, `changedSpan` the words that differ between a removed line
  and its replacement, out to word boundaries and never between the
  halves of a surrogate pair, drawn as `.fv-mark`; `Hunk` is memoised and
  the comment box holds its own text, so a keystroke or a roster poll
  redraws no cells, and the focus returns to the row when the box closes;
  `keys.ts` the keys — `useReviewKeys`, by key position so a Cyrillic
  layout works: j k walk the files, ↑ ↓ the lines of the open one, n p
  its hunks, c comments on the focused line, o opens the file there, s
  flips the two columns (pref `review.split`, on by default with a mouse,
  the toggle in the lens head) and keeps the line focused; the rows walk
  with or without an agent to comment to, and are collected only for a
  key that needs them; the legend in the head, hidden under
  1180 px and on touch; the hook listens only while the lens is on screen
  (`active`) and never over a field, a dialog or the file viewer;
  `VerdictDialog.tsx`, `draft.ts` the comments and the messages they
  become): commits, files (fresh first), hunks with a comment box whose
  text goes to the pane's agent through `/api/submit` as
  "Review feedback on path:line". Nothing is written into a repository.
  **The Review mode** (`features/review/ReviewQueue.tsx`, the fifth mode,
  `?mode=review`): every workspace with uncommitted work in one list —
  `queue.ts` (`reviewQueue`, tested) reads the roster's `session.dir` and
  `session.review`, the ones with changes since the last look first — and
  the Changes lens of the picked one beside it, its pane the workspace's
  first agent pane (a shell when none runs there, comments then go to any
  agent), `onSeen` the roster's refresh; under 920 px the list and the
  lens take turns, a Queue button leads back. `actions.openReview(machine,
  dir)` (a task's run has a **Review** link beside Open) switches to the
  mode with store field `reviewAsked`, which the queue picks once the
  roster lists that folder and clears; a folder without changes is a toast.
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
  rejected: work on it and say when it is ready for another look; an
  approval with "and ask the agent to commit" ticked adds `COMMIT_ASK`,
  the button reads **Approve & commit** — the agent commits, remuda never
  writes into a repository) —
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
  cached 15s; `?check=1` re-probes) and reads the workbench page for
  the remote authority and code-server's version (`settingOf`, tested),
  which the Code bar shows beside a reload button (a fresh frame for the same folder when the editor lost its connection or shows a stale state). **The editor wears the page's theme**: `POST /api/code/{machine}/theme {theme}` → `code.Service.Theme`, once per machine and theme while remuda runs — Catppuccin's extension installed from Open VSX when missing, then `workbench.colorTheme` set in code-server's `settings.json` (`WithTheme`, tested: merged as JSON, else the line replaced in a file with comments), which the editor applies as the file changes; `Code.tsx` asks when the frame is up and whenever the page's theme changes; the installer seeds the same. **Its version** rides on the vitals (`code_server`, the release the `~/.local/opt/code-server` link names) and `Latest.Code` is `setup.CodeVersion`, the release the installer carries (`health.PinnedCode`, set by the app): the ⓘ says when a machine lags it and Updates offers `what = code`, `remuda code install` on the machine (`updates.Plan`, tested). `web/src/features/code/Code.tsx` is the picker (machine, then a folder from the roster's workspaces — or **All workspaces on the machine**, its first row and the default past one folder: `POST /api/code/{machine}/workspace` writes every folder herdr has a workspace in to `~/.config/code-server/remuda.code-workspace` on the machine, `code.Service.Workspace` / `WorkspaceJSON`, tested, and the frame opens `?workspace=` on it; the file is written again whenever the roster's folders change, and the editor, watching it, takes the new folders in place) and the iframe, kept mounted behind the other modes once opened. `actions.openInCode(machine, folder, path, line?)` (the files rail's code button, the Changes lens's per-file button, the file viewer's header button — the last two at a line; store field `codeFile`, a folder of "" keeps the one picked) switches the mode and moves the picker to that machine and folder; **when that editor is already on screen** the file goes to it in place — `POST /api/code/{machine}/open {path, line}` → `code.Service.Open`, code-server's own CLI with `--reuse-window` over its IPC socket (`OpenScript`, tested; `file:line` carries the line; the CLI exits 0 without a listener, so its last line is read for "error") — and nothing reloads; only a first frame mounts with `payload=[["openFile","vscode-remote://<authority>/path"]]`
  (the path escaped per segment), which the workbench opens on load —
  the authority is what code-server's own page states (`Status.Authority`,
  read off `data-settings` by the probe: "remote" since 4.9x, the host
  before; the page's host when the probe found none), or the editor
  answers "Unable to resolve resource"; the frame's address is settled once per request, so
  coming back to the folder later does not open the file again; a path that is the folder itself opens no file (the rail can name it after a Read of the directory). **Follow** (the Code bar's button, pref `code.follow`): while the pane on screen in Agents runs an agent on this machine, its tail is polled at a tile's pace (`useChat`, `last=4`) and every edit it finishes — `newestEdit` in `code/follow.ts`, tested: the last Edit / Write / MultiEdit / NotebookEdit with a result and no error, by `tool_id` — opens in the editor the same way; what was already there when Follow began is not news. The flag `-code-port`
  changes the port on every machine at once.
- `web/src/lib/paths.ts` — `findPaths` / `findUrls`, shared by the terminal link
  provider (`terminal/engine/links.ts`) and the chat linkifier; keeps a
  `:line` suffix, drops trailing punctuation, never a path inside a URL.
  A link needs a name a file would have — a dotfile (`.env`, `.gitignore`;
  the dot kept on `.mcp.json`) or a name with an extension, in any
  alphabet — so a folder (`internal/server`) or a slashed phrase
  (`Approve/Reject`) is none. Every alternative of `PATH_RE` starts only
  where a token starts (a lookbehind), so a line costs its length: tried
  from every position inside a long token, the greedy segments cost its
  square, and a CSV row with base64 blobs pasted into one agent's chat
  froze the whole page for a minute at every render of that turn — the
  Wall first, since it renders every agent's last turns (`paths.test.ts`
  keeps the bound).
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
- `internal/push/` — Web Push (RFC 8291/8292 via webpush-go); `OnAlert` hands
  every alert to the activity feed as it fires. VAPID keys at
  `<config>/remuda/vapid.json` (a damaged file is an error, never a silent
  rotation — rotating orphans every device); subscriptions in
  `push-subscriptions.json`, 404/410 from the push service drops one. The
  scheduler is the point: `blocked` alerts after 10s, `done` (only from
  `working`) after 60s for turns ≥ 60s or 10s with `done:"always"`; **any
  status change on the pane cancels its pending alerts** — answered at the
  desk means no push. Delays are vars so `push_test.go` can shrink them.
  **Quiet hours** (`quiet.go`): a device's prefs may carry `quiet {from,
  to, tz}` — "HH:MM" by its own IANA zone, the zone database embedded so
  it loads on any host; `Normalize` drops a window that does not parse;
  every send goes through `deliver`, which skips a device whose window
  covers the moment of sending (`Quiet.Covers`: a window past midnight
  wraps, equal times are no window, an unknown zone reads as UTC) — the
  timers, the watchdog's `Notify`, the schedules' and the tasks' pushes
  alike, while `OnAlert` still records every one for the feed; a test
  push is sent regardless, since the person just pressed the button.
  The page re-reads the device's zone on every load (`readAlertPrefs`),
  so a window set at home fits the clock on a trip; the window last set
  is kept on the device (pref `alerts.quiet`) while the hours are off.
  Routes: `GET /api/push` (public key), `POST/DELETE /api/push/subscribe`,
  `POST /api/push/test`. **Read aloud** (`features/alerts/voice.ts`,
  tested; `useVoice.ts`, called from Shell): with the switch in the
  alerts panel on (pref `alerts.voice` {on, voice}), this device speaks
  through the browser's own `speechSynthesis` — nothing passes through
  remuda — what an agent answered when it finishes (the roster's
  `said`), that one waits for the person, a watchdog note: only the
  changes seen while the page is open and in front, each once by a key
  per episode (`linesToSay`; the first roster only seeds), in the voice
  picked in the panel, else a Russian voice for Cyrillic text when the
  browser has one.
- `web/public/sw.js` shows every push (Safari revokes silent subscriptions),
  silent while a window is visible; click → `select-pane` message to an open
  window or `openWindow(/?machine=&pane=)`; `pushsubscriptionchange`
  re-subscribes. `web/src/features/alerts/push.ts` is single-flight and re-registers on every
  load with permission granted. Push needs a secure context: localhost or
  `tailscale serve` HTTPS; iPhone needs the home-screen app.
- `internal/feed/` — the **activity**: what happened across the agents while
  nobody was looking. `push.Service.OnAlert` hears every alert the pushes
  carry, or would have carried without a subscribed device — a question
  after 10 s, a finish after 10 s (any turn), a watchdog note, a schedule
  that failed, a task that moved — and `feed.Service.Add` keeps it in
  `feed.json` (300 newest, numbered); `GET /api/feed?limit=` lists them, the
  roster carries `feed` (the newest id) so the bell shows a dot when it is
  past pref `feed.seen`. `features/activity/Activity.tsx` is the panel behind
  the bell (store panel `activity`): by day (`group.ts`, tested), a row
  opens its pane, the alert settings at its foot; the palette has it too.
- `internal/radar/` — the sidebar's reading of every agent, after
  hhdebb/herdr-radar's design and written here (`radar_test.go` covers the
  rules). The watcher feeds every snapshot into the `Tracker`; the roster
  carries `pane.radar = {state, last_active}` for agent panes. Rules: a
  blink through idle within 2.5s of working is still working; **done is held
  until seen** (herdr focused the pane, or a client opened its terminal /
  chat, or `POST /api/seen` — a tile's tail, `GET /api/chat?last=N` on the
  Wall or in the pop-out, is not a look) — and a seen finish stays seen until the agent works again, because herdr itself keeps reporting `done` for hours, until the pane is focused in herdr; **a question is held until the agent works
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
  `HealthButton.tsx`, `MachineMenu.tsx`; its head is a + menu — a
  workspace on a connected machine, a PC — and a ⋯ menu — message
  several agents, the order, Sidebar settings; `RadarSettings.tsx` is
  `RadarFields`, the order, the row label, the idle tiers and the
  shells switch, shown by **Settings** (`features/settings/
  SettingsDialog.tsx`, dialog kind `settings`, the rail's last button
  and a palette row: the theme, the panels' widths, the sidebar, the
  way to the alerts and the keys, the version); one spinner clock for
  the braille frames and the pulsing question). A finished or
  idle row says what the agent answered after its title, in the quiet
  colour and in its tooltip (`pane.said` from the roster, `.radar-said`:
  the title keeps up to 62% of its line, the answer the rest, each
  clipped with an ellipsis); the row's height and its other parts stay
  where they are.
  Every remote machine has a hue of its own (`features/roster/hue.ts`,
  tested: from the id, never red or green, which mean blocked and done):
  the sidebar block's left edge, a wall tile's left edge, a dot before
  the machine in a palette row; the local machine has none.
  Machines and workspace groups fold (prefs `collapsed` / `wsFolded`); a
  folded group's head carries its agents' states (working / ? / ✓ counts).
  `web/marks/*.svg` are the vendor logos (MIT packaging, see NOTICE.md),
  `npm run marks` regenerates `src/ui/marks.ts`; drawn inline in currentColor.
- `internal/said/` — the first line of every finished agent's last answer,
  for the roster (`pane.said`) and so for the sidebar row and the wall
  tile. `Tracker` hears `events.PaneStatus` (a finish — done or idle —
  queues a read, a start of work clears the line, a closed pane is
  forgotten) and `Observe` seeds from every snapshot the agent panes
  already finished when first seen, once; the reads go through
  `chat.Service` (`SessionVersion`, then `LastAnswer`) in one goroutine
  per machine, off the request path — a remote read is a script over
  ssh — and a file whose version is the one read last is not parsed
  again. `FirstLine` strips the Markdown marks (headings, list and quote
  marks, emphasis, code, links, tags; fences and rules skipped) and cuts
  at a word by `Max` 120 characters; `features/chat/said.ts` is the same
  cut on the page, both tested. The same read notes **the pull request
  the agent named** (`transcript.PullRequest`, tested: the last GitHub
  `/pull/N` or GitLab `/merge_requests/N` in the assistant's own text;
  `pane.pr {url, label}` on the roster, kept while the agent works
  again, gone with the pane): a "PR #15" / "MR !9" link in a wall
  tile's head, in the sidebar row's tooltip, and "Open PR #15" in the
  workspace's ⋯ menu; a link first seen or changed goes on the bus as
  `events.PullRequestNamed`.
- `internal/prstatus/` — **the state of the pull request** an agent
  named: `Tracker` hears `events.PullRequestNamed` and, for a GitHub one
  (`/pull/`), runs `gh pr view <url> --json state,isDraft,mergeable,
  reviewDecision,statusCheckRollup` on the pane's machine (`shell.Text`
  over a `shell.Shell` the app finds through `fleet.Get`; the script from
  `internal/script`; `Parse` counts the rollup's check runs and status
  contexts as pass / fail / pending / skipped, tested on `shell.Fake`) —
  read when the link appears, then at most every `Every` (2 min) on
  `Touch`, which the roster handler calls, so a page nobody reads asks
  GitHub nothing; one read in flight per machine; a pane gone
  (`events.PaneStatus` to `Gone`) or a machine removed is forgotten; a
  GitLab merge request is left alone. No gh, not signed in or a machine
  away leaves the status absent with its reason, logged once an hour per
  machine. The roster's `pane.pr.status {state, draft, mergeable,
  review, checks {total, pass, fail, pending, skipped}, at}`
  (`rosterPR`, so the ETag moves with it); `features/roster/prStatus.ts`
  (tested: `checksWords` "2/2 checks" / "1 failed" / "3 running",
  `reviewWords` merged / closed / draft / conflicts / approved / changes
  requested / review pending, `checksTone` and `reviewTone` — red /
  amber / green / dim / none, each chip its own — and `statusTone`, the
  graver of the two, `prWords`) draws it as two `.pr-state` chips in the
  `.tile-pr-group` beside the link in a wall tile's head (the review
  chip goes in a tile under 560 px — `.tile` is a size container — and
  under 920 px), in the sidebar row's tooltip and in the workspace
  menu's "Open PR #15" row. gh runs as the person on their own machine, like
  git does for the Changes lens: remuda itself still makes no request
  but the version checks.
- `internal/recap/` — what a conversation came to, and the **standup**
  over every agent. `Of(turns, since)` (pure, tested) counts the turns
  from `since` on: prompts, answers (turns that ended in text), the
  seconds and tokens and cost of the answers, tool calls by name, the
  failed ones (`IsError`), the longest (`Took`, with its file or
  command), the shell commands (the first `MaxCommands`), the files
  edited and the ones only read (`inputField`: an object input, or the
  string a big one was cut to), subagents started (Agent / Task),
  questions asked and whether one is still open, the first and last
  times, the first line of the last answer (`said.FirstLine`).
  `Markdown(items, since, now)` is the report to paste: a heading per
  agent that did something (`Numbers`, `Calls`, the files, the last
  answer), the quiet ones named at the end. `Service` reads through
  `chat.Service.Open` + `Turns()` (priced turns; a remote pane's through
  the Reader as the chat does, nothing new runs on a remote): `GET
  /api/recap?machine&pane[&session]` or `?machine&dir&session`
  [`&since`] is one conversation's, `GET /api/standup?tz[&since]` is
  every agent pane on every connected machine (`Standup`, the machines
  read side by side) with the radar's state and alert, since local
  midnight in `tz` unless `since` says, and the Markdown.
  `features/standup/`: `StandupPanel.tsx` is the panel (store panel
  `standup`; the sun button in the sidebar head beside Usage, the
  palette's "Standup"; a `Select` for since midnight / 24 h / 7 days,
  pref `standup.period`; the agents that need the person first, then
  the finished, then the working, the dearest first — `order` in
  `standup.ts`, tested with `numbers`, `calls`, `fileList`,
  `recapMarkdown`; a row opens the pane's chat; the quiet agents folded;
  Copy as Markdown through `copyText`), `RecapDialog.tsx` one
  conversation's (dialog kind `recap`: the Recap button in
  `chat/ChatTools.tsx` for the session shown, the palette for the pane
  and for a transcript open from the search), `RecapBody.tsx` the lines
  both draw.
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
  machine (`GET /api/search?q=&machine=&dir=&days=`; the 🔍 in the
  header, or the last row of the ⌘K palette with what was typed there). No index: `Scan` walks `~/.claude/projects` newest-first,
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
  `data-ids~=uuid` → scroll + `.turn-hit`, then `actions.clearJump`, so a
  later mount of the pane starts at the bottom in its live session), else opens the transcript on
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
  pending one until the next id; `PriceTurns` (turns.go) prices a
  transcript's turns the same way for the chat. **Live meter**: `Tracker` keeps a sum per
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
  composer offers them under ⚡ and edits them in a dialog; a text may
  carry `{dir}`, `{branch}` and `{file}` (`chat/quickvars.ts`, tested:
  `needs` / `expand`), filled on the pick by `expandPrompt` in
  QuickPrompts.tsx — the pane's folder, its branch through one
  `GET /api/review` only when the text wants it, and the file last opened
  in the viewer (store field `lastFile`, set by `openFile`); one the page
  cannot fill stays as written. The composer
  also dictates through the browser's own `SpeechRecognition` (nothing
  passes through remuda) and every assistant turn has **forward** — the
  turn's text, headed "From <workspace on machine>:", to another agent
  through `features/agents/SendToAgent.tsx`, the one dialog Tasks, Chat and the review share
  (`send.targets` pref remembers the last target per key). **Message several agents** (`features/agents/BroadcastDialog.tsx`, the
  palette and the ✉ in the sidebar head, dialog kind `broadcast`): tick the
  agents — All / Working / Resting pick by status, an agent asking a question
  is never picked — one text goes to each in turn through `/api/submit`, and
  the dialog says how each went; a refusal is shown, never retried or queued
  (`broadcast.ts` the pure part, tested).
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
  top-bar `mode-switch` (Agents | Wall | Tasks | Code | Review) is a per-device pref.
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
  `code.go`, `update.go`, `tasks.go`, `screen.go`, `question.go`, `terminal.go` with `termhttp.go` and
  `peer.go`), each registering its routes in a `…Routes(mux)` that
  `Handler()` calls; a handler reads the request, asks one service and
  writes the answer, and the logic lives in the context packages.
  `/api/health` and `/api/login` are mounted outside the token guard;
  everything else behind it. Every route resolves its machine through
  `machineFor` (query → `X-Remuda-Machine` header → POST body `machine`)
  and `ready()`. The three create routes share `create()` (agent kind
  validated by `sessions.KnownAgent` *before* any RPC, and a `resume`
  session id by `text.SessionID` with an agent to go with it; container
  made, agent started by `sessions.StartAgent`, or `StartAgentResumed`
  with `resume`, watcher refreshed); the two
  close routes share `closeHandler`, the two renames (`PATCH
  /api/tabs/{id}`, `PATCH /api/sessions/{id}`, body `{label}`) share
  `renameHandler` — herdr's `tab.rename` / `workspace.rename`.
- `internal/sessions/` — the input side of herdr: `Agents` / `KnownAgent`,
  `StartAgent` (retries `agent_pane_busy` for a few seconds), `StartAgentResumed` (the same, with the kind's resume arguments after a session id — `ResumeArgs`, the one table of them: `claude --resume`, `codex resume`, `gemini --resume`, `kimi --session`; the restarts use it too), `Keys`
  (herdr's `send_keys` grammar), `RefOK` (a branch name git gets as an
  argument), and `Input`: one lock per pane, `Submit` (`agent.prompt`, else
  `send_text` + Enter; `ErrAgentBusy` while a menu is open), `SendKeys`,
  `SubmitToFolder` (the scheduler's door).
- `internal/chat/` — the conversation behind a pane: `Service.Open` (which
  file the chat lens shows: the pane's live one followed through
  compactions, a session of its folder by id, or a transcript on its own
  by folder and id — a `NotFoundError` for an unknown one), `SessionFile`,
  `LastWrite` / `LastWriteOfPane`, `SessionVersion`, `LastTurn`,
  `LastToolCalls`, `LastAnswer`: the radar's recovery, the watchdog, the
  task watch and the said tracker read through it; `ParseQuestion`
  (question.go, pure, tested) reads an agent's dialog and its choices off
  a pane's visible screen for the chat's question card. Every answer carries `subagents`, the conversation's agents as they stand (`Conversation.Subagents`, listed by `Open`; its ETag moves with their files), and `subagent=<id>` answers one of them on its own (`Conversation.Subagent`: read-only, no sessions, nothing marked seen, an ETag of its file). `GET /api/chat?last=N` (1..200) answers the last N turns only
  — `Conversation.LastTurns`, the `Tail` (the slice, and the agent's
  plan over the whole file, `transcript.Plan`, which every answer carries
  as `plan` while it has items, so a tile sees a plan set thirty turns
  ago) kept per file version in
  `tailCache`, so the wall's tiles and the lens on the same pane parse a
  file once per change (the cached slice is a copy, so the cache never pins
  a whole parsed file); the ETag is the same either way, a tail lists no
  sessions, and every GET with a pane but a tail marks it seen. In the UI a tab is renamed in
  place by double-click, or from the workspace's ⋯ menu (touch). The roster carries an ETag (304 when unchanged) and
  the agent list. Local files stream through `http.ServeContent` (Range for
  video); the WS uses per-message compression and coalesces queued chunks.
- `web/src/styles.css` — one design system in **Catppuccin's colours,
  Frappé by night and Latte by day** (`lib/theme.ts` holds the swatches,
  tested): every token a `light-dark()` pair picked by the root's
  `color-scheme`, which `app/theme.ts` sets through `data-theme` from the
  pref `theme` (system / dark / light; `startTheme` in main.tsx before
  the first render, `useTheme` / `useThemePref` / `setThemePref`, the
  three "Theme: …" rows of the palette; the pop-out's document and
  `<meta name="theme-color">` follow); the terminal lens takes
  `terminalTheme` and the wall's screens `ansiPalette`, Catppuccin's own
  terminal mapping, so a tile shows the lens's colours. Four surface
  layers (`--bg` base → `--bg-raised` mantle → `--bg-input` → `--bg-overlay`,
  `--bg-active` / `--bg-elev` for a raised chip), hairlines from the
  surfaces (`--line`, `--line-strong`, `--line-hover`), peach as the
  accent with `--on-accent` ink, every colour with a `-soft` fill and a
  `-ring` mixed from it (`color-mix`), shadows and sheen as inks the
  theme picks, a radius scale (`--r-xs` 4 … `--r-xl` 16 — never a px radius),
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
  in and back out — the opener is read before the first render, since an
  autoFocus field has the focus by the time an effect runs — `#root` inert while open, `as="form"` + `onSubmit`,
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
  portal so nothing clips it). `Menu.tsx` is a menu behind any button
  and `Tip.tsx` a tooltip (`TipProvider` once over the page): Radix's
  DropdownMenu and Tooltip in the design system's clothes (`.menu`,
  `.menu-item` with an icon slot, a hint and a shortcut, `.tip`) — the
  keyboard walks a menu, Esc closes, both open where there is room, a tooltip never opens on a touch — every icon-only button of the
  header, the sidebar, the tab strip, the composer and a wall tile
  carries one, with an `aria-label` for the reader, and a `Menu` takes a
  `tip` for its trigger; `radix-ui` is the one dependency
  beside react and xterm, for the primitives whose keyboard and focus
  behaviour is not worth writing twice. `Resizer.tsx` (over `panels.ts`, tested:
  `PANELS`, `clampWidth`, `dragWidth`, `keyWidth`) is **a panel's
  draggable edge** — the sidebar of Agents and of Tasks, the files rail,
  the Review queue, a task's detail — a handle beside the panel in the
  same row that takes no room: the width is a pref per device
  (`panel.*`, clamped to the panel's range) and reaches the stylesheet
  as a CSS variable on the row's element (`--sidebar-w`, `--rail-w`,
  …), painted straight on the DOM while the pointer drags and written
  when it lets go, so a drag redraws nothing but the layout; ← → on the
  focused handle step it, ⇧ by more, ↵ and a double-click bring the
  usual width back; none under 920 px or on a coarse pointer, where the
  panels are drawers; "Reset the panels' widths" in the palette
  (`resetPanels`).
- `web/` — React + Vite + xterm.js SPA, embedded via `web/embed.go`
  (`go:embed all:dist`). Build with `make web` before `go build`; `npm
  test` runs Vitest on the pure modules (`*.test.ts` beside each; node,
  no DOM). Laid out by feature: `lib/` → `ui/` → `features/<name>/` (one
  folder per lens or panel, its server calls in its own `api.ts`) →
  `app/`; a dependency points down that list, and a feature never imports
  another feature's screens (its `api.ts` and pure modules, and the few
  shared pieces `agents/SendToAgent`, `files/target`, are fine; from
  `app/` a feature takes only `store`, `current`, `act`, `pip`, `theme`).
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
