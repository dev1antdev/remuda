# How remuda is put together

remuda is one Go binary with the React page inside. This is the map of the
Go side: what the pieces are, how they may depend on each other, and where
a change goes. `CLAUDE.md` says what each package does in detail;
`AGENTS.md` says how to work here.

## The shape

Three rings, and a dependency may only point inwards.

```
┌─ adapters ───────────────────────────────────────────────────────────┐
│  internal/server    HTTP + WebSocket, one file per context, thin     │
│  internal/sshx      OpenSSH as a shell.Shell and a transport         │
│  shell.Local        sh -c on this machine                            │
│  internal/tsnetx    the tailnet listener                             │
├─ contexts (the use cases) ───────────────────────────────────────────┤
│  sessions   what people type into panes, the agents remuda starts    │
│  chat       the conversation behind a pane                           │
│  review     the Changes lens                                         │
│  updates    what a machine can update, and running it as a job       │
│  tasks      the task list, and starting an agent on a task           │
│  health · jobs · restart · watch · radar · watchdog · push ·         │
│  schedule · search · usage · quick · code                            │
├─ the floor ──────────────────────────────────────────────────────────┤
│  machines   the registry; a Machine is a shell.Host with herdr on it │
│  herdr      the RPC client, the attach PTYs, the snapshot questions  │
│  transcript the session file parser and the per-machine Reader       │
│  shell      the port: Run / Stream, Host, Fake                       │
│  script     building POSIX scripts, reading their answers            │
│  events     the bus                                                  │
│  state · auth · text · version                                       │
└──────────────────────────────────────────────────────────────────────┘
        internal/app wires it all and lists who hears which event
```

The contexts are the bounded contexts of the product, in its own words:

| Context | Owns | Package(s) |
|---|---|---|
| Fleet | the machines, their vitals, the newest versions out there | `machines`, `health` |
| Sessions | workspaces, tabs, panes; what reaches an agent | `sessions`, `watch`, `radar`, `watchdog` |
| Transcripts | what an agent said, read-only | `transcript`, `chat`, `search`, `usage` |
| Review | a folder's uncommitted work since the last look | `review` |
| Updates | updating herdr, the agents, remuda; the restarts after | `updates`, `jobs`, `restart` |
| Tasks | remuda's own list, and an agent started on a task | `tasks` |
| Alerts | pushes to the phone | `push` |
| Schedules | prompts on a timetable | `schedule` |

## The rules

1. **`internal/server` holds no logic.** A handler reads the request,
   asks one service, writes the answer. If a handler grows a decision
   that is not about HTTP, the decision moves to its context package.
   Every context has one file in `server` with its routes, registered by
   a `…Routes(mux)` that `Handler()` calls.
2. **Scripts go through the port.** A package that runs scripts on a
   machine takes a `shell.Host` (`Run`, `Stream`, `ID`, `Label`, `Home`,
   `Local`) and never a `*machines.Machine`. The scripts themselves are
   built with `internal/script`: quoting, the `PATH` line, the `cd ||
   exit 0` preludes, the portable `stat`, the separators an answer is
   cut by. `review`, `jobs`, `search` and `usage` are written this way;
   their tests run on a `shell.Fake` with a recorded answer.
3. **Packages hear each other through the bus.** `watch` publishes
   `events.Snapshot` and `events.PaneStatus`; `jobs` publishes
   `events.JobDone`; the rest subscribe. Nothing holds a reference to
   its listeners, and the full list of subscriptions is in
   `internal/app/app.go`, so "who reacts to a pane going done" is one
   screen.
4. **The transcript is read through one reader.** `transcript.Reader`,
   one per machine, is the only code that stats, tails and parses
   session files, locally or over ssh; it follows compactions and
   resolves herdr's session ids. `chat.Service` answers the questions
   the other contexts have of a pane's conversation.
5. **herdr owns every process and status; remuda keeps no session
   state.** The hard rules in `AGENTS.md` stand above all of this.

## What goes where

| You are adding… | It goes in |
|---|---|
| a route | `internal/server/<context>.go`, a thin handler calling the context |
| a decision about a pane, an agent, a folder | the context package, as a method on its service |
| a script a machine runs | the context package, built with `internal/script`, tested on `shell.Fake` |
| something one part must tell another | an event type in `internal/events`, a `Subscribe` line in `app.go` |
| a new feature | a package with its use cases, a file in `server` with its routes, a line in `app.go`, a bullet in `CLAUDE.md` |

## Why it is cut this way

The product is a set of lenses on herdr and the transcripts: the roster,
the chat, the terminal, the Changes lens, the updates, the tasks. Each
lens is a context with a vocabulary of its own, and the code uses that
vocabulary (`Input.Submit`, `Conversation`, `Baseline`, `Update`,
`Launcher`). The domain is thin — the state lives in herdr, git and the
session files — so there are no aggregates, repositories or command
buses: a service with methods, a port for the one thing every context
needs (a shell on a machine), and a bus for the few things they tell
each other. Hexagonal in shape, light in weight.
