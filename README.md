# remuda

*A remuda is the herd of spare horses on a cattle drive; each morning a
rider picks the one for the day.* This one is yours: every AI coding
agent on every computer you own, from your browser or your phone —
chat, live terminal, the changes they made, tasks, schedules, the
machines' vitals and updates. Self-hosted; every byte stays on hardware
you control. (Formerly herdr-ui; it runs on top of **herdr**, which it
installs, starts and updates for you.)

A from-scratch, auditable replacement for
[herdr-web-ui](https://github.com/devswha/herdr-web-ui):

- **One Go binary.** The UI is embedded; `go build` from this repo is the
  whole supply chain. No Bun, no npm at runtime, nothing phoning home.
- **herdr owns the agents.** remuda is only a bridge to herdr's local RPC
  socket — your workspaces, tabs, panes, worktrees and agent statuses are
  herdr's own, and nothing breaks if remuda restarts.
- **Every PC you SSH to.** Add a remote machine by its ssh host; remuda
  reaches its herdr over one OpenSSH ControlMaster (your keys, your
  ~/.ssh/config, your tunnels) and shows its workspaces, terminals and chat
  beside the local ones. No daemon to install on the remote.
- **Read-only on your data.** The chat view reads Claude Code's own session
  files under `~/.claude/projects/` (resolved through herdr's exact session
  id); remuda never writes them. Sending a message goes through herdr's
  `agent.prompt`, exactly like typing into the pane.
- **No telemetry, no third-party services.** The server talks to your
  browser, to herdr, and to the SSH hosts you add. That's the complete list.

## How it works

```
browser / phone ⇄ HTTP + WebSocket ⇄ remuda (Go) ─┬─ local herdr.sock (RPC)
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
- **The address follows you** — `?machine=&pane=&mode=&lens=` is what you
  are looking at, so a link, a reload or a notification lands on the same
  pane in the same lens.
- **Submit** — herdr's `agent.prompt` (bracketed paste + Enter), refused
  while the agent waits on a menu; falls back to `pane.send_text` for plain
  shells. Named keys (`Enter`, `Escape`, arrows, `Ctrl+C`) via
  `pane.send_keys`.
- **New workspace / close** — `workspace.create` + `agent.start` (any agent
  herdr has a manifest for), `workspace.close`.
- **Tasks** — the **Agents | Tasks** switch in the top bar opens remuda's
  own task list: projects on the left, Today / Upcoming / All open, quick
  add, done, due dates, priorities, labels, descriptions, search — and a
  **board** per project: the columns a task moves through (New → In
  progress → Testing → Done out of the box; yours to rename, recolour,
  reorder and add to, in the project's ⚙), cards dragged between them on
  a desk, swiped through a column at a time on a phone. Each project can
  carry its **own fields** (text, number, one choice, several, a date, a
  checkbox, a link), shown on the card and handed to the agent with the
  task. It lives in one SQLite file (`tasks.db`) in remuda's state dir —
  no account, no other server, nothing leaves the machine. Every task has
  **Send to agent** — the task's title, description and fields go to a
  running agent as a prompt (the arrow offers "for grooming", which
  prefixes `/grill-me`) — and **Start an agent on it**: a worktree
  branches off one of your workspaces, Claude starts there and gets the
  task once it is up (its "trust this folder" question for the new
  worktree is answered for you), the task moves to *In progress*, and
  every answer the agent finishes comes back to the task as a comment
  with a push — and moves it on to *Testing*, unless you moved it first.
  Which columns do what is the project's own setting. Mark it done from
  there.

## Install & run

On a Mac or a Linux box with nothing on it — no Go, no Node, no checkout:

```bash
curl -fsSL https://github.com/dev1antdev/remuda/releases/latest/download/install.sh | sh
```

The script ([`internal/setup/scripts/install.sh`](internal/setup/scripts/install.sh):
read it first if you like) downloads the release binary for the machine's
OS and CPU (macOS and Linux, arm64 and amd64), checks it against the
release's `checksums.txt` and refuses a download that does not match, puts
it at `~/.local/bin/remuda` and runs `remuda setup`: herdr installed if
missing, its headless server started, the status hooks for the agents it
finds, and remuda as a background service on `127.0.0.1:7317`. On a Mac
the page then opens in your browser, signed in; elsewhere the sign-in link
is printed. Nothing asks for sudo, and the same line run again is the
upgrade. What it cannot do for you is install a coding agent — Claude Code,
Codex or Gemini CLI — and sign you in to it: do that, then `remuda setup`
once more for that agent's hooks. (git, which worktrees and the Changes
lens use, comes with Apple's Command Line Tools: macOS offers them the
first time git is called.)

Arguments go to `remuda setup` (`… | sh -s -- -tailscale`);
`REMUDA_INSTALL_VERSION=v0.5.1` picks a release instead of the newest and
`REMUDA_INSTALL_ONLY=1` stops after the binary.

With the binary in hand, the pieces by name:

```bash
remuda setup                      # herdr installed if missing, its server
                                    # started, the agent hooks, this binary to
                                    # ~/.local/bin and the service running it
remuda setup -host studio.remote     # the same on a remote machine over ssh; one
                                    # without remuda fetches the release for
                                    # its own OS and CPU
remuda code [-host studio.remote]    # code-server, for the Code mode
remuda uninstall                  # the way back (below)
remuda -h                         # all of it, and the server's flags
```

### Uninstall

```bash
remuda uninstall          # remuda: the service, the binary, the log
remuda uninstall -all     # and your data, code-server and herdr: nothing left
```

It first lists what it found — what would go, and what stays unless you
say so — and asks before removing anything (`-yes` skips the question,
`-n` only shows the list, `-host` does it on a remote machine). remuda's
own things always go: the launchd / systemd service, `~/.local/bin/remuda`,
the log, the ssh connections it holds open. Three things are more than
remuda's own and go only by their flags:

- `-data` — the state folder: the token, your tasks, schedules, saved
  prompts, push keys, the tailnet node's state (the node itself stays
  listed in your Tailscale admin console until you remove it there).
- `-code` — code-server: its service, the release under `~/.local/opt`,
  and the editor's own settings and extensions.
- `-herdr` — herdr: its hooks in your agents' settings, its server —
  **which ends every agent and shell running in it** — the binary (through
  Homebrew when that is where it came from) and `~/.config/herdr`. A
  remuda you started by hand is stopped with it, since it would start
  herdr's server again; without `-herdr` it is only pointed out.

`-all` is the three together. What no command can reach is said at the
end: the page's sign-in and preferences in your browser, and the files
you attached in chats (each project's `.remuda/uploads`).

herdr is installed with Homebrew when there is one, else with herdr's own
installer into `~/.local/bin`. remuda also starts herdr's headless
server whenever none answers — at its own start, and on a remote machine
when it connects — so agents are there whether or not a terminal window
is. The server is started through your login shell with a clean
environment, so its panes see the PATH and exports your terminal has.
Updates — herdr, Claude Code, Codex, Gemini CLI, and remuda itself
on a remote machine — run from the page: the ⓘ beside a machine shows
what is out of date with an Update button, the ⋯ menu has "Updates…";
each shows the exact command first and streams its output. After an
agent update, the chats running that agent are restarted onto the new
version: each is asked to leave (its own `/exit`) and started again in the
same pane with its session resumed (`claude --resume`, `codex resume`,
`gemini --resume`); a busy chat is restarted a little after it finishes,
and a message half-typed in a chat's box is lost. The same restart is
offered on its own ("Restart Claude Code chats") for an agent that
updated itself. An update of herdr ends with its server on the new
version: herdr's own live handoff when the installer did one, otherwise
the server is restarted — herdr restores the workspaces and panes, and
remuda starts every chat again with its session resumed ("Restart
herdr's server" is offered whenever the server lags the binary).

remuda watches its own releases on GitHub too (one request every six
hours, none with `-no-version-check`). When a newer one is out, the ⓘ of
every machine behind it says so with a **Download & install** button, the
Updates dialog lists it, and a toast in the corner says it once. The
machine downloads the binary for its OS and CPU itself, checks it against
the release's `checksums.txt`, installs it as `~/.local/bin/remuda` and
restarts the service; the remuda serving the page restarts itself two
seconds after its install, and the page offers a reload. Pull requests land
on the `release` branch, which is built and tried as a whole; a release
fast-forwards it into `main` — so `main` is always what is out — and
`make publish` there pushes the tag, and the Release workflow
(`.github/workflows/release.yml`) builds the four binaries and
`checksums.txt` and publishes them under it. CI
(`.github/workflows/ci.yml`) runs the build, the tests, the TypeScript
check, `govulncheck`, `npm audit` and a secret scan of the whole history
on every push to `main` or `release` and every pull request.

The long way, piece by piece:

Requirements: Go 1.22+, Node 18+ (build only), herdr 0.9+ installed (its
server need not be running: remuda starts it).

```bash
make build        # builds web UI + go binary into bin/remuda
./bin/remuda      # listens on http://127.0.0.1:7317
```

Flags: `-addr`, `-socket` (herdr socket path), `-herdr-bin`.

### Remote machines

Every machine saved in herdr (`herdr machine list`) appears automatically —
add a PC in herdr and remuda picks it up within 30s, tagged *herdr*. **+ PC**
in the sidebar adds an extra one by any ssh destination your OpenSSH reaches
with keys. Either way remuda brings up one `ControlMaster` to it, autodetects
the remote herdr binary (incl. `~/.local/bin/herdr`) and socket, reaches its
RPC through a forwarded unix socket — or through `nc -U` over the same
connection when that sshd refuses socket forwarding — and runs
`herdr terminal attach` over the master for live terminals. Nothing is
installed on the remote (when no herdr server answers there, remuda starts
one, as the user); removing a machine tears the SSH connection down and
touches nothing there.

On first run remuda generates an access token (stored at
`~/Library/Application Support/remuda/token`, 0600) and prints a sign-in URL.
Every request needs the token — as the printed `?token=` link once per device
(it becomes an HttpOnly cookie), or `Authorization: Bearer`.

## As a background service on a Mac or Linux box

```bash
make deploy HOST=studio.remote     # the right binary → ~/.local/bin/remuda there
make service HOST=studio.remote    # launchd LaunchAgent (macOS) / systemd --user (Linux):
                                # 127.0.0.1:7317, starts at login, restarts if it exits
make service                    # the same on this machine
```

Logs: `~/Library/Logs/remuda.log` (macOS) or `journalctl --user -u remuda`.
`remuda uninstall` removes the service and the rest (see Uninstall above).

## Docker / Portainer

For a **Linux host** that runs herdr (the container shares the host's unix
socket; macOS VMs can't, run the binary there):

```bash
cp .env.example .env     # HOST_HOME, UID/GID, optional token and projects root
docker compose up -d     # http://127.0.0.1:7317
```

In Portainer: add the repository as a stack, set the same variables in the
stack environment. The image carries `remuda`, `herdr` (for `terminal
attach`), `ssh` and `git`; it mounts the host user's `~/.config` (herdr's
socket + remuda's state), `~/.claude` (transcripts), `~/.ssh` (remote
machines) and the projects root (file viewer), all at the same paths as on
the host, running as that user's uid.

```bash
docker compose --profile test run --rm test   # go vet + go test + tsc
docker compose --profile dev up dev           # go run on :7317, Vite HMR on :5173
docker build --target runtime -t remuda .   # just the image
```

## Code — VS Code in the browser

The **Code** mode frames [code-server](https://github.com/coder/code-server)
(VS Code served over HTTP, MIT) running on the machine you pick — the editor
for the workspace's folder, with its own terminal, git and extensions (from
Open VSX). Each machine runs its own copy, bound to `127.0.0.1:7320` with
no auth of its own; remuda is the door: `/code/<machine>/` goes through
the same bearer token, locally to the port, remotely through a port
forwarded over the ssh master the roster already uses. Install it on a
machine with

```sh
remuda code                      # this machine
remuda code -host studio.remote     # a remote one, over ssh
remuda code uninstall            # its service off again; status asks how it is
```

(the binary carries the script, `internal/setup/scripts/install-code.sh`:
it downloads a pinned release, verifies its checksum on all four
platforms, installs a launchd / systemd user service; `make code [HOST=]`
from a checkout is the same thing). The editor stays
mounted behind the other modes, so switching back does not reload it.

One thing to weigh before opening it on a machine: the editor runs inside
remuda's page, as this origin, so whatever runs in it — an extension, a
repository's own workspace settings — can reach remuda's API as you, on
every machine here. remuda keeps its own token from the editor (the
cookie is stripped on the way to code-server), but the editor itself is
as trusted as this page. Open the Code mode only on machines you trust
with everything remuda can do; see SECURITY.md.

## From your phone

The simplest way: let remuda join your tailnet by itself.

```bash
remuda -tailscale                      # or: REMUDA_TAILSCALE=1 make service
```

The binary becomes a node of its own (no `tailscaled`, no `tailscale
serve`), gets a MagicDNS name and a real certificate, and serves
`https://remuda.<your-tailnet>.ts.net`. The first start logs a login URL
(the desk's page shows it as a toast too); after that it reconnects on its
own. `-tailscale-hostname` names the node, `TS_AUTHKEY` skips the login
(an auth key from the Tailscale admin console). HTTPS certificates and
MagicDNS must be enabled in the tailnet's DNS settings. The loopback
listener stays for the desk.

Or keep remuda on loopback and bring your own transport:

```bash
tailscale serve --bg --https=443 http://127.0.0.1:7317   # your tailnet only
# or
ssh -L 7317:127.0.0.1:7317 your-mac                       # plain SSH tunnel
```

Either way, open the address in the phone browser, sign in with the token
once, then "Add to Home Screen" — it installs as a PWA.

**Never** expose the port directly to the internet.

## Security model

- Binds `127.0.0.1` by default; remote access is your tunnel's problem
  (Tailscale/SSH), not an auth system invented here. `-tailscale` is that
  tunnel built in: the node is only reachable from your tailnet, and the
  token is still asked for.
- Outbound: nothing, except (unless `-no-version-check`) one request to
  the npm registry and one to GitHub every six hours to learn the newest
  Claude Code and herdr, so a machine can say it wants an update.
- Single bearer token, constant-time compared, HttpOnly cookie, no accounts.
- Cross-origin mutations rejected (`Sec-Fetch-Site`).
- Anyone with the token can type into your terminals — treat it like an SSH
  key.

A second copy beside the real one (to try `-tailscale`, say) gets its own
token, tasks and push subscriptions with `REMUDA_STATE_DIR=/some/dir`.

## Development

```bash
make server    # go run, API on :7317
make dev       # vite dev server on :5173, proxies /api and /ws
make typecheck # tsc + go vet
make webtest   # vitest on the page's pure modules
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
uploads into the pane's folder (`.remuda/uploads/`) and mentions the file
as `@path` in the message — Claude reads the image from there. In the
terminal lens the paths are typed, shell-quoted. Works for remote machines
too (the file goes over your SSH session).

**Push alerts** (🔔 in the header): standard Web Push with remuda's own
VAPID keys — the only third party is your browser vendor's push service.
remuda watches every machine's agents itself, so alerts arrive with the
app closed: *needs you* 10 s after an agent blocks on a question, *finished*
a minute after a long turn. Answer at the desk first and nothing is sent.
Needs HTTPS (tailscale serve) or localhost; on iPhone, the home-screen app.

**Changes:** a third lens next to Chat and Terminal shows the workspace's
uncommitted work — what `git diff` shows, plus new files — with what moved
since you last looked marked, and the commits made since. A merge or a
pull brings nothing into the list; only the work in the folder does. Tap a
line of a diff and write what should change; a few of those make a review.
**Approve** tells the agent the work is reviewed and it can carry on (a
task it works on can move along), **Reject** tells it what to fix and to
come back for another look; both take a note and carry your comments, and
both start the next look from now. The comments can also go on their own,
to this agent or any other. "Mark reviewed" just clears the counts — the
agent hears nothing.

**Between agents:** every assistant turn has a *forward* link that sends its
text, headed "From *workspace*:", to any other agent on any machine — the
same dialog Tasks uses to send a task. **Quick prompts** (⚡ in the composer)
are saved texts, for every folder or one, shared by all your devices.
**Dictation** (🎤) uses the browser's own speech recognition — Safari on
iPhone, Chrome on the desk; nothing passes through remuda.

**Machines** carry the names herdr gave them; the ⋯ beside a machine
renames it for remuda (the Mac you sit at too), copies its SSH host, or
removes one you added here.

**A new workspace** names a machine and a folder; the folder is picked
from the machine's own folders (repositories marked, the workspaces
already there as quick picks) or typed.

**Several remuda at once:** each machine may run its own (the phone on
the studio Mac's, the desk on its own). herdr lets one client attach to a terminal
at a time, so when a remote machine's own remuda is running (and
reachable on its loopback port 7317), this one reaches that machine's
terminals through it instead of attaching directly — both sides can look
at the same pane, and nobody gets "already has an attached client". The
vitals probe finds it and reads its token over your SSH; without it the
direct attach stays.

**Machine vitals:** the ⓘ next to each machine shows which Claude Code,
herdr and remuda run there, disk and load; it turns amber when an update
is out (with the right command for how it was installed), a remote
remuda is behind this one, or the disk is nearly full.

**Cost and context:** every agent pane shows how full its context is (a
pill in the header and on the sidebar row from 50% up, red near the edge,
with "Compact now" in the chat from 80%) and what the session has cost so
far at Claude Code's own API prices. 📊 opens the report: by day, by
project, by model, by machine, the sessions that cost the most — every
machine reads its own transcripts, nothing is sent anywhere. On a
subscription it is a measure of where the work went, not a bill.

**Search** (🔍 or ⌘K): full text over every conversation on every
machine — what you or an agent said, never tool output — filtered by
machine, folder and period. A hit jumps to the turn in the running pane, or
opens the old transcript read-only. No index, no database: each machine
scans its own `~/.claude/projects` on demand (remuda's scanner where the
binary is deployed, grep otherwise).

Planned: a remote-machine session picker, incremental transcript parsing for
very long sessions.

## License

MIT
