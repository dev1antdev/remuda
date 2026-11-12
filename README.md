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

- **Radar sidebar** — every repository a folder, laid out after
  [Orca](https://onorca.dev)'s: its icon, its name and how its agents
  stand; a click folds it shut or opens it, and the sidebar's ⋯ folds or
  opens them all. A folder's own ⋯ starts a new workspace in it — in a
  repository a worktree of its own, a new folder and branch, so its agent
  never touches another workspace's files (a second agent on the same
  checkout is a tab) — sets its icon or opens it in the editor.
  Open, a folder holds a card per workspace — the main
  checkout and its worktrees — a click away from its agent. The agents read
  after [herdr-radar](https://github.com/hhdebb/herdr-radar)'s design,
  reimplemented here: a braille spinner while it works, a pulsing `?`
  while it waits on you, a green `✓` that **stays until you have looked at
  the pane**, a ring that cools with the time since the last turn — fresh,
  plain, then the whole card dims after two hours — and how long it has
  been so ("12m", "3h"). Under the name, the branch the folder has checked
  out and what the agent is doing, or, once it finished, the first line of
  what it answered, so the sidebar reads like a list of results. A
  workspace running several agents lists them in small lines under its
  card, each a click to open. Each agent wears its vendor's logo and hue
  (27 vendors), the busiest folder sorts first; the sidebar's ⋯ flips to a
  flat list by activity or herdr's own order, its settings hold the
  thresholds and the label. Above the pane each tab says what it runs —
  the agent by its logo and name, a shell as "terminal" — and + opens a
  terminal in the workspace's folder at once, or an agent.
- **Folder icons** — click a folder's icon (or "Folder icon…" in a
  workspace's ⋯ menu, or the palette) to give it a face: first the ones
  the project already has — its favicon, logo or app icon, found in the
  checkout on its own machine, and its owner's avatar on GitHub (or on
  GitLab, through `glab` when the machine has it) — then any emoji by
  group or by its English name, a site's icon — type `gog.com`, remuda
  reads its favicon once, right then, and keeps a copy; nothing is fetched
  later — or a picture from your computer, a company's logo made into a
  small square. Without one a folder wears its initial on a tile in a
  colour of its name. The icon is the server's, so every device and the
  Wall, the palette and the header show it.
- **Screen-sharing mode** — sharing your screen at work and the sidebar
  names a client's projects? Alt+H (or "Screen-sharing mode" in the View
  menu or the palette) turns it on: the folders and machines you marked
  with "Hide when sharing the screen" in their ⋯ menus — or everything,
  the mode's own pick when nothing is marked — read as "Project A",
  "Machine 2", their icons a plain tile, their branches gone, the agents'
  titles and answers hidden; a hidden folder's chat, terminal and files
  are covered until you click "Show this pane", its wall tiles show their
  state only, and the palette, the toasts, Activity and the Mac app's
  menu say the neutral names too. An amber "Sharing" chip in the header
  says it is on and holds its menu. It lives on this device only; nothing
  the server keeps or sends changes, so a notification on your phone still
  names the folder.
- **Jira** — set your site in Settings › Jira (Jira Cloud: the site, your
  account's e-mail and an API token from id.atlassian.com; a server: its
  address and a personal access token) and the issue keys a workspace's
  branch or its agents name — `feature/CRMOPS-6279-…`, "fixed
  CRMOPS-6279" — show as a chip on its sidebar card and wall tile,
  "CRMOPS-6279 · In Review" in the status's colour, a click opening the
  issue. "New task from a Jira issue…" (the Tasks head's link menu, the
  palette) makes a task of one, its description and labels with it. The
  token stays on the server and is never shown back; remuda asks the site
  only while a page is open, and nothing until you set it.
- **Roster** — workspaces, tabs and panes from `session.snapshot`, with
  herdr's own agent statuses: `RUN` (working), `NEEDS YOU` (blocked),
  `READY` (idle), `DONE`.
- **Terminal lens** — xterm.js fed by a shared `herdr terminal attach` PTY
  per pane (all viewers share one attach; replay buffer for late joiners).
- **Chat lens** — the transcript herdr names via `agent_session`, parsed
  into turns (text, thinking, tool calls with outputs), polled with ETags.
  Claude Code's session files, Codex's rollouts and Gemini CLI's chats
  all read into the same turns, so a Codex or Gemini pane has its chat,
  recap, standup line, meter and search like a Claude Code one (Gemini's
  reader is written from the CLI's own types and has not been seen live).
- **One socket for the news** — the page keeps one WebSocket to its
  remuda, which says when the roster moved or a transcript grew, so the
  sidebar, the wall and the chat redraw within about a second instead of
  on the next poll, and the polls themselves slow down to a safety net:
  fewer requests, less battery on a phone. Without the socket (an old
  proxy, a hiccup) everything polls as before. remuda itself hears herdr
  the same way: one event stream per machine, so a question or a finish
  is on the page, the Wall and your phone about a quarter of a second
  after the agent stopped, and herdr is asked for its state every 15–20 s
  instead of every 2–4 while the stream stands.
- **The address follows you** — `?machine=&pane=&mode=&lens=` is what you
  are looking at, so a link, a reload or a notification lands on the same
  pane in the same lens.
- **Submit** — herdr's `agent.prompt` (bracketed paste + Enter), refused
  while the agent waits on a menu; falls back to `pane.send_text` for plain
  shells. Named keys (`Enter`, `Escape`, arrows, `Ctrl+C`) via
  `pane.send_keys`.
- **Answer** — while the agent waits on a dialog, the chat shows the
  question as it stands on the screen (Claude Code's permission prompts,
  its "trust this folder", an AskUserQuestion) with a button per choice;
  a tap walks the cursor there and presses Enter, Esc closes. From the
  phone, the wall and the pop-out too. A dialog remuda cannot read leaves
  the blind ↑ ↓ Enter Esc. Beside the composer's tools a small chip says
  Claude Code's permission mode as its footer does — accept edits, plan,
  auto, manual, bypass in amber — and a tap cycles it with shift+tab, the
  key a phone has no way to send.
- **New workspace / close** — `workspace.create` + `agent.start` (any agent
  herdr has a manifest for, with the model, extra arguments and first
  prompt you chose under Options), `workspace.close`.
- **The rail** — the places of the page down the left edge, each an
  icon with its name: Agents, Wall, Tasks, Code, Review, then Standup,
  Usage, Schedules, Activity and Settings. On a phone it lies along the
  bottom under the thumb, with Code, the panels and Settings behind
  More. The sidebar's head keeps two buttons: + (a workspace, a PC) and
  ⋯ (message several agents, the order, the settings).
- **Catppuccin, all four flavours** — Latte by day and Frappé, Macchiato
  or Mocha by night, following the system by default; "Theme: …" and
  "Dark flavour: …" in the palette (⌘K) or the Look section of Settings
  pin one on this device, and the accent — peach unless you say — is
  any of Catppuccin's fourteen colours, a swatch each in Settings. The
  terminal, the Wall's screens and the Code editor wear the matching
  palette.
- **English or Russian** — the page speaks the language of your browser,
  or the one you pick in Settings › Look › Language ("Язык: Русский" in
  the palette); a change reloads the page. The agents' answers, the
  pushes, the standup and the MCP answers stay as they are.
- **Text size, density and font** — the Look section of Settings also
  sets the page's text size (Small to Larger, "Text size: …" in the
  palette too; the terminal keeps its own size in its View menu), the
  density (Compact tightens the sidebar rows, the tabs, the tile heads
  and the gaps between turns, on a wide screen with a mouse) and the
  monospace font for code and the terminal, from the families installed
  on this device — each shows in its own face in the list, an
  uninstalled one falls back to the system's monospace. Nothing is
  downloaded: no web fonts.
- **Panels** — the sidebar, the files rail, the Review queue and a
  task's detail each have a draggable edge: pull it to the width you
  like and it stays that way on this device; Tab to the edge and use
  ← → (⇧ for bigger steps), ↵ or a double-click for the usual width;
  "Reset the panels' widths" in the palette puts them all back.
- **A floating terminal** — Ctrl+` (from a terminal too), Shell on the
  rail or "Floating terminal" in the palette opens a shell over whatever
  is on screen, on the machine of the pane you are on (a Select switches
  machines), without changing that pane. It is a shell of its own, made
  once in the machine's home folder and kept out of the sidebar; Ctrl+`
  or × hides it and the shell keeps running, so it comes back as you
  left it; drag its top edge for its height, or End the shell for a
  fresh one next time.
- **Settings that follow you** — "Sync these settings between devices",
  the last row of Settings › Look, keeps the theme, its flavour and
  accent, the density, the code font, the sidebar's settings, the Wall's,
  the review's two columns, the files rail, the timeline, the tasks view,
  the standup period, Code's Follow and the panels' widths in this remuda,
  so the phone and the desk agree: a change on one device reaches the
  others within a minute (at once when they come back into view), the
  newer choice winning. Alerts, quiet hours, read-aloud, the text sizes
  (the page's and the terminal's) and the prompt history stay on the
  device they were set on.
- **Wall** — the **Wall** mode in the top bar puts every agent on one
  screen: a tile per agent pane on every connected machine, with its
  state and how long it has lasted, what it runs this second (the tool
  call under way, else thinking or writing) and how far its plan is
  (3/7) or, once it has finished,
  the first line of what it answered, its pull request as a link when
  it named one (GitHub or GitLab; the workspace's ⋯ menu has it too)
  and how it stands beside the link — "2/2 checks · approved", red when
  a check failed, the branch conflicts or the review asks for changes,
  amber while checks run or a merge request's threads are open ("2 open
  threads") — read with `gh` (`glab` for a GitLab merge request) on
  that machine as you, every two minutes while the page is open, so
  remuda itself asks GitHub nothing. The chips open a menu: when a check
  failed, *Hand the failed checks to the agent* reads the failed steps'
  log with `gh run view --log-failed` on that machine and sends its end
  to the agent as a message, the way you would paste it; *Send the
  review comments to the agent* does the same with the reviewers'
  verdicts and their comments on the lines, numbered as the Changes
  lens numbers yours. Nothing is sent unless you choose the row, and an
  agent waiting on a question refuses, as the chat does;
  the last turns of its conversation and a line to answer it,
  in a grid that fits the window (S / M / L). Hide the idle ones, or
  keep only the agents that need you — a question, a watchdog note, a
  finish you have not looked at, each ringed on the wall — hide any
  tile, bring them back; **focus** one agent and it takes the whole wall
  while the others wait in a strip under it; a click on a tile's head opens that agent
  in Agents. **Office** turns the wall into a show for a second screen or
  a TV: one agent large — the one asking you something, else whoever
  needs you, else each in turn every 20 s — the rest as cards in a column
  beside it, in full screen; a card's click or the pin on any tile keeps
  that agent featured, ← and → walk them by hand, Esc leaves. The tiles ask only for the last few turns, so a wall of a
  dozen agents stays light. **Screen** turns every tile into the pane's
  terminal as it is — the rows with their colours, scaled to the tile,
  the prompt and the status at the bottom — a wall of monitors, read
  from herdr without attaching, so nothing is resized and no attach slot
  is taken; a click opens that terminal. **Zoo** turns every agent into a
  little creature in its vendor's colour, for the TV across the room: it
  bounces at a laptop while it works, raises a hand under a "?" when it
  asks you something (and holds up what it asks), holds up an amber "!"
  for a watchdog note, dances over a finish you have not looked at,
  sleeps with a "z" when idle (dim when long idle, under a blanket at
  night), slumps under an empty battery when it hit its plan limit, and
  tilts its head when its state is unknown; press and hold one (or its
  speaker under the pointer) and it tells you its day in its own voice —
  "Today I answered 3 times and edited app.go. Now I'm waiting for your
  answer." — high for Gemini, low for the Codex robot — the pose and the sign say it
  even without colour. Each vendor is a species of its own — a round
  critter with antennae for Claude Code, a robot for Codex, an egg
  between sparkles for Gemini CLI, a moon rabbit for Kimi Code — and
  wears its model's hat (a top hat for Opus, a wizard's for Fable, a
  beret for Sonnet, a sprout for Haiku, a cap for OpenAI's models) and a
  trinket of its own, so two agents of one kind tell apart; it grows
  rounder as its context fills (a belt from 80 %, a puff and slim again
  after a compaction), piles coins for what it cost (amber or red when a
  budget runs short), flies its pull request's flag in its checks'
  colour and wears a crown once it is merged, brings out coffee after
  20 minutes of work and sweats after an hour, and its subagents at
  work hop around it as babies. The creatures stand in a pen per
  machine whose weather is the machine's body — sun and sweat when it
  runs hot, a storm for under-voltage, boxes for a nearly full disk —
  under the sky of your hour, and the day runs along the foot: answers,
  waits, spend, merges, who worked longest. When an agent finishes the
  resting ones clap, a merged pull request sets off fireworks, the eyes
  follow your pointer, and with the voices switched on (off by default)
  each chirps in its species' voice. A click opens it, a hover or a long
  press tells its state in words, office mode and the pop-out show it
  large, and reduced motion keeps every creature still in its pose. In Chrome a tile — or the agent on screen,
  from the header — **pops out** into a small window that stays above
  every other app (Document Picture-in-Picture): the agent keeps you
  company over your editor, and the line to answer it is right there.
- **Review** — the **Review** mode lists every workspace on every machine
  with uncommitted work, the ones with changes since you last looked
  first, and opens the Changes lens of the one you pick beside the list:
  approve, reject or comment from one place instead of visiting each
  pane.
- **Arena** — one prompt to several agents at once: "Arena…" in a task's
  Start dialog, or "Arena: one prompt to several agents…" in the palette.
  Pick two to six lanes — an agent and its model on a repository of any
  machine — and each starts in a fresh worktree of its own, so their work
  never meets. The Arenas panel shows them side by side: how each stands,
  what it answered, +ins −del · files, how long it ran and what it cost.
  "Keep this one" leaves the winner as an ordinary workspace; every other
  lane's work — commits and uncommitted changes — is saved as a patch in
  remuda's state folder first, then its agent stops and its worktree and
  branch go.
- **Models scorecard** — which model actually does your work best, from
  remuda's own records: the Usage panel's Models tab (or "Models
  scorecard" in the palette) has a row per agent and model over the last
  7, 30 or 90 days — how many of its task runs finished, how many were
  approved at your first review in the Changes lens rather than rejected
  first, how many of the pull requests they opened merged, how many of its
  arena lanes you kept, and what a run, and an approved one, cost and how
  long a run took; a run whose agent has not answered yet counts in those
  figures once it has. A share needs five runs behind it before it becomes a
  percentage (before that it says "2 of 3"), and only such figures compete
  for the quiet green mark of a column's best. The button at the end of a
  row opens an arena on a recent task with two models side by side: the
  one that ran it and the row's own.
- **Standup** — the sun button in the sidebar head (or "Standup" in the
  palette) is one report over every agent on every machine: what each
  did since midnight — prompts and answers, how long and what it cost,
  the tools it called and which failed, the longest call, the commands
  it ran, the files it edited and read, subagents started, questions
  asked — and the first line of its last answer, the agents that need
  you first. Under each agent a thin **day strip** shows *when*: green
  while it worked, red while it waited on a question of its own, dim
  while it sat idle, the hours marked under it; hover a stretch for its
  times, click one to open the chat right there. Switch to the last 24
  hours or 7 days; **Copy as Markdown**
  gives the report ready to paste into a channel. Come back to the page
  after an hour or more and a toast says what happened meanwhile —
  "Away 3 h: 2 agents answered, 1 waits for you, $4.20 spent" — with the
  standup one click away; nothing when nothing happened. The same recap of one
  conversation — the session on screen, or a transcript opened from the
  search — is the sun button beside the chat's export, and in the
  palette. All of it is read from the transcripts remuda already has;
  nothing is sent anywhere. **Listen** (the headphones beside the period,
  or "Listen to the day" in the palette) reads the day aloud on the way
  to work, the way a colleague would tell it: "Good morning. Here is
  your day so far. 3 agents did some work. Together they spent about
  9 dollars at API prices. In web on GOG, Codex is waiting for your
  answer…" — who waits for you first, who finished what and what it
  cost, its last answer in its own words, the reviews waiting and a plan
  limit near its end, then what is left; a few minutes at most, in the
  page's language. Play, pause, back, skip to the next agent; the
  sentence being said is on screen and the agent marked in the report,
  and a phone's lock screen shows it with its buttons (Chrome on
  Android goes on with the screen locked; Safari on an iPhone stops
  when the screen locks, so keep it on). It is the browser's own voice,
  the one picked under Read aloud, and nothing passes through remuda;
  a recap has Listen too. **A standup digest every morning**: in
  Schedules, pick "Standup digest" instead of a prompt, a time ("Weekdays"
  at 9:00) and what it covers — since midnight, the last 24 hours, the
  last 7 days, or since the previous digest — and every device that gets
  remuda's alerts receives one push: how many agents finished, how many
  wait for you, what it cost, a line per agent; a tap opens the Standup.
  Quiet hours hold it, and a period with nothing in it sends nothing.
  **A scout every few hours**: the third kind of schedule, "Start an
  agent", opens a fresh tab in a folder, starts the agent you name (with
  a model and arguments if you like, or a saved recipe that fills the
  four fields at once) and hands it your prompt once it is
  up — "look through this project for what could be better and create a
  task for each" is the one the form suggests; with "Close the tab when
  the agent has answered" on, the tab goes by itself once the agent is
  done, a push says what it answered, and the conversation stays
  readable through the search. A question the agent asks is yours:
  nothing closes while it waits. **A scout that runs at 3 am does not
  stand at a permission prompt until morning**: with "Don't wait for
  permissions: deny and carry on" (on for a new schedule, Claude Code
  only) the agent is started with Claude Code's own
  `permissions.denyInsteadOfPrompting` for that run — any call that
  would wait for your approval is denied at once and the turn goes on —
  and once it has answered the schedule's last result says how many
  calls were denied ("answered: … · 2 calls denied") and names them, so
  does the push, and the chat lens of its tab lists them over the
  composer with *Open in the terminal*; a run Claude Code stopped after a
  string of refusals, before it answered, says it stopped. In Claude Code's default mode
  every edit asks first, so a scout that should change files wants
  `--permission-mode acceptEdits` (or `auto`) in its extra arguments, or
  allow rules in its settings; a `--settings` you pass yourself as JSON
  gets the setting merged in, one naming a file is refused with what to
  put in it. Schedules saved before the switch keep asking, as they did,
  until you turn it on. For the scout to create tasks, its
  agent needs remuda's tasks MCP set up (see Tasks below) — and, while it
  denies instead of asking, those tools allowed: Claude Code asks before
  it uses an MCP tool no rule allows, so put `mcp__remuda__*` in
  `permissions.allow` in its settings (or in a `--settings` JSON of its
  extra arguments), or its tasks are denied with the rest. **Every few
  minutes, in working hours**: the "Every N minutes" preset runs a
  schedule every 5, 10, 15, 20 or 30 minutes from one hour to another,
  on weekdays only if you like — every 10 minutes from 9:00 until 17:00,
  Monday to Friday, is `*/10 9-16 * * 1-5` — and under the When row the
  form reads any cron back in words with its next three runs ("Mon–Fri,
  every 10 min, 9:00–16:50 · next: 14:10, 14:20, 14:30"), naming the
  host's clock when it is another zone than your device's. Nothing piles
  up: a run is skipped, noted in quiet words and pushed nowhere, while the
  agent it would reach — or the one the schedule's last run started — is
  still working or waits for your answer.
- **Tasks** — the **Agents | Wall | Tasks | Code | Review** switch in the top bar opens remuda's
  own task list: projects on the left, Today / Upcoming / All open /
  Proposed, quick add, done, due dates, priorities, labels, descriptions,
  search — and a **board** per project: the columns a task moves through
  (Proposed → New → In progress → Testing → Done out of the box; yours to
  rename, recolour, reorder and add to, in the project's ⚙), cards
  dragged between them on a desk, swiped through a column at a time on a
  phone. **Proposed** is where what an agent files lands (see "Agents and
  your tasks" below), apart from your own New column: the Proposed list
  shows every proposal with **Accept** (it goes to New) and **Decline**
  (it is deleted), one by one or all at once. Which column takes the
  proposals is one of the four marks in the project's ⚙; a project from
  before gets a Proposed column once, and stays without one if you remove
  it. Select several — ⌘-click, ⇧-click a range, a long press on a phone
  — and the bar under the list moves, labels, finishes or deletes them at
  once, or starts an agent on each of them through one dialog: a worktree
  per task, one after another, the first new pane opened. Each project can
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
  A project's settings say **where its agents work** — a machine and one
  of its repositories, the agent, its model and arguments (a recipe fills
  them) — and the Start dialog begins from there. Switch on **the queue**
  and mark a column *Queued*: the cards you put there start by themselves,
  top first, as many at once as you allow, once the machine is there, the
  agent's plan window has room and no budget over the folder is crossed;
  the column's head says what the next card waits for, and a start that
  was refused waits until you move the card again. A **pipeline** adds
  stages after the first agent's work — say Codex running the tests, then
  Claude opening the pull request: each agent ends its answer with
  `STAGE DONE`, the next starts in a new tab of the same worktree with the
  previous answer in its prompt and the card in its column; an answer
  without the line (a question in the agent's own words too) or a card you
  move stops it there, while a permission prompt only waits for you —
  unless the project's agents **don't wait for permissions** (on for
  agents you set up now, Claude Code only, off for settings saved
  before): then the queue's Claude Code and every stage's deny what
  would wait for your approval and carry on, and a comment by remuda on
  the task lists the calls each run was denied; a run that stops on
  them is reported so and stops the pipeline, the card left where it
  is. An agent you start on a task yourself asks as usual.
  Agents can propose tasks, but only you queue them.
  When the agent opens a pull request and names it (the Review mode's
  *Approve & open PR* asks for exactly that), the task notes the link, moves
  on the same way, and shows the request with its checks on the card and in
  the task while the agent's pane is still there. Once that request is
  merged on GitHub — checked with `gh` every two minutes while the run
  is live, page or no page — the task moves to *Done* with the comment
  "PR #15 merged" and a push, unless you moved it elsewhere first. A
  GitLab merge request (`MR !613`, gitlab.com or your company's own
  instance) is followed the same way through `glab` on that machine: its
  pipeline's jobs as the checks, its approvals, its threads still open,
  the failed job's log and the open threads handed to the agent from the
  same menu, and the merge moving the task to *Done*. Once merged, the
  workspace's ⋯ menu offers *Remove the worktree and its branch…*,
  which closes the workspace and runs `git worktree remove` and `git
  branch -D` on the machine, refusing a worktree with uncommitted
  changes before anything closes.
  Closing any workspace that stands on a worktree herdr or remuda made
  (under `~/.herdr/worktrees`) takes the worktree with it: the folder
  goes when nothing in it is uncommitted, the branch once it is merged,
  and a toast says what stayed and why. *Leftover worktrees…* in a
  machine's ⋯ menu finds the ones left from workspaces closed elsewhere
  and removes the clean ones.
  A *Review* link beside the run opens its worktree in the Review mode.
  The card says what the agents started on it have spent so far
  ("$4.20", in the quiet colour), and the run in the task says the time
  it took and the model — read off the same meter as the header's pill,
  a measure at API prices, and kept once the agent's pane is gone.
  Which columns do what is the project's own setting. Mark it done from
  there. A task can also start as a **GitHub issue**: paste its address
  (the link button in the Tasks head, or "New task from a GitHub
  issue…" in the palette), and `gh` on the machine you pick reads it as
  you — title, text and labels become a task whose first line names the
  issue, so an agent started on it knows where it came from; **Create &
  start an agent…** opens the agent dialog on it at once. Nothing is
  written to GitHub. A task an agent made (through its key, see the
  agents' tools) wears a small robot badge and says which machine and
  pane it came from; an agent writes its description in Markdown
  (headings, lists with checkboxes, code, links), rendered as it would
  be anywhere else.

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
finds (and brought up to date when a newer herdr writes newer ones), and
remuda as a background service on `127.0.0.1:7317`. On a Mac
the page then opens in your browser, signed in; elsewhere the sign-in link
is printed. Nothing asks for sudo, and the same line run again is the
upgrade. What it cannot do for you is install a coding agent — Claude Code,
Codex, Gemini CLI or Kimi Code — and sign you in to it: do that, then `remuda setup`
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
Updates — herdr, Claude Code, Codex, Gemini CLI, Kimi Code, herdr's own
agent hooks and its plugins from GitHub, and remuda itself on a remote
machine — run from the page: the ⓘ beside a machine shows what is out of
date with an Update button, the ⋯ menu has "Updates…"; each shows the
exact command first and streams its output. A newer herdr writes newer
hooks into your agents' settings: the ⓘ says whose are outdated, an
update of herdr installs them again by itself, and "Update herdr plugins"
is `herdr plugin update` for the plugins installed from GitHub (herdr
0.9.4 and up). After an
agent update, the chats running that agent are restarted onto the new
version: Claude Code is told its own `/restart` and comes back on the
installed version with the conversation kept, in place; the others — and
a Claude Code that does not have the command yet — are asked to leave
(their own `/exit`) and started again in the same pane with the session
resumed (`claude --resume`, `codex resume`, `gemini --resume`, `kimi
--session`); a busy chat is restarted a little after it finishes, and a
message half-typed in a chat's box is lost. The same restart is
offered on its own ("Restart Claude Code chats") for an agent that
updated itself, and for one chat at a time from the workspace's ⋯ menu
or the palette ("Restart Claude Code on its session…"): for an agent
that got stuck, the same restart in its pane, refused while it has a
question open or is still working. An update of herdr ends with its server on the new
version: herdr's own live handoff when the installer did one, otherwise
the server is restarted — herdr restores the workspaces and panes, and
remuda starts every chat again with its session resumed ("Restart
herdr's server" is offered whenever the server lags the binary).

**After a reboot the chats come back too.** herdr restores the
workspaces and panes, but every pane is a bare shell again. remuda
remembers which agent and which session each pane ran, so when a
machine's herdr comes back with those panes empty, the machine's ⓘ
turns amber — "3 chats stopped with the reboot: Claude Code in api,
Codex in web, …" — with a **Resume** button, and a toast in the corner
says the same once. Resume starts each chat again in its own pane on
its session (`claude --resume`, `codex resume`, …), one job whose log
sits under the machine's recent jobs; a pane whose folder or session
file is gone, or that runs something else by then, is skipped with the
reason. Nothing starts until you press the button, and a chat you left
yourself is never offered back.

**A tab closed by mistake comes back.** When a tab or a workspace with
an agent in it closes — from remuda or anywhere else — remuda keeps the
agent's session for a while (the last 20 per machine). Right after a
close made here a toast offers **Reopen**; later, **Alt+Z** or "Reopen a
closed agent…" in the palette lists them, newest first, and one click
starts the agent again on its own conversation, in a tab of its
folder's workspace or a new workspace when that one is gone.

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
on every push to `main` or `release` and every pull request, and on
`release` the end-to-end smoke as well: a throwaway herdr, a remuda of
its own and headless Chrome, a scratch workspace typed into and read
back, then every screen drawn at a desk's width and a phone's in English
and Russian and checked for its place on the screen — nothing past an
edge, nothing sideways, no console error (`make e2e` runs the same thing
on your machine, touching nothing of yours).

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
add a PC in herdr and remuda picks it up within 30s. **+ PC**
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
Every request needs the token — as the printed `?token=` link once per device,
or `Authorization: Bearer`. A browser that signs in becomes a **device**: its
HttpOnly cookie carries a secret of its own, never the token, and Settings ›
Devices lists every one ("Chrome on macOS", "Safari on iPhone", renameable,
with when it was last seen and from where). Lost a phone? Revoke that device
there, or sign out every other device at once; the rest keep working and the
token stays what it was.

**Agents and your tasks.** The agents on every machine get a key of their
own that opens this remuda's task list, and a read of what the other agents
are doing, nothing else: remuda places it,
with the address to reach it at, in the machine's own state dir (on a remote,
over the same SSH connection, with this remuda's port forwarded back to the
machine's loopback), and `remuda mcp` / `remuda tasks` there read the two
files. Settings › Devices lists the keys, one per machine, with a Revoke each.

**Viewer links** sign a screen in to look only. Make one in Settings ›
Devices under a name — "the office TV", a colleague's — and open it where
the viewing happens: that browser gets the Wall, the sidebar, the chats,
the screens and their timelapses, the standup, the usage and Activity, and nothing that types or
changes — no composer, no keys, no question buttons, no Changes lens, no
tasks; the terminal lens shows the pane's screen as the Wall does, without
attaching, and the file viewer opens only files inside the machines'
workspace folders. Every call past that is refused with "this link is for
viewing only", a viewer's look marks nothing as seen, and the header says
*Viewing only*. The link is shown once; revoking it signs every screen it
opened out at once, and the pairing link opened on such a screen makes it
a device of your own.

## As a background service on a Mac or Linux box

```bash
make deploy HOST=studio.remote     # the right binary → ~/.local/bin/remuda there
make service HOST=studio.remote    # launchd LaunchAgent (macOS) / systemd --user (Linux):
                                # 127.0.0.1:7317, starts at login, restarts if it exits
make service                    # the same on this machine
```

Logs: `~/Library/Logs/remuda.log` (macOS) or `journalctl --user -u remuda`.
`remuda uninstall` removes the service and the rest (see Uninstall above).

### Backups of the cockpit

Everything the cockpit keeps lives in its state dir: your tasks, the
schedules, budgets, recipes, webhooks and settings, the signed-in devices,
the agents' keys, the token, the push keys and its Tailscale node. On a
Raspberry Pi that is an SD card. **Settings › Backups** copies it every
night (at a time you pick, in the cockpit's own zone) to another machine
of your fleet, into that machine's remuda folder under
`backups/<cockpit>/`, the newest 14 kept by default; *Back up now* takes
one at once, and a failed run sends a notification. The databases are
copied through SQLite itself, so a copy is consistent while the cockpit
runs. From a shell:

```bash
remuda backup now                  # through the running service
remuda backup list                 # the copies on the target
remuda restore remuda-20261105T033000Z.tar.gz   # the service stopped first
```

`restore` refuses while a remuda serves the state dir, moves the dir as it
is aside (`…/remuda.before-restore-<time>`) rather than deleting it, and
keeps the backups that machine holds for others. Restored on a new
machine, the cockpit keeps its Tailscale address, so every device stays
signed in — never run the old one and the restored one at the same time.
A backup holds the token: it is written 0600 into a 0700 folder on your
own machine, and nowhere else. It holds remuda's state dir alone: the
machine's own `~/.ssh` — its key and the hosts it reaches — is not in
it, so a cockpit restored on a new machine needs that machine's key let
in on your other machines again. A server (`-server`, below) keeps no
copies on its own disk; pick one of its machines. The agents' timelapses
(below) stay out of the copies, and a restore on the same machine keeps
them.

### A server that only serves the page

A small box that is always on — a Raspberry Pi — can hold the cockpit
for every browser and phone while the agents run on your other
machines. Run it as a server:

```bash
remuda setup -host pi.remote -server -tailscale   # no herdr there; the unit runs remuda -server
```

`-server` (or `REMUDA_SERVER=1`) keeps the machine remuda runs on out of
the cockpit: it is not in the sidebar, the Wall or any picker, no
workspace starts there, and its herdr is neither needed nor kept. Its
health stands behind **Server** on the rail instead — load, disk,
memory, temperature, the CPU's clock and a Raspberry Pi's throttling
(under-voltage, capped clock; the full firmware word where `vcgencmd`
is readable, the under-voltage alarm alone where it needs root) with the
day's sparklines — and the icon turns amber or red when the box is
warm, throttled or full. Add the machines the agents run on with the +
beside Machines, or `herdr machine add` on the server.

A box that already runs remuda keeps its binary through setup, so update
it first — *Download & install* in its Updates — to a release that knows
`-server`; run `remuda setup … -server` from that release too. Setup
refuses a remuda older than the flag and leaves the running service as
it is. A herdr already installed there stays, unused; its Updates offer
remuda's own release alone.

Every machine's ⓘ shows the same body now (memory, temperature, clock,
throttling), and **Update everywhere** in Updates runs every connected
machine's pending updates one after another — herdr, the agents,
code-server, remuda's release — each command shown first, the cockpit
itself last.

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
for the workspace's folder, or for **every workspace on the machine at
once** (the picker's first row, and the default past one folder: remuda
writes them into one `.code-workspace` on the machine and keeps it in
step as workspaces come and go) — with its own terminal, git and
extensions (from Open VSX). Each machine runs its own copy, bound to `127.0.0.1:7320` with
no auth of its own; remuda is the door: `/code/<machine>/` goes through
the same bearer token, locally to the port, remotely through a port
forwarded over the ssh master the roster already uses. A file from the
files rail, from a row of the Changes lens or from the file viewer (at
its line) opens here — in the editor already on screen when there is
one, through code-server's own command line, so nothing reloads. The
Code bar's **Follow** opens every file the agent on screen finishes
editing, as it happens; **Reload the editor** gives the folder a fresh
frame when the connection was lost. The editor wears the page's own
flavour — Catppuccin Latte, Frappé, Macchiato or Mocha, following the
switch — through the Catppuccin extension remuda installs; and when a
machine's code-server
is older than the release remuda carries, the ⓘ says so and Updates
offers the upgrade. Install it on a
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

## Previews — what the agents serve

An agent on GOG starts a dev server on `:3000`; **Previews…** in the
workspace's ⋯ menu (or the ⌘K palette) lists the ports the machine's
processes listen on — the workspace's own first (`3000 · node · vite`), the
other workspaces' after, the rest of the machine's folded away — and
**Open** shows one in a tab of its own. With `-tailscale` the port opens on
the cockpit's own tailnet name, the same number when it is free:
`https://remuda.<tailnet>.ts.net:3000`. A port of its own keeps the app's
absolute paths working, its live reload too, and keeps the app's scripts
off remuda's origin. Without `-tailscale` it opens on a port of the host
remuda listens on (`http://127.0.0.1:41000`…), which a browser elsewhere
reaches through a forward of its own. Only the browser signed in to remuda
gets in — a viewer link does not — and remuda's cookie never reaches the
app. Nothing is listened on by itself: a preview opens at your Open and
closes after half an hour unused.

When a pane announces a dev server itself — Vite's `Local: http://localhost:5173/`,
Next's, `python3 -m http.server`'s, uvicorn's, a plain `listening on :3000` —
the corner says so once ("web serves on :5173 · Preview") as soon as the port
really listens; Preview opens it the same way. Nothing opens by itself.

## Triggers — what a pane prints

herdr itself watches what every pane prints and tells remuda the moment a
line matches. Built in: the dev servers above, and **tests that failed** —
go test, jest, vitest, pytest, cargo, mocha, rspec — one push per pane, never
while its agent is still at work on them or while you have its terminal open,
then not again for half an hour. **Triggers…** in a workspace's ⋯ menu (or the
⌘K palette) adds your own: a line to look for — text as written or a regular
expression — everywhere, on one machine or in one folder, and what it does: a
push, a row in the activity, a preview offered, or the task whose agent runs
in that pane moved to a column (`ready for QA` → Testing). Test tries a rule
on a pane's last 500 rows before you save it. A regular expression is read the
way herdr reads it — `\w`, `\d` and `\b` know every alphabet, Cyrillic too —
and one herdr would refuse or read otherwise (`\<`, `\12`, a bare `{`) is
refused when you save it, with what to write instead. A muted workspace's
triggers stay quiet, though a task still moves. The webhooks hear them as
"tests that failed" and "a trigger of yours".

## Timelapse — forty minutes of an agent's work in thirty seconds

While an agent works, the cockpit reads its screen every 4 seconds — the
way the Wall's screen view does, without attaching, so the pane is never
resized and no terminal slot is taken — on every machine, whether or not a
page is open, and keeps a frame only when the screen moved. When the turn
ends (the agent at rest, asking you something, its pane closed, its
machine out of reach for two minutes — a laptop asleep) the
recording is written once, gzipped, under the cockpit's state dir in
`timelapse/<machine>/`; a turn still going is written again every ten
minutes, so a power cut loses no more than that. **Timelapse…** in the
View menu of the chat and the terminal (or *Timelapse of this pane…* in the
⌘K palette) plays it back: 10×, 30× or 60× the turn's own time, or a frame
at a time, a scrub bar over the turn, ← → a frame either way, Space to
pause, the pane's recordings and those of the earlier panes in its folder
to pick from — and **Export as GIF**, drawn in the page in your theme's
terminal colours. A turn under a minute is not kept, a recording holds at
most 900 frames and 8 MB of screens while it is recorded (a longer turn,
or one whose screen scrolls, is thinned evenly), each pane keeps its 20
newest and the folder 200 MB, the oldest going first. The recordings are
not in the nightly backups. **Settings › Timelapse** turns recording off
and says how much is kept, with *Delete all*; the player deletes one. A
viewer link plays them; under the screen-sharing mode a hidden folder's
are not shown. A second remuda started beside the real one with
`REMUDA_STATE_DIR` records nothing unless it is given `-record`.

## Agents and your tasks

Any coding agent remuda runs — Claude Code, Codex, Gemini CLI, Kimi Code —
can read and write your task list through **MCP**: `remuda mcp` is a Model
Context Protocol server over stdio with ten tools — `tasks_list`,
`task_get`, `task_create` (a Markdown description, a column, a due date, a
priority, labels, the project's fields), `task_update` (move it to another
column, mark it done, retitle, rewrite), `task_comment`, `projects_list`,
`labels_list`, and three that only read what your agents are doing:
`agents_list` (every agent on every machine, its state and for how long,
its last answer, its pull request, its cost; the agent's own pane marked),
`agent_recap` (what one conversation came to, as the Recap dialog shows it)
and `standup` (the report over all of them) — and it tells the agent when
to use which: look who is already on what before filing or taking a task.
A task an agent makes remembers the pane that made it and lands in the project's
**Proposed** column (unless the agent named a column), where the Tasks
mode's Proposed list lets you accept or decline it — so a scout's dozen
never mixes with your own New tasks. One command registers the server with
every agent on a machine, through the agent's own CLI (`claude mcp add`,
`codex mcp add`, `gemini mcp add`; Kimi when its CLI can):

    remuda mcp install            # on this machine
    remuda mcp install -host gog  # on a remote, over ssh
    remuda mcp status

or the row **Connect the agents to your tasks** under a machine's ⓘ →
Updates. An agent already running sees the tools once it is restarted. From
a shell, `remuda tasks list | show | add | move | comment` is the same thing
(`-json` for scripts). With that, a schedule such as *"every three hours:
look through this project for what could be improved and create a task for
each, with a description"* turns an agent into a scout that fills the board
while you are away.

How the agents get in: the remuda that holds the list leaves an `agent-key`
and an `agent-url` in remuda's state dir on every machine its agents run on;
the key opens the tasks routes and a read of the roster, a recap and the
standup, nothing else, on the cockpit itself or
through its door forwarded over ssh (`REMUDA_AGENT_URL` and
`REMUDA_AGENT_KEY` override them, for a script of your own). An agent whose
CLI cannot register a server gets the lines by hand — Claude Code's
`.mcp.json`:

    { "mcpServers": { "remuda": { "command": "/Users/you/.local/bin/remuda", "args": ["mcp"] } } }

Codex's `~/.codex/config.toml`:

    [mcp_servers.remuda]
    command = "/Users/you/.local/bin/remuda"
    args = ["mcp"]

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
- Single bearer token, constant-time compared, no accounts. A signed-in
  browser holds a device cookie (a random secret, hashed on disk), never the
  token: a device is revoked in Settings without touching the others.
- An agent's key (hashed on disk too) reaches the task routes and nothing
  else; it is refused at once anywhere else. One per machine, revocable in
  Settings; a revoked one is replaced when remuda next connects there.
- Cross-origin mutations rejected (`Sec-Fetch-Site`).
- Anyone with the token can type into your terminals — treat it like an SSH
  key.

A second copy beside the real one (to try `-tailscale`, say) gets its own
token, tasks and push subscriptions with `REMUDA_STATE_DIR=/some/dir`, and
places no agent door of its own, here or on your other machines, so the
agents keep talking to the real one (`-door` opens it anyway, `-no-door`
closes it for any instance). It also writes nothing into herdr — no
figures in its sidebar, no toasts, so the two never fight over the same
panes (`-mirror` writes anyway) — and records no timelapses of your
screens (`-record` records anyway).

## Development

```bash
make server    # go run, API on :7317
make dev       # vite dev server on :5173, proxies /api and /ws
make typecheck # tsc + go vet
make webtest   # vitest on the page's pure modules
make e2e       # the end-to-end smoke and the display checks: throwaway herdr + remuda + headless Chrome
```

When the page stalls — nothing clickable for a moment — the browser
console gets a line like `[remuda] The page stalled 4.2 s on the Wall (13
tiles)` with the mode, the lens and the pane it was on, and a stall of
two seconds or more shows a toast saying so; paste the line into an
issue and the cause is half found.

## Status / roadmap

Working today (verified against herdr 0.9.1, protocol 22, local + a real
remote machine): multi-machine roster with herdr's agent statuses, live
terminal (shared attach, replay, input, resize, find, touch key bar) on local **and
remote** panes, chat (exact session via `agent_session`, remote transcripts
fetched by delta over SSH, thinking/tool folding, submit via `agent.prompt`,
approval nudge), workspace create/close with `agent.start`, add/remove remote
machines, token auth, PWA shell.

Also: new tab / new worktree / close from a workspace's ⋯ menu, tabs in a
strip above the pane, machines mirrored from `herdr machine list`, and a
**file viewer** — click any path the agent prints in the terminal or chat to
open the file where it lives (local or remote), its code coloured in its
language, with a **Changes** tab showing its uncommitted `git diff`, coloured
the same way. A path is a link when it names a file
(`main.go`, `docs/plan.md`, `.env`), not a folder. A picture the agent
read or wrote — a screenshot it looked at, an icon it drew — shows as a
thumbnail under the call in the chat; a click opens it. **Subagents**: an
agent Claude Code spawns for a task (a fork, an Explore, a custom one)
shows under the call that started it — working, quiet or done, what it
is on this second or the first line of its report, how many tool calls —
and *Open* reads its own conversation; a tile's state line says "waiting
on 2 subagents" or "2 subagents working" while they run.

**Files the agent touched:** a rail beside the chat and the terminal (View ›
Files the agent touched) lists every file the agent edited or read
in this conversation, from its own tool calls — newest first, with how
many edits, when it last touched the file and a mark when that last edit
failed. A click opens the file with its uncommitted diff; the code button
opens it in the Code mode, VS Code on that machine. Edits an agent made
through shell commands are not its tool calls: the Changes lens shows
those. Hidden on a phone.

**Full screen** (View › Full screen, or the palette): the header,
the sidebar and the rail go and the lens takes the whole screen — a
terminal on the whole monitor, a chat on the whole phone. The tab strip
stays with its tools; Esc outside the terminal, or the same button,
brings the page back.

**Chat beside the terminal** (View › Chat beside the terminal, or the
palette): on a screen at least 1180 px wide with a mouse, the chat sits
on the left and the terminal on the right with a draggable edge between
them, so the conversation and the screen are read together. The lens
switch, or a click in either side, picks which one the tools, the find
and the keys serve; Changes still takes the whole area.

**Attachments:** the paperclip, a pasted screenshot (Ctrl/Cmd+V) or a drop
uploads into the pane's folder (`.remuda/uploads/`) and mentions the file
as `@path` in the message — Claude reads the image from there. In the
terminal lens the paths are typed, shell-quoted. Works for remote machines
too (the file goes over your SSH session).

**Push alerts** (🔔 in the header): standard Web Push with remuda's own
VAPID keys — the only third party is your browser vendor's push service.
remuda watches every machine's agents itself, so alerts arrive with the
app closed: *needs you* 10 s after an agent blocks on a question, *finished*
after every turn or only the long ones, as you choose. Answer at the desk
first and nothing is sent. While the page is in front the same alerts show
in its corner, with Open and an optional chime, so you see them whatever
the system's notification settings say. The alerts panel says what this
browser is and where it stands: notifications allowed or blocked (and
where to allow them, in the browser and in macOS or Windows), whether it
is subscribed to this cockpit, the last push sent and the last one
received; *Send a test* shows both. The Mac app tells you in its window
when the remuda it points at does not answer, and opens Settings for a
new address.
**A question is answered from the notification itself**: its push names
what is asked ("Bash command · rm -rf build") and carries the first two
choices as buttons in the dialog's own words — *Yes*, *No, and tell
Claude…* — so a permission is granted from the lock screen; before the
keys go, remuda reads the screen again, and a dialog that changed or
closed meanwhile is left alone and the page opens on the pane instead.
A tapped notification opens the pane in its terminal; the alerts panel
can make that the chat, or the lens the page was on, and the Mac app's
menu, an Activity row and Alt+N follow the same choice.
**Quiet hours** keep a device silent between two times of its own clock
(22:00 to 07:00, say); the Activity panel still keeps what happened, so the
morning starts with the list. Needs HTTPS (tailscale serve) or localhost;
on iPhone, the home-screen app. **A machine that drops off** gets a word
too: "GOG is away" with the reason once a remote has been unreachable
for two minutes, "GOG is back" with how long it was gone when it answers
again; a blip says nothing, and Activity keeps both.

**The Mac app** (`make mac` → `bin/remuda.app`, see `macos/README.md`)
is the same page in a window of its own, with what a browser tab cannot
be: a Dock icon with the Needs-me count as its badge, a menu bar item
that says who needs you — the dot in the colour of the most pressing
agent, the list with the first line of what each said, a click opening
its chat — native notifications with the phone's buttons (Open, Send
Esc, the question's own two choices), and ⌥⌘R to show or hide the
window from anywhere. It signs in by itself on the Mac that runs remuda
and is one more device of the alerts above, so quiet hours and the alert
settings apply to it; a remuda elsewhere is paired by its link in
Settings…. Not in it: the pop-out and dictation, both browser APIs.

**Read aloud** is the same panel's last switch: the browser's own voice
says what an agent answered when it finishes ("api: Tests pass."), that
one is waiting for you, or a watchdog note — while the page is open and
in front, in the voice you pick, a Russian one for a Russian answer.
Nothing passes through remuda; a reload says nothing. The same voice
reads the Standup aloud and gives the zoo's creatures theirs.

**Webhooks** are the same panel's last section, for a phone without Web
Push or a channel the team reads: a Telegram bot (its token and your chat
id), a Slack incoming webhook, an ntfy topic, or any address that takes
JSON. Each one hears the kinds you tick — a question, a finish, a
watchdog note, the digest, a schedule, a task, a budget, a review, a
machine away or back, a backup that failed, a sign-in running out — with
the title, the body and a link back to the pane, built from the address
you set at the top of the section (your tailnet's is offered when there
is one). A Test button on each sends one; the row says when the last one
was delivered or why it failed. Quiet hours apply per webhook. The
secrets stay on the server in `webhooks.json`: the page sees a masked
address and whether a token is set, and nothing is sent anywhere you did
not name here.

**Mute a workspace** when one folder keeps pinging while you work on
another: the workspace's ⋯ menu offers *Mute for an hour* and *Mute until
tomorrow 09:00*. Until then its panes send no push, no webhook and add no
Activity row — a bell-off on the sidebar row and the Wall tile says so,
with the time left in its tooltip — and the menu row reads *Unmute · 42
min left*. The mute ends by itself; the page's badge and read-aloud go on
as before, since they read the roster rather than the alerts.

**Inside herdr** (Settings): the terminal at the desk says what the phone
says. remuda writes what it knows of each agent into herdr itself, as
display tokens on its pane — `$remuda_cost` ("$4.20"), `$remuda_context`
("62%"), `$remuda_model`, `$remuda_pr` ("PR #15 ✓"), `$remuda_task`
("#42"), `$remuda_alert` ("! stuck 25 min", "! plan limit") and
`$remuda_said`, the first line of its last answer — and on each workspace
`$remuda_review` ("3 files to review") and `$remuda_cost`. The watchdog's
note also replaces the word in herdr's `state_text`. This is on by
default; the values are written only when they move and again before
herdr's five-minute TTL runs out, so they vanish by themselves once
remuda stops, and turning the switch off clears them at once. herdr shows
a token only where a sidebar row of its `config.toml` names it: the
section shows rows to paste (a row with nothing to say disappears) — in
place of a `[ui.sidebar.agents]` or `[ui.sidebar.spaces]` your config
already has, since herdr ignores a config with a table written twice —
then `herdr config check` and `herdr server reload-config`. A second switch, off by default, also pops
remuda's alerts — a question with its words, a finish after a turn over
a minute, a watchdog note — as herdr's own toast on the machine the
agent runs on, never over the tab herdr has in front, never for a muted
workspace and not inside the quiet hours you set there. One switch for
every machine; herdr shows a toast only where its config turns its
toasts on (`delivery = "herdr"` under its `[ui.toast]`), and *Send a test toast*
says, machine by machine, whether it did. A machine that runs its own
remuda beside the cockpit gets one writer, not two: the cockpit, which
knows your tasks, writes into that herdr and pops its toasts, and the
machine's own stays out there while the cockpit does — its Settings
section says so.

**Changes:** a third lens next to Chat and Terminal shows the workspace's
uncommitted work — what `git diff` shows, plus new files — with what moved
since you last looked marked, and the commits made since. A merge or a
pull brings nothing into the list; only the work in the folder does. Tap a
line of a diff and write what should change; a few of those make a review.
A diff shows in one column or two (the old file on the left, the new on
the right) with the words that changed in a line marked, the code
coloured in the file's language, and the lens
walks by keys: `j` `k` through the files, `↑` `↓` the lines, `n` `p` the
hunks, `c` comments on the line, `o` opens the file there, `s` flips the
columns. A hunk shows three lines around a change; the `↑` on its `@@`
row unfolds ten more from the file, "all N lines" the whole stretch up to
the previous hunk, and the last hunk unfolds downward to the end of the
file the same way — the unfolded lines walk and take comments like any
other.
**Approve** tells the agent the work is reviewed and it can carry on (a
task it works on can move along) — pick *and ask the agent to commit* and
it is **Approve & commit**: the agent commits the reviewed work and says
so; pick *and ask it to commit, push and open a pull request* and it is
**Approve & open PR**: the agent commits, pushes, opens the pull request
with `gh` and names its link, which a task it works on picks up (remuda
itself never writes into a repository); **Reject** tells it
what to fix and to end the answer that brings the work back with the line
`READY FOR REVIEW` — when it does, the queue lists that workspace first,
marked *ready again*, and a push says so; both take a note and carry
your comments, and both start the next look from now. The comments can also go on their own,
to this agent or any other. "Mark reviewed" just clears the counts — the
agent hears nothing.
**No secret leaves through a commit you asked for.** Pick the commit or
the pull request and remuda first reads exactly what the commit would
carry — every changed line, staged or not, and every new file git does not
ignore; for a pull request also the commits not pushed yet — for private
keys, AWS, GitHub, GitLab, Slack, Stripe, Anthropic, OpenAI, Google,
Telegram, npm, Hugging Face, Groq, xAI, OpenRouter, SendGrid, Vault,
DigitalOcean, Docker Hub, PyPI, Shopify, Linear, Atlassian and age keys
and tokens, JWTs, Bearer and Basic credentials, passwords in addresses
such as `postgres://app:…@db`, and long random values given to a name
like `API_KEY` or `password`, leaving placeholders such as `<your-key>`,
`changeme` or `xxxx` alone. Anything
it finds holds the button back: the dialog lists each one by file and
line with the secret masked, a click shows its row in the diff, marked,
and **Send anyway…** goes only after a confirm that says how many. A check
that cannot run says so, and Send anyway is the way past it too; a plain
Approve, Reject and Mark reviewed never wait. A line ending in
`gitleaks:allow` (or `remuda:allow-secret`) is let through, so a test
fixture can say it is one. The lens head also warns when Claude Code's
`.claude/settings.local.json` — where its "don't ask again" approvals
go, tokens in the commands included — is tracked by git or not ignored,
or when either settings file holds what looks like a secret, and says
what to do. remuda writes nothing into the repository or `.claude` for
any of it.
**Second opinion** asks another agent — Codex for work Claude Code did,
Claude Code for the rest, or whichever you pick — to review the same
changes in a new tab of the workspace, told to change nothing and to
answer one finding a line. Its comments come back under the lines they
name, blue beside your own, with how much each matters; *Add to my
review* takes one into your draft, *Add all* every one, *Dismiss* sets it
aside, and the ones on lines the diff no longer shows are listed apart.
Its tab closes once it has answered, unless you say otherwise; a Claude
Code reviewer denies whatever would wait for your permission and goes on
to its answer (a review only reads), unless you switch that off in the
dialog. Should it stop on what it was denied before answering, the block
says so and its tab stays, the denied calls listed there.

**Activity** (the bell in the header) is what happened while you were
away: every finish, question, watchdog note, failed schedule, task
move and workspace ready for another look, by day, newest first, from every machine — the same things the
pushes carry, kept whether or not a device is subscribed. A row opens
its pane; the bell wears a dot until you look. Chips at the top narrow
the list to a kind — questions, finishes, watchdog notes, tasks,
schedules, budgets, review, machines, what was sent — and a select to
one machine, each chip saying how many it holds; the bell's tooltip
says when a filter is on. The alert settings sit at the panel's foot. Everything typed into an agent through remuda is a
quiet row there as well — "iPhone sent «yes» to api · w3:p1" — so you
can see afterwards what went where from which device; those rows light
no dot.

**Code is coloured** in the chat's fenced blocks and in the Changes lens's
diffs — Go, TypeScript, Python, shell, JSON, YAML, TOML, CSS, HTML, Rust,
SQL, Makefiles, Dockerfiles — in Catppuccin's own hues for the flavour you
wear, by a small highlighter of remuda's own, no library behind it.

**Diagrams are drawn:** a ```` ```mermaid ```` block an agent writes — a
flowchart, a sequence diagram, a state machine — shows as the diagram in
the chat and in a task's comments, in your flavour's colours, with its
source a click away (Source); the file viewer draws a Markdown file and a
`.mmd` diagram too (Preview, beside File and Changes). Mermaid is loaded
only when a diagram is on screen; a block it cannot read stays code with
the reason; a wall tile keeps it as code.

**Between agents:** every assistant turn has a *forward* link that sends its
text, headed "From *workspace*:", to any other agent on any machine — the
same dialog Tasks uses to send a task. **Message several agents** (✉ in
the sidebar head, or the palette) sends one text to the agents you tick —
everyone, the working ones, the resting ones, or by hand — each through
the chat's own path in turn, and says how each went; an agent busy with a
question refuses and is shown so, never queued. **Resume** an older conversation of a workspace: the chat lens's session picker lists them, and an older one picked there gets *Resume in a new tab* — a new tab in the same workspace with the agent started on that conversation (`claude --resume`, and the same for Codex, Gemini and Kimi). **Continue with another agent** (View menu, or the palette) hands the conversation to a different agent — Codex's take, a cheaper model, a fresh Claude when this one is stuck: a new tab in the same workspace, the agent you pick started there with the recap of the conversation and its last exchanges in full as its first prompt (how many is yours to say; the whole stays under 60 KB), the old pane left as it is, the handover noted in the activity. **Quick prompts** (⚡ in the composer) are saved texts, for every folder or one, shared by all your devices; `{dir}`, `{branch}` and `{file, the handover noted in the activity. **Fork** a conversation (the View menu, the workspace's ⋯ menu, or the palette): Claude Code or Codex goes on from a copy of it in a new tab — the same history, a second path to try — while this one goes on as it is (`claude --resume … --fork-session`, `codex fork`). **Quick prompts** (⚡ in the composer) are saved texts, the handover noted in the activity. **Move the work to another machine** (the workspace's ⋯ menu, or the palette) carries what an agent is doing to a clone of the same repository on another of your machines — close the laptop, carry on on the desktop: the branch and its commits (as a git bundle through remuda, nothing pushed), the staged and unstaged changes as they were and the new files land in a fresh worktree there, and the agent you pick starts in it with where the work came from and the gist of the conversation; every step shows as it runs, and the work and the agent here stay until you close them. **Quick prompts** (⚡ in the composer) are saved texts}` in one become the pane's folder, its branch and the file you last opened. **↑**
on an empty line brings back what you sent before from this device to
that folder, newest first; ↓ walks forward and back to the line you were
writing.
**Quote**: select a piece of an agent's answer and a *Quote* button
appears beside it (or press Q) — the piece goes into your reply as a
Markdown quote, after whatever you had typed, so you answer one point of a
long answer without retyping it.
**The queue**: an idea for the agent while it is still working? Alt+↵ (or
the inbox button) keeps the text as a note above the box instead of
sending it. Each note has its own *Send*, *Send all as one message* sends
them numbered, a click on one puts it back in the box, × drops it. Nothing
in the queue ever goes by itself; the notes stay on this device, per pane.
**Export** (View › Export as Markdown on the chat lens, or the palette) saves
the conversation as a Markdown file — the text as it is, thinking and
tool calls folded — to share with someone who is not in remuda.
**A table's cells copy themselves**: hover a cell (or press it for a
moment on a phone) and a copy button appears in its corner, taking the
cell's words as they read — the sure way, since the browser's own table
selection turns into whole rows the moment a drag leaves the cell.
**Find** (⌘F / Ctrl+F on the chat lens, or 🔍 in the tab strip) marks
every place a word occurs in the conversation shown and says "3 of 12";
Enter and Shift+Enter walk them, the current one brought into view, Esc
closes. It works on a transcript opened from the search too, and on the
terminal lens over the pane's scrollback — the last five thousand rows
herdr keeps, whatever this browser has seen of them — with ⌘F there as
well on a Mac and Ctrl+Shift+F elsewhere, since Ctrl+F belongs to the
shell: the pane scrolls to each match (for every viewer, as the wheel
does), the match is marked on the screen, Esc gives the terminal the
focus back, and on a phone the key bar has a search key.
**The timeline** (View › Timeline, on by default with a mouse) is a
thin bar over the conversation: every prompt and its answer laid out in
time — a break over ten minutes drawn as ten, so a lunch does not
flatten the day — with the moments worth a look marked and named under
it: the first failed call, the longest call, a loop of identical calls,
the questions the agent asked, a compaction. A click on any of them
jumps to that turn. It works on a transcript opened from the search too.
**Plan** — when the agent keeps a task list (Claude Code's own
TaskCreate / TaskUpdate, or TodoWrite), a quiet line over the chat says
how far it is and what it is on, in the agent's own words — "Plan 3/7 ·
Building the page" — and unfolds to the list; a wall tile says 3/7 in
its state line, and in the conversation each change to the list is one
line, not a block of JSON. Read from the transcript, so a remote
machine's agent has it too.
**Dictation** (🎤) uses the browser's own speech recognition — Safari on
iPhone, Chrome on the desk; nothing passes through remuda.

**Type `/` or `@`** in the composer and a list opens over it: `/` as the first character lists the slash commands the pane's agent knows — Claude Code's own, your commands and skills under `~/.claude`, the project's under its `.claude` — and `@` lists the folder's files as git sees them (Codex and Gemini panes get their own command lists); ↑ ↓ walk, ↵ or Tab picks, Esc closes, a tap picks on the phone. **Switch the model** (View menu, or the palette) tells a Claude Code pane `/model` with Opus, Sonnet, Haiku or an id you type: the terminal shows the switch, the next answer's foot line names the model, and Claude Code keeps the pick as its default for new sessions, as its own `/model` does.

**Machines** carry the names herdr gave them, and every remote one a
hue of its own — the edge of its block in the sidebar, of its tiles on
the Wall, a dot in the palette — so three machines are told apart at a
glance; the ⋯ beside a machine
renames it for remuda (the Mac you sit at too), copies its SSH host, or
removes one you added here.

**A new workspace** names a machine and a folder; the folder is picked
from the machine's own folders (repositories marked, the workspaces
already there as quick picks) or typed. Under the agent, **Options**
unfolds what else to tell it: the model (Opus, Sonnet or Haiku for
Claude Code, or any model id; an id for Codex, Gemini and Kimi Code),
extra arguments as you would type them after the command
(`--permission-mode plan`), and a first prompt, which remuda hands over
once the agent is up — a trust question Claude Code asks for the folder
is yours to answer first, and the prompt waits two minutes for it before
giving up with a note in Activity. The model last used for each agent is
remembered on the device. A new tab and a new worktree take the same
options.

**A new worktree comes ready to run.** A worktree has none of the files
git ignores — no `.env`, no local config — so an agent started in one
often cannot run the project. List them in a `.worktreeinclude` at the
repository's root, in `.gitignore`'s syntax (the same file Claude Code
reads):

```gitignore
.env
.env.*
!.env.example
config/local/
```

Every worktree remuda makes — a repository folder's New workspace, or a task's
*Start an agent on it* — gets them copied from the main checkout before
its agent starts, and a toast (or the task's comment) says which. Only
files git ignores are copied, never one the worktree already has, never
anything outside the repository, and nothing at all when the patterns
match more than a thousand files.

**Recipes** save a start you make again and again — the agent, its
model, the extra arguments and the first prompt, under a name like
"Planner" or "Scout", with a folder when the recipe belongs to one.
Pick one in the Recipe field of the New workspace, tab or worktree
dialog and every option fills in (each field stays yours to change);
"Save as recipe…" under the options keeps what you typed; "New agent
from recipe…" in the palette and "Agent from recipe…" in the sidebar's
+ menu start one straight away in a folder — the recipe's own, else the
folder you last started it in, else the one you are looking at. A
recipe also fills the "Start an agent" kind of schedule. Recipes are
kept on this remuda and shared by every device that uses it;
"Recipes…" in the palette edits the list.

**Several remuda at once:** each machine may run its own (the phone on
the studio Mac's, the desk on its own). herdr lets one client attach to a terminal
at a time, so when a remote machine's own remuda is running (and
reachable on its loopback port 7317), this one reaches that machine's
terminals through it instead of attaching directly — both sides can look
at the same pane, and nobody gets "already has an attached client". The
vitals probe finds it and reads its token over your SSH; without it the
direct attach stays.

**Machine vitals:** the ⓘ next to each machine shows which Claude Code,
herdr and remuda run there, disk and load — and the day they had: a small
chart under each from the readings remuda already takes every fifteen
minutes (kept in `vitals.json`, nothing new is probed), with the limits
drawn as dashed lines, so a disk filling up or a load that never drops
is visible as a trend, not a number. It turns amber when an update
is out (with the right command for how it was installed), a remote
remuda is behind this one, or the disk is nearly full.

**Before something quietly stops working:** a scout at three in the
morning or a queued task dies on an expired login. Claude Code says when
its stored sign-in ends (`claude auth status`, 2.1.318 and up); the
vitals read it every six hours and whenever you press *Read again* —
only the end of the sign-in leaves the machine, never the account's
e-mail or organization. Claude Code also says the sign-in may end up to
three days before that day, and that renewing it moves neither, so
remuda counts to the earlier one: from ten days before the end, the
machine's ⓘ says "Claude Code's sign-in on GOG may end in 6 days, by
21 Nov 2026 at the latest", red from two days before the earliest end
and from then on, with *Open a shell there* for `claude auth login`
(remuda never signs in for you); a push comes three days before the
earliest end, a day before it and once it may end at any time, one each,
never twice across a restart. The
same ⓘ warns of a model or mode being retired: fast mode on Opus 4.8 is
removed on 30 November 2026 (Claude Code 2.1.316: its requests then fail
and do not fall back), so a recipe, an agent schedule, a project's
agents or a pipeline stage set to it — by its model and `--settings
'{"fastMode":true}'`, or by the default model and fast mode saved in
Claude Code's settings on that machine — and a running agent whose last
answer came in fast mode on it are counted there ("set for 1 recipe and
2 running agents here: move to Opus 5.5"), red once the day has come.
Every form that edits a model says the same as you type, with a button
that puts Opus 5.5 in the field.

**Keep awake while agents work:** a laptop that falls asleep in the
middle of a task leaves a dead chat for the morning. The row of that
name in a machine's ⋯ menu (off by default) has remuda hold the machine
awake — `caffeinate -i` on a Mac, `systemd-inhibit` on Linux, started
as you, no sudo — for as long as any agent on it is working, and let it
go two minutes after the last one settled, or when remuda stops. The
display may still sleep. The ⓘ's Awake row says how it stands ("Kept
awake · 2 agents working"), and why it could not hold on a system with
neither tool. The hold's pid is kept in remuda's state dir on the
machine, so a reconnect or a restart finds the one still running
instead of starting another.

**Wake a sleeping machine:** a desktop at home sleeps while the cockpit
runs on a box in the same network and you are away with the phone. Give
the machine its MAC address once (⋯ › MAC address…) and the menu offers
*Wake up* whenever the machine is away: remuda sends the magic packet on
every network it is on itself — never through another machine — and the
usual reconnect, every 15 s, picks the machine up once it answers. The
machine's firmware must allow Wake-on-LAN, and the cockpit must sit on
the same network, since the packet does not route.

**Cost and context:** every agent pane shows how full its context is (a
pill in the header and on the sidebar row from 50% up, red near the edge,
with "Compact now" in the chat from 80%) and what the session has cost so
far at Claude Code's own API prices. Under every answer in the chat, a
quiet line says how long it took, the tokens it used and what it cost
("4 min 12 s · 38k tokens · $0.42"); every tool call in the log ends
with how long it took, a failed one is red, and that line counts the
failed and the repeated calls. An Edit shows what it changed right
under its line — the old and new lines, the words that differ marked,
a long one folded past two dozen lines — so a review of the agent's
work needs no file opened. On the Wall a tile says how long the
running command has gone on, amber past a minute. 📊 opens the report: by day, by
project, by model, by machine, the sessions that cost the most — every
machine reads its own transcripts, nothing is sent anywhere. On a
subscription it is a measure of where the work went, not a bill.

**Budgets:** at the top of that report, a limit per day, week or month
on everything, on one machine or on one of its folders. The usage pill
in the header turns amber when a budget covering the pane is 80 % spent
and red past the limit, with the budget in its tooltip; a push comes
once at 80 % and once when the limit is crossed, then not again until
the next period. The spend is read every five minutes from the same
transcripts, at the same API-equivalent prices: on a subscription a
measure, not a bill.

**Plan limits:** what a subscription really runs out of — Claude's and
Codex's 5-hour and weekly windows — at the very top of the report, in
the usage pill's tooltip (its colour turns amber at 80 %) and in each
machine's ⓘ: "Claude 5 h: 62 %, resets 14:00". A push comes at 80 % and
when a window is used up, once per window. Codex writes its limits into
its own files, so they show by themselves. Claude Code hands them only to
its status line: Updates in a machine's ⓘ offers "Show Claude Code's
plan limits", which puts remuda in front of your status line in
`~/.claude/settings.json` (your file backed up first) — remuda notes the
numbers and runs your own status line as before; `remuda statusline
uninstall` puts it back. When an agent stops on a limit, its sidebar row
says so and the chat offers "Continue after the reset": at the moment
the window resets, remuda says "continue" to it — your booking, called
off with one click, and never sent to an agent that is busy again,
asking a question or on another conversation by then.

**Command palette** (⌘K, Ctrl+K elsewhere): type a few letters and
jump to any agent on any machine — by its name, workspace, folder,
machine or tab — or run what the page has a button for: a mode, a lens,
a new workspace, tab or worktree, a rename, the pop-out, the files
rail, the panels. The liveliest agents come first, the current one is
marked; ↑ ↓ move, ↵ runs, Esc closes. The last row searches every
conversation for what you typed. **?** opens the keyboard sheet: every
key the page answers, in groups. **Alt+N** goes to the next agent that
needs you — a question open, a watchdog note, a finish you have not
looked at — round and round in the sidebar's order; on the Wall that
tile takes the wall, elsewhere the agent opens on its chat. The browser
tab counts them too: its title reads "(2) remuda" and the favicon wears
a dot in the colour of the most pressing one — red for a question,
amber for a watchdog note, green for a finish — so another tab or the
dock tells you before you switch.

**Search** (🔍, or the palette's last row): full text over every conversation on every
machine, Claude Code's and Codex's — what you or an agent said, never tool output — filtered by
machine, folder and period. A hit jumps to the turn in the running pane, or
opens the old transcript read-only. No index, no database: each machine
scans its own `~/.claude/projects` on demand (remuda's scanner where the
binary is deployed, grep otherwise). Every turn of a pane's conversation
has a **Copy link** button of its own: the address opens remuda on that
pane, in its chat, scrolled to that turn — for a note, a ticket or a
teammate on the same tailnet. Beside it a **pin** keeps the turn: a
pinned turn wears a mark in the margin, and **Pins…** in the View menu
(or the palette) lists every turn you pinned in that folder's
conversations, newest first — a click opens the conversation there, an
older session of the folder included, so a pin survives a compaction.
Pins travel with your settings when sync is on.

**Wall:** a mode for the big screen — every agent on every connected
machine as a tile: its state and workspace, its context and cost, how
long it has been at it, what it runs this second and how far its plan
is — or, once it is done, the first line of its answer — the last dozen turns of its
conversation, the question it asks with its choices as buttons,
and a line to answer. Tiles keep the roster's order, so nothing jumps
when an agent changes state; hide the idle ones, keep only the ones that
need you, or hide any single tile, focus one agent with the rest in a
strip under it, pick the tile size, click a head to open the agent in full, switch the
wall to **Screen** to see every terminal as it is instead of its chat, or put it in
**Office** mode for a TV — one agent large, whoever needs you first, else each in turn,
the rest as cards, pinnable, full screen. In Chrome, a tile
or the current agent **pops out** into a small always-on-top window
(Document Picture-in-Picture), one at a time, closed from its own frame.

Planned: a remote-machine session picker, incremental transcript parsing for
very long sessions.

## License

MIT
