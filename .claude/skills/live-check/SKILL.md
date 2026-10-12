---
name: live-check
description: Exercise remuda against the real herdr end to end — a scratch workspace through the API, a prompt, keys, the chat and the pane's screen, then delete it — touching nothing of the person's. Use after changing internal/server, internal/herdr, internal/machines, submit or keys, the chat or terminal transports, where unit tests cannot reach.
---

# Live check

The person's own agents sit in the same herdr and show in the same
roster. Only the workspace this check creates is yours; everything else
is read-only to you. Never `--takeover`, never type into a pane you did
not create, never `herdr server stop` without `HERDR_SOCKET_PATH`.

## 1. A server to talk to

Your own instance beside the running one — its own token, tasks and
pushes, the same herdr:

```sh
make build
export REMUDA_STATE_DIR=$(mktemp -d)
./bin/remuda -addr 127.0.0.1:7417 -no-version-check &
BASE=http://127.0.0.1:7417
TOKEN=$(./bin/remuda -print-token)
api() { curl -sS -H "Authorization: Bearer $TOKEN" "$@"; }
api $BASE/api/health            # {"ok":true,"herdr":true,"herdr_version":…}
```

Or the service that already runs: `BASE=http://127.0.0.1:7317`, `TOKEN=$(remuda -print-token)`
(the file is `~/Library/Application Support/remuda/token` on macOS,
`~/.config/remuda/token` on Linux). A bearer token and no Origin header
is how the CSRF guard lets curl through.

```sh
api $BASE/api/roster | jq '.machines[] | {id, label, state, workspaces: (.sessions | length)}'
```

## 2. A scratch workspace

```sh
mkdir -p /tmp/remuda-live
R=$(api -X POST $BASE/api/sessions -H 'Content-Type: application/json' \
      -d '{"machine":"local","dir":"/tmp/remuda-live","agent":"claude"}')
echo "$R"                                  # {"id":"<workspace>","pane":"<pane>"} — "agent_error" if the agent did not start
WS=$(echo "$R" | jq -r .id); PANE=$(echo "$R" | jq -r .pane)
```

`agent` is any kind herdr has a manifest for (`claude`, `codex`,
`gemini`, …) or `shell`. The roster shows the pane within a poll (2 s):

```sh
api $BASE/api/roster | jq --arg p "$PANE" '.. | objects | select(.id? == $p)'
```

## 3. Drive it

The screen, without a WebSocket (the HTTP transport the phone falls back
to): open a stream, long-poll its bytes.

```sh
SID=$(api -X POST "$BASE/api/term/open?machine=local&id=$PANE&cols=120&rows=40" | jq -r .sid)
api "$BASE/api/term/read?sid=$SID&wait=3" | tail -c 3000
```

A fresh folder makes Claude Code ask whether to trust it. Read the screen
and answer it the way a person would, with keys — this is your folder.

```sh
api -X POST $BASE/api/keys -H 'Content-Type: application/json' \
    -d "{\"machine\":\"local\",\"pane\":\"$PANE\",\"keys\":[\"enter\"]}"
```

Keys are lowercase with `+` for modifiers: `enter`, `esc`, `up`, `down`,
`tab`, `shift+tab`, `backspace`, `ctrl+c`.

A prompt goes through herdr's `agent.prompt`:

```sh
api -X POST $BASE/api/submit -H 'Content-Type: application/json' \
    -d "{\"machine\":\"local\",\"pane\":\"$PANE\",\"text\":\"Reply with the single word pong.\"}"
```

A 409 while the agent waits on a menu (`agent_blocked`) is the correct
answer: never type into the menu, answer it with keys. The transcript:

```sh
api "$BASE/api/chat?machine=local&pane=$PANE" | jq '{session_id, turns: (.turns | length), last: .turns[-1]}'
```

Other things worth a look from here: `GET /api/term/read` after a
resize (`POST /api/term/resize`), `POST /api/seen` and the radar state on
the roster pane, `GET /api/review?dir=/tmp/remuda-live`.

## 4. Clean up — always

```sh
api -X DELETE "$BASE/api/sessions/$WS"
api $BASE/api/roster | jq --arg w "$WS" '[.. | objects | select(.id? == $w)] | length'   # 0
kill %1                              # your instance, if you started one
rm -rf "$REMUDA_STATE_DIR" /tmp/remuda-live
```

A scratch workspace left behind sits in the person's herdr sidebar. If
the delete fails, say so in the report with the workspace id.

## Reading herdr itself

`/herdr` — the snapshot, the RPC by hand, a throwaway server.
