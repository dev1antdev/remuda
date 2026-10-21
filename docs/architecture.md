# How remuda is put together

remuda is one Go binary with the React page inside. This is the map of
both: what the pieces are, how they may depend on each other, and where a
change goes. `CLAUDE.md` says what each package does in detail;
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

## The page

The same idea on the browser side: layers a dependency may only point
down through, and one folder per lens.

```
┌─ app ────────────────────────────────────────────────────────────────┐
│  app/   the shell: the store (what is selected, the lens, the mode,  │
│         the overlays), the URL that follows it, Header, PaneArea,    │
│         Wall, Overlays — the only place the features are composed    │
├─ features ───────────────────────────────────────────────────────────┤
│  roster · chat · terminal · review · tasks · updates · usage ·       │
│  search · schedules · alerts · files · code · agents · quick         │
│  one folder each: api.ts (its routes), pure modules with their       │
│  tests, the screen in pieces                                         │
├─ ui ─────────────────────────────────────────────────────────────────┤
│  ui/    Dialog, Notice, Toasts, Select, Icon, Logo, Markdown, hooks  │
├─ lib ────────────────────────────────────────────────────────────────┤
│  lib/   http · query · store · prefs · text · time · paths · host ·  │
│         transfer · version                                           │
└──────────────────────────────────────────────────────────────────────┘
```

1. **A screen reads the server through `useQuery`** (`lib/query.ts`): a
   fetch that follows its inputs, polls while on screen, drops an answer
   that arrived too late, says "unchanged" on a 304, and takes a
   `mutate` for what the screen already shows ahead of the server. The
   roster is read once for the page (`features/roster/useRoster.tsx`)
   and handed out.
2. **The page's own state is one store** (`app/store.ts`): the requested
   pane, the lens, the mode, the open dialog, the menu, the panel, the
   notice, the confirm, the viewers. `actions` are the only writes, the
   URL follows it (`app/url.ts`), and nothing fixes selection up in
   effects — `current.ts` derives the shown pane every render.
3. **A feature never imports another feature's screens.** Its `api.ts`
   and pure modules are fair game, and the few pieces shared on purpose
   live where they are named (`agents/SendToAgent`, `files/target`).
   `app/` composes.
4. **The terminal is an engine outside React.** `features/terminal/
   engine/` is plain TypeScript — the transport, the intercepted modes,
   the mouse reports, the selection that stays with the text, the layer,
   copy, links, paste, the geometry — wired by `engine.ts`; `Terminal.tsx`
   is the mount that owns one engine per pane and re-applies props.
5. **Pure logic gets a test beside it** (`*.test.ts`, Vitest, node, no
   DOM): what a label says, how a diff is cut, where the screen is in the
   selection's memory. A piece that would need the DOM is split until the
   logic does not.

## Why it is cut this way

The product is a set of lenses on herdr and the transcripts: the roster,
the chat, the terminal, the Changes lens, the updates, the tasks. Each
lens is a context with a vocabulary of its own, and the code uses that
vocabulary (`Input.Submit`, `Conversation`, `Baseline`, `Update`,
`Launcher`). The domain is thin — the state lives in herdr, git and the
session files — so there are no aggregates, repositories or command
buses: a service with methods, a port for the one thing every context
needs (a shell on a machine), and a bus for the few things they tell
each other. Hexagonal in shape, light in weight. The page mirrors it:
no global state manager, no router library — one small store, one
query hook, folders named after the lenses.
