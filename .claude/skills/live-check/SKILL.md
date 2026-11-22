---
name: live-check
description: Exercise remuda against the real remuda-core end to end — a scratch workspace through the API, a prompt, keys, the chat and the pane's screen, then delete it — touching nothing of the person's agents. Use after changing internal/server, internal/core, internal/machines, submit or keys, the chat or terminal transports, where unit tests cannot reach.
---

# Live check

The person's own agents sit in the same remuda-core and show in the same
roster. Only the workspace this check creates is yours; everything else is
read-only to you. Never type into a pane you did not create, never stop the
core, and never attach to a terminal of the person's. A scratch remuda on
the person's core does one write as it connects: it marks the panes it
places to be kept (`terminal.keep`) so a reboot restores them — no screen
changes and nothing is typed. When the existing panes must stay untouched,
use `scripts/e2e.sh`'s own core instead.

## 1. A server to talk to

Your own instance beside the running one — its own token, tasks and
pushes, the same remuda-core. Run the check in one shell (or wrap it in
`( … )`), so the trap and the variables below live together:

```sh
make build
STATE= WORK= RID= WS= ATTEMPT=
BASE=http://127.0.0.1:7417
TOKEN=
api() { curl -fsS -H "Authorization: Bearer $TOKEN" "$@"; }
stop_instance() {
  if [ -n "$RID" ]; then kill "$RID" 2>/dev/null; wait "$RID" 2>/dev/null || true; RID=; fi
}
cleanup() {
  trap - EXIT
  if [ -n "$ATTEMPT" ] && [ -z "$WS" ]; then
    # the POST may have made the workspace before its answer was lost: find it by this check's own folder
    if ! WS=$(api "$BASE/api/roster" | jq -r --arg d "$WORK" \
        'first(.machines[] | select(.id == "local") | .sessions[] | select(.dir == $d) | .id) // empty' 2>/dev/null); then
      echo "live-check: could not read the roster after a creation attempt; left $WORK" >&2
      stop_instance
      return 1
    fi
    if [ -z "$WS" ]; then
      echo "live-check: no workspace stands on $WORK after a creation attempt; left $WORK" >&2
      stop_instance
      return 1
    fi
  fi
  if [ -n "$WS" ]; then
    if ! api -X DELETE "$BASE/api/sessions/$WS" | jq -e '.ok == true' >/dev/null 2>&1; then
      echo "live-check: could not close the workspace: $WS; left $WORK" >&2
      stop_instance
      return 1
    fi
    if ! api "$BASE/api/roster" | jq -e --arg w "$WS" \
        '(.machines | any(.id == "local" and .state == "connected" and (.sessions | type == "array")))
         and ([.. | objects | select(.id? == $w)] | length == 0)' >/dev/null 2>&1; then
      echo "live-check: the workspace is still there: $WS; left $WORK" >&2
      stop_instance
      return 1
    fi
  fi
  stop_instance
  [ -n "$STATE" ] && rm -rf "$STATE"
  [ -n "$WORK" ] && rm -rf "$WORK"
  return 0
}
trap 'saved_status=$?; cleanup; cleaned=$?; trap - EXIT; if [ "$cleaned" -ne 0 ]; then [ "$saved_status" -ne 0 ] && exit "$saved_status"; exit 1; fi; exit "$saved_status"' EXIT
trap 'exit 130' INT TERM

# an instance of your own, its own state; to drive the service already running, skip these four lines and set
# BASE=http://127.0.0.1:7317 and TOKEN=$(remuda -print-token) instead
STATE=$(mktemp -d /tmp/remuda-state.XXXXXX) || exit 1
REMUDA_STATE_DIR=$STATE ./bin/remuda -addr 127.0.0.1:7417 -no-version-check &
RID=$!
TOKEN=$(REMUDA_STATE_DIR=$STATE ./bin/remuda -print-token)
api $BASE/api/health            # {"ok":true,"herdr":true,"version":…} — the key keeps its old name
```

The token file of the service is `~/Library/Application Support/remuda/token`
on macOS and `~/.config/remuda/token` on Linux. A bearer token and no Origin
header is how the CSRF guard lets curl through.

```sh
api $BASE/api/roster | jq '.machines[] | {id, label, state, workspaces: (.sessions | length)}'
```

## 2. A scratch workspace

```sh
WORK=$(mktemp -d /tmp/remuda-live.XXXXXX) || exit 1   # this check's own folder
ATTEMPT=1                                    # from here the trap must find the workspace, answer or not
R=$(api -X POST $BASE/api/sessions -H 'Content-Type: application/json' \
      -d "{\"machine\":\"local\",\"dir\":\"$WORK\",\"agent\":\"claude\"}")
echo "$R"                                  # {"id":"<workspace>","pane":"<pane>"} — "agent_error" if the agent did not start
echo "$R" | jq -e '((.id | type) == "string") and ((.id | length) > 0) and ((.pane | type) == "string") and ((.pane | length) > 0)' >/dev/null || { echo "no workspace: $R"; exit 1; }
WS=$(echo "$R" | jq -r .id); PANE=$(echo "$R" | jq -r .pane)
```

`agent` is a kind `sessions.KnownAgent` takes — `claude`, `codex`, `gemini`,
`kimi`, `pi`, `opencode`, `cursor`, `amp`, `grok`, `qwen` — or `shell`; `rc
templates` lists what the core has. The roster shows the pane within a poll
(2 s):

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

A prompt goes through `/api/submit`, which `sessions.Input` hands to the
adapter's `AgentPrompt`: the core's `terminal.send` with the agent's
foreground pid checked as it types (`send_foreground`, Core 33).

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
the roster pane, `GET /api/review?dir=$WORK`.

## 4. Clean up — always

```sh
exit 0        # the EXIT trap closes the workspace, checks it went, and removes the folders
```

There is no second cleanup here: the one function does it whether you
reach this point or leave early. It deletes the workspace if one is still
recorded, requires the DELETE's `ok` and a connected local roster that no
longer lists it, then stops `$RID` and removes `$STATE` and `$WORK`. A
signal exits the shell, so the `EXIT` trap runs once. When you drove the existing service, `STATE` and `RID` are empty
and nothing but `$WORK` goes. If the workspace cannot be closed, the trap
leaves `$WORK` and prints its id, rather than delete the folder an agent is
still working in — say so in the report with the workspace id.

## Reading remuda-core itself

`/core` — the terminals, the schema, remuda's `remuda` document, one
request over the socket, a core of its own for a test.
