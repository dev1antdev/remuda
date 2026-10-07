# herdr-ui

Self-hosted browser and phone client for **herdr**. Read and reply to the
agent sessions herdr runs on your computer — Claude Code and the rest — as
chat, with the live terminal a click away, from any of your devices. Every
byte stays on hardware you control.

A from-scratch, auditable replacement for
[herdr-web-ui](https://github.com/devswha/herdr-web-ui):

- **One Go binary.** The UI is embedded; `go build` from this repo is the
  whole supply chain. No Bun, no npm at runtime, nothing phoning home.
- **herdr owns the agents.** herdr-ui is only a bridge to herdr's local RPC
  socket — your workspaces, tabs, panes, worktrees and agent statuses are
  herdr's own, and nothing breaks if herdr-ui restarts.
- **Every PC you SSH to.** Add a remote machine by its ssh host; herdr-ui
  reaches its herdr over one OpenSSH ControlMaster (your keys, your
  ~/.ssh/config, your tunnels) and shows its workspaces, terminals and chat
  beside the local ones. No daemon to install on the remote.
- **Read-only on your data.** The chat view reads Claude Code's own session
  files under `~/.claude/projects/` (resolved through herdr's exact session
  id); herdr-ui never writes them. Sending a message goes through herdr's
  `agent.prompt`, exactly like typing into the pane.
- **No telemetry, no third-party services.** The server talks to your
  browser, to herdr, and to the SSH hosts you add. That's the complete list.

## How it works

```
browser / phone ⇄ HTTP + WebSocket ⇄ herdr-ui (Go) ─┬─ local herdr.sock (RPC)
                                        │          ├─ `herdr terminal attach` PTY
                                        │          └─ reads ~/.claude/projects/…
                                        │
                                        └─ per remote machine: one ssh ControlMaster
                                             ├─ -L forward of the remote herdr.sock (RPC)
                                             ├─ `ssh -tt host herdr terminal attach` (PTY)
                                             └─ `ssh host tail -c …` of the transcript
```

- **Radar sidebar** — one row per agent, read the way a person would (after
  [herdr-radar](https://github.com/hhdebb/herdr-radar)'s design, reimplemented
  here): a braille spinner while it works, a pulsing `?` while it waits on
  you, a green `✓` that **stays until you have looked at the pane**, a ring
  that cools with the time since the last turn — fresh, plain, then the
  whole row dims after two hours. Each agent wears its vendor's logo and
  hue (27 vendors), workspaces are headers with worktrees as a tree, the
  busiest project sorts first; `⇅` flips to a flat list by activity or
  herdr's own order, `⚙` holds the thresholds and the row label.
- **Roster** — workspaces, tabs and panes from `session.snapshot`, with
  herdr's own agent statuses: `RUN` (working), `NEEDS YOU` (blocked),
  `READY` (idle), `DONE`.
- **Terminal lens** — xterm.js fed by a shared `herdr terminal attach` PTY
  per pane (all viewers share one attach; replay buffer for late joiners).
- **Chat lens** — the transcript herdr names via `agent_session`, parsed
  into turns (text, thinking, tool calls with outputs), polled with ETags.
- **Submit** — herdr's `agent.prompt` (bracketed paste + Enter), refused
  while the agent waits on a menu; falls back to `pane.send_text` for plain
  shells. Named keys (`Enter`, `Escape`, arrows, `Ctrl+C`) via
  `pane.send_keys`.
- **New workspace / close** — `workspace.create` + `agent.start` (any agent
  herdr has a manifest for), `workspace.close`.
- **Tasks** — the **Agents | Tasks** switch in the top bar opens your own
  [Vikunja](https://vikunja.io) (2.x) in herdr-ui's clothes: projects and
  saved filters on the left, Today / Upcoming / All open, quick add, done,
  due dates, priorities, labels, descriptions, search — and "Open in
  Vikunja" for everything else. Connect once per herdr-ui with the Vikunja
  URL and an API token (Settings → API Tokens there; scopes `projects`,
  `projects_views_tasks`, `tasks`, `tasks_labels`, `labels`, `other: user`).
  Every task has **Send to agent** — the task's title, description and link
  go to a running agent as a prompt (the arrow offers "for grooming", which
  prefixes `/grill-me`). The token is stored on the herdr-ui host and never sent to a browser:
  the page talks to Vikunja only through herdr-ui's own port, so
  a phone and a port forward need nothing extra.

## Install & run

Requirements: Go 1.22+, Node 18+ (build only), herdr 0.9+ running.

```bash
make build        # builds web UI + go binary into bin/herdr-ui
./bin/herdr-ui      # listens on http://127.0.0.1:7317
```

Flags: `-addr`, `-socket` (herdr socket path), `-herdr-bin`.

### Remote machines

Every machine saved in herdr (`herdr machine list`) appears automatically —
add a PC in herdr and herdr-ui picks it up within 30s, tagged *herdr*. **+ PC**
in the sidebar adds an extra one by any ssh destination your OpenSSH reaches
with keys. Either way herdr-ui brings up one `ControlMaster` to it, autodetects
the remote herdr binary (incl. `~/.local/bin/herdr`) and socket, reaches its
RPC through a forwarded unix socket — or through `nc -U` over the same
connection when that sshd refuses socket forwarding — and runs
`herdr terminal attach` over the master for live terminals. Nothing is
installed on the remote; removing a machine tears the SSH connection down and
touches nothing there.

On first run herdr-ui generates an access token (stored at
`~/Library/Application Support/herdr-ui/token`, 0600) and prints a sign-in URL.
Every request needs the token — as the printed `?token=` link once per device
(it becomes an HttpOnly cookie), or `Authorization: Bearer`.

## As a background service on a Mac or Linux box

```bash
make deploy HOST=gog.remote     # the right binary → ~/.local/bin/herdr-ui there
make service HOST=gog.remote    # launchd LaunchAgent (macOS) / systemd --user (Linux):
                                # 127.0.0.1:7317, starts at login, restarts if it exits
make service                    # the same on this machine
```

Logs: `~/Library/Logs/herdr-ui.log` (macOS) or `journalctl --user -u herdr-ui`.
Remove with `deploy/install-service.sh uninstall`.

## Docker / Portainer

For a **Linux host** that runs herdr (the container shares the host's unix
socket; macOS VMs can't, run the binary there):

```bash
cp .env.example .env     # HOST_HOME, UID/GID, optional token and projects root
docker compose up -d     # http://127.0.0.1:7317
```

In Portainer: add the repository as a stack, set the same variables in the
stack environment. The image carries `herdr-ui`, `herdr` (for `terminal
attach`), `ssh` and `git`; it mounts the host user's `~/.config` (herdr's
socket + herdr-ui's state), `~/.claude` (transcripts), `~/.ssh` (remote
machines) and the projects root (file viewer), all at the same paths as on
the host, running as that user's uid.

```bash
docker compose --profile test run --rm test   # go vet + go test + tsc
docker compose --profile dev up dev           # go run on :7317, Vite HMR on :5173
docker build --target runtime -t herdr-ui .   # just the image
```

## Code — VS Code in the browser

The **Code** mode frames [code-server](https://github.com/coder/code-server)
(VS Code served over HTTP, MIT) running on the machine you pick — the editor
for the workspace's folder, with its own terminal, git and extensions (from
Open VSX). Each machine runs its own copy, bound to `127.0.0.1:7320` with
no auth of its own; herdr-ui is the door: `/code/<machine>/` goes through
the same bearer token, locally to the port, remotely through a port
forwarded over the ssh master the roster already uses. Install it on a
machine with

```sh
make code                   # this machine
make code HOST=gog.remote   # a remote one, over ssh
```

(`deploy/install-code.sh`: downloads a pinned release, verifies its
checksum, installs a launchd / systemd user service). The editor stays
mounted behind the other modes, so switching back does not reload it.

## From your phone

Keep herdr-ui on loopback and bring your own transport:

```bash
tailscale serve --bg --https=443 http://127.0.0.1:7317   # your tailnet only
# or
ssh -L 7317:127.0.0.1:7317 your-mac                       # plain SSH tunnel
```

Open the address in the phone browser, sign in with the token once, then
"Add to Home Screen" — it installs as a PWA.

**Never** expose the port directly to the internet.

## Security model

- Binds `127.0.0.1` by default; remote access is your tunnel's problem
  (Tailscale/SSH), not an auth system invented here.
- Single bearer token, constant-time compared, HttpOnly cookie, no accounts.
- Cross-origin mutations rejected (`Sec-Fetch-Site`).
- Anyone with the token can type into your terminals — treat it like an SSH
  key.

## Development

```bash
make server    # go run, API on :7317
make dev       # vite dev server on :5173, proxies /api and /ws
make typecheck # tsc + go vet
```

## Status / roadmap

Working today (verified against herdr 0.9.1, protocol 22, local + a real
remote machine): multi-machine roster with herdr's agent statuses, live
terminal (shared attach, replay, input, resize, touch key bar) on local **and
remote** panes, chat (exact session via `agent_session`, remote transcripts
fetched by delta over SSH, thinking/tool folding, submit via `agent.prompt`,
approval nudge), workspace create/close with `agent.start`, add/remove remote
machines, token auth, PWA shell.

Also: new tab / new worktree / close from a workspace's ⋯ menu, tabs in a
strip above the pane, machines mirrored from `herdr machine list`, and a
**file viewer** — click any path the agent prints in the terminal or chat to
open the file where it lives (local or remote), with a **Changes** tab
showing its uncommitted `git diff`.

**Attachments:** the paperclip, a pasted screenshot (Ctrl/Cmd+V) or a drop
uploads into the pane's folder (`.herdr-ui/uploads/`) and mentions the file
as `@path` in the message — Claude reads the image from there. In the
terminal lens the paths are typed, shell-quoted. Works for remote machines
too (the file goes over your SSH session).

**Push alerts** (🔔 in the header): standard Web Push with herdr-ui's own
VAPID keys — the only third party is your browser vendor's push service.
herdr-ui watches every machine's agents itself, so alerts arrive with the
app closed: *needs you* 10 s after an agent blocks on a question, *finished*
a minute after a long turn. Answer at the desk first and nothing is sent.
Needs HTTPS (tailscale serve) or localhost; on iPhone, the home-screen app.

**Between agents:** every assistant turn has a *forward* link that sends its
text, headed "From *workspace*:", to any other agent on any machine — the
same dialog Tasks uses to send a task. **Quick prompts** (⚡ in the composer)
are saved texts, for every folder or one, shared by all your devices.
**Dictation** (🎤) uses the browser's own speech recognition — Safari on
iPhone, Chrome on the desk; nothing passes through herdr-ui.

**Search** (🔍 or ⌘K): full text over every conversation on every
machine — what you or an agent said, never tool output — filtered by
machine, folder and period. A hit jumps to the turn in the running pane, or
opens the old transcript read-only. No index, no database: each machine
scans its own `~/.claude/projects` on demand (herdr-ui's scanner where the
binary is deployed, grep otherwise).

Planned: a remote-machine session picker, incremental transcript parsing for
very long sessions.

## License

MIT
