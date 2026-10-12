---
name: herdr
description: Read herdr's state and talk to its RPC by hand — the live snapshot, the API schema, saved machines, one request over the socket, a throwaway server for a test. Use when debugging what remuda sees, a new RPC method or field, or a test that needs a herdr of its own.
---

# herdr by hand

herdr is the runtime remuda sits on: it owns every workspace, tab, pane,
agent and status. remuda only reads its snapshot and asks it to act.
Everything here is read-only except where said.

## What is running

```sh
herdr status server          # version, protocol, the socket path
herdr api snapshot | jq .    # the live session.snapshot — exactly what the watcher polls
herdr api schema | jq .      # every RPC method, its params and result
herdr machine list --json    # the remotes remuda mirrors (SyncHerdr, Source "herdr")
```

The default socket is `~/.config/herdr/herdr.sock`; `HERDR_SOCKET`
overrides it for remuda, `HERDR_SOCKET_PATH` for herdr's own commands.

## One request over the socket

Newline-delimited JSON, **one request per connection**: herdr closes the
socket after its reply, so nothing is ever pooled (`internal/herdr/
client.go`).

```sh
printf '{"id":"x","method":"session.snapshot","params":{}}\n' \
  | nc -U ~/.config/herdr/herdr.sock \
  | jq '.result.snapshot | {workspaces: (.workspaces | length), panes: (.panes | length), agents}'
```

The reply is `{"id":…,"result":{"type":"session_snapshot","snapshot":{…}}}`;
the snapshot holds `workspaces`, `tabs`, `panes`, `layouts`, `agents`,
the focused ids, `version` and `protocol`.

A refusal comes back as `{"error":{"code":"…","message":"…"}}` — a
`*herdr.Error` in Go; a failure to reach the socket is a transport error
(`Client.OnTransportError`, which marks a remote machine dead).

Methods remuda uses: `ping`, `session.snapshot`, `agent.start`,
`agent.prompt`, `pane.read`, `pane.send_text`, `pane.send_keys`,
`workspace.create`, `workspace.close`, `workspace.rename`,
`tab.create`, `tab.close`, `tab.rename`, `worktree.create`. The pane
fields that matter are in `herdr.PaneInfo` (`agent_status`:
blocked | done | idle | unknown | working; `agent_session` for the
transcript). Error codes remuda handles by name: `agent_blocked` (a menu
is open → 409), `agent_pane_busy` / `agent_not_ready` (retried while a
new pane's shell comes up).

Mutating methods (`agent.prompt`, `pane.send_*`, `workspace.close`, …)
act on the person's agents. By hand, only on a pane you created
(`/live-check`).

## The terminal

`herdr terminal attach <terminal id>` is a PTY remuda shares between all
viewers of a pane (`attach.go`). herdr allows one attached client per
terminal: a second attach prints "already has an attached client; retry
with --takeover" into the PTY and exits — remuda surfaces that
(`RefusedError`), it never passes the flag, and neither do you.

## A herdr of your own, for a test

herdr ignores `HOME` for its socket. A throwaway server is addressed by
`HERDR_SOCKET_PATH` in every command that touches it — start, status,
stop — or the stop lands on the person's real server.

```sh
D=$(mktemp -d /tmp/hs.XXXX)                          # short: a unix socket path has ~100 bytes
export HERDR_SOCKET_PATH=$D/herdr.sock HERDR_SOCKET=$D/herdr.sock
herdr server &                                       # headless
herdr status server
REMUDA_STATE_DIR=$D/remuda ./bin/remuda -addr 127.0.0.1:7417 -socket $D/herdr.sock
HERDR_SOCKET_PATH=$D/herdr.sock herdr server stop    # when done
```

In Go, `herdr.EnsureServer(bin, socket)` does the start and
`internal/herdr/server_test.go` is the whole pattern, including the stop
that waits for the socket to go quiet.

## herdr's own documentation

For herdr as a product (its config, keys, integrations) rather than
remuda's use of it: `herdr --skill` prints its agent skill,
https://herdr.dev/llms.txt is the debugging reference. Neither is needed
for the RPC above.
