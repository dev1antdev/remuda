#!/bin/sh
# Claude Code PreToolUse hook for Bash: refuses the few commands that would
# reach the person's real herdr or steal their attach. The rules are the
# ones CLAUDE.md spells out under Invariants and "A throwaway herdr server
# for a test"; the rest of Bash is left alone. Exit 2 refuses and hands
# stderr to the agent as the reason.
. "$(dirname "$0")/common.sh"

cmd=$(field .tool_input.command)
[ -n "$cmd" ] || exit 0

has() { printf '%s' "$cmd" | grep -Eq -e "$1"; }
refuse() {
  printf 'remuda guard: %s\n' "$1" >&2
  exit 2
}

# herdr's attach slot is the person's to take, never ours (CLAUDE.md, Peers):
# the flag is refused on a line that could run it (herdr or attach beside
# it); prose that only names it goes through the Write / Edit tools.
if has '(herdr|attach).*--takeover'; then
  refuse "never pass --takeover: a second attach is refused by herdr and the refusal is surfaced (RefusedError / 409); stealing the slot is the person's call only. To write the flag into a file use the Write or Edit tool; to search for it use Grep."
fi

scoped() { has 'HERDR_SOCKET_PATH='; }

# `herdr server stop` on the default socket stops the person's server and
# every agent in it. A throwaway server is addressed by its socket.
if has 'herdr[[:space:]]+server[[:space:]]+stop' && ! scoped; then
  refuse "'herdr server stop' without HERDR_SOCKET_PATH=... stops the person's real herdr server and every agent in it. Address a throwaway server by its socket in the same command: HERDR_SOCKET_PATH=/tmp/hs/herdr.sock herdr server stop (see internal/herdr/server_test.go)."
fi

# herdr ignores HOME for its socket, so a changed HOME still talks to the
# real server — a test that leans on HOME alone is the mistake that
# happened once.
if has '(^|[[:space:];&|(])HOME=[^[:space:]]*[[:space:]]+([^[:space:]]*/)?herdr([[:space:]]|$)' && ! scoped; then
  refuse "herdr ignores HOME for its socket: with HOME changed it still reaches the person's real server. Set HERDR_SOCKET_PATH=<your socket> in the same command (HOME may stay changed beside it)."
fi

exit 0
