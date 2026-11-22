---
name: core
description: Read remuda-core's state and talk to its API by hand — the terminals it runs, the schema, remuda's own document, one request over the socket, a throwaway core for a test. Use when debugging what remuda sees, a new RPC method or field, or a test that needs a core of its own.
---

# remuda-core by hand

remuda-core is the runtime remuda sits on: it owns every terminal, the
program and agent in it, and its status. remuda keeps its workspaces and
tabs in the core's `remuda` document and labels every terminal it starts.
Everything here is read-only except where said.

## What is running

```sh
rc server status --json      # version, protocol generation, the socket
rc ls --json                 # live terminals: address, labels, state, cwd, size
rc get t7 --json             # one terminal: its argv, cwd, labels, state, status
rc read t7                   # its screen and history as text
```

The API itself is `schema/rc.openrpc.json` in `dev1antdev/remuda-core`; `rc
schema` prints the same from a core, and may open the running core's lock to
answer, so run it against a throwaway core only.

`rc` and `remuda-core` are the same binary. The client socket is
`<session>.sock` under `$XDG_RUNTIME_DIR/remuda-core/`, else
`/run/user/<uid>/remuda-core/`, else the session's state dir
(`~/.local/state/remuda-core/<session>/`); `RC_SOCKET` names another
endpoint and `RC_SESSION` another session.

## One request over the socket

Plain mode is JSON-RPC 2.0, one message a line; a connection may carry
several calls, and ends in framed mode only when a caller asks for it.

```sh
printf '{"jsonrpc":"2.0","id":1,"method":"terminal.list","params":{}}\n' \
  | nc -U "$RC_SOCKET" \
  | jq '.result.terminals[] | {address, labels, state}'
```

A refusal comes back as a JSON-RPC error with a stable `kind`:
`{"code":-32000,"message":"no terminal t9","data":{"kind":"not_found","hint":"…"}}`.

Remuda's own model is the document it keeps on the core:

```sh
printf '{"jsonrpc":"2.0","id":2,"method":"doc.get","params":{"name":"remuda"}}\n' \
  | nc -U "$RC_SOCKET" | jq '.result.value.workspaces[] | {id, label, folder, tabs: [.tabs[].label]}'
```

Each workspace is a folder, each tab a pane tree (`layout`, the same shape
as `herdr.LayoutNode` in `internal/herdr/layout.go`); a terminal carries the
labels `remuda.space` and `remuda.tab` that place it. A terminal labelled
for a workspace the document lacks joins that workspace id all the same;
only an unlabelled one stands in a workspace of its own (`core-<address>`).

Methods remuda uses: `terminal.run`, `terminal.send`, `terminal.keys`,
`terminal.interrupt`, `terminal.read`, `terminal.attach`, `terminal.label`,
`terminal.kill`, `terminal.rm`, `terminal.keep`, `terminal.wait`,
`terminal.put`, `terminal.list`, `terminal.get`, `doc.get` / `doc.set`,
`events.subscribe`, `file.read` / `file.write` / `file.edit`,
`templates.list`, `machine.*` and `server.*`. The fields remuda reads are
in `herdr.PaneInfo` (`agent_status`: blocked | done | idle | unknown |
working; `agent_session` for the transcript) and the terminal record in
`internal/core/terminals.go` (below).
Error kinds remuda handles by name: `agent_blocked` (a menu is open → 409),
`agent_pane_busy` / `agent_not_ready` (retried while a new pane's shell
comes up), `status_mismatch` (a send the core refused untyped: remuda sends
it once more in the same request, `core.Adapter.sendTo`), and
`agent_not_found` / `agent_not_ready` on a message for an agent (409,
nothing sent: it is never typed into the pane).

Mutating methods (`terminal.send`, `terminal.keys`, `terminal.kill`,
`doc.set`, …) act on the person's agents and workspaces. By hand, only on a
terminal or workspace you created (`/live-check`).

## The terminal

A terminal takes any number of viewers, and there is no `--takeover`: the
core sends each viewer its own first paint and then the live bytes, and
the last viewer to act sets the size. remuda attaches one viewer per open
page (`core.Terminals`); `rc attach` is a PTY viewer of its own (ctrl+\
detaches). Attach is framed mode (`schema/README.md` in the core repo):
`[channel u32 LE][kind u8][len u32 LE][payload]`, kinds DATA, CREDIT,
CLOSE, RESIZE, FOCUS, FACTS, AUTOMATIC_INPUT.

`rc read t7` prints the visible screen and the saved history; remuda reads
the same with `terminal.read` (`format` "vt" keeps the colours).

## Its event stream

`rc events --json` follows every change as one JSON line. The core's own
types: `created`, `lost`, `restarted`, `keep`, `finished`, `removed`,
`status`, `turn`, `title`, `labels`, `size`, `foreground`, `doc` (the
`remuda` document moved) and `output` (a row that matched a watch — the
core runs the regex). It has terminals only, so the adapter turns them into
remuda's kinds (`pane.updated`, `pane.agent_status_changed`, `pane.moved`,
`pane.output_matched`, …).

Remuda holds two kinds of subscription, and they recover differently. The
mirror (`internal/core/adapter.go`) is one unfiltered stream per machine
(`internal/watch/stream.go`): read again after a `doc` event, and on a
`missed` it reads the document and refills in place on the same
subscription. The output watches (`internal/core/feeds.go`) are a separate
stream; a `missed` there ends it with `events_lost`, and the watcher
subscribes again.

## A core of its own, for a test

`internal/core/coretest.Server` is the pattern and the only one: it takes
`RC_BIN`, else `remuda-core` on PATH, and skips without one. It starts the
core with an environment of the test's own — `RC_SESSION=test`, `RC_SOCKET`,
`HOME`, `XDG_RUNTIME_DIR`, `XDG_STATE_HOME`, `XDG_CONFIG_HOME` and
`RC_LAUNCHD_DIRECT` (this process's pid, so macOS does not hand the core to
launchd), all under one short temp folder — and stops it over that socket
when the test ends. `scripts/e2e.sh` starts one the same way for the whole
run. Never `server stop` or `serve` by hand: `guard-bash.sh` refuses both,
because either may reach the person's core and every agent in it.

`E2E_KEEP=1 make e2e` keeps the run's artifacts in its temp folder after
the core is stopped; it is not a core to talk to. For a core to talk to,
write a Go test on `coretest.Server` or read one through a test's own
`*coretest.Core` — its `Env` is the environment such a core runs under —
and `rc schema` there prints the API as that core has it.

## remuda-core's own documentation

`rc --skill` prints the core's agent skill, `docs/commands.md` in
`dev1antdev/remuda-core` every command's help, and `schema/README.md` the
wire protocol. `schema/rc.openrpc.json` there is the API.
