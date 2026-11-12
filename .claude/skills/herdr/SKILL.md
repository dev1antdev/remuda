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

Methods remuda uses: `ping`, `session.snapshot`, `events.subscribe`, `agent.start`,
`agent.prompt`, `pane.read`, `pane.send_text`, `pane.send_keys`,
`workspace.create`, `workspace.close`, `workspace.rename`,
`tab.create`, `tab.close`, `tab.rename`, `worktree.create`, `layout.export`,
`layout.apply`, `pane.close`, `pane.process_info` (the scenes), and the
writes into herdr's own UI (`internal/mirror`, source `remuda`):
`pane.report_metadata`, `workspace.report_metadata`,
`notification.show`. The pane
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
stop — or the stop lands on the person's real server. It needs a `HOME`
of its own too: herdr restores the saved session of the `HOME` it runs
under (`~/.config/herdr/session.json` — the person's workspaces, their
agents started again with `--resume`) and saves its own over it. remuda
on it gets the same `HOME`, or it mirrors the person's saved remote
machines (`herdr machine list`) and dials them.

```sh
D=$(mktemp -d /tmp/hs.XXXX)                          # short: a unix socket path has ~100 bytes
export HOME=$D HERDR_SOCKET_PATH=$D/herdr.sock HERDR_SOCKET=$D/herdr.sock
unset HERDR_PANE_ID HERDR_ENV                        # an agent in a herdr pane has the real server's
(cd $D && herdr server > $D/herdr.out 2>&1 &)        # headless
herdr status server
REMUDA_STATE_DIR=$D/remuda ./bin/remuda -addr 127.0.0.1:7417 -socket $D/herdr.sock
HOME=$D HERDR_SOCKET_PATH=$D/herdr.sock herdr server stop    # when done
```

## Its event stream

`events.subscribe {subscriptions: [{type}, …]}` keeps the connection open:
`subscription_started`, then one event a line — the global kinds as
`{"event":"pane_created","data":{…}}`, the per-pane ones (bound to a
`pane_id`) as `{"event":"pane.agent_status_changed",…}` and
`pane.output_matched` (`source` recent, `lines`, `match` substring or
regex in Rust's dialect, a line at a time; told when a match appears, and
once at subscribe for one already standing). A connection's subscriptions
are fixed: a second subscribe on it closes it. A gone pane refuses the
whole request (`pane_not_found`), so does an unreadable regex
(`invalid_regex`); a slow reader gets `events_lost` or a closed
connection. The watcher holds one per machine (`internal/watch/stream.go`).

In Go, `herdr.EnsureServer(bin, socket)` does the start and
`internal/herdr/server_test.go` is the whole pattern, including the stop
that waits for the socket to go quiet.

## herdr's own documentation

For herdr as a product (its config, keys, integrations) rather than
remuda's use of it: `herdr --skill` prints its agent skill,
https://herdr.dev/llms.txt is the debugging reference. Neither is needed
for the RPC above.
