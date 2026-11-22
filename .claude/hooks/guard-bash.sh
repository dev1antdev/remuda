#!/bin/sh
# Claude Code PreToolUse hook for Bash: refuses the few commands that would
# reach the person's real herdr or steal their attach (a check of the command
# text, so a spelling it does not know is beyond it). The rules are the
# ones CLAUDE.md spells out under Invariants and "A throwaway remuda-core
# for a test"; the rest of Bash is left alone. Exit 2 refuses and hands
# stderr to the agent as the reason.
. "$(dirname "$0")/common.sh"

cmd=$(field .tool_input.command)
[ -n "$cmd" ] || exit 0
# the lines as written and as the shell joins them, one ending in a backslash going on with the next: a rule that
# matches either form refuses
cmd=$(printf '%s\n' "$cmd"; printf '%s\n' "$cmd" | awk '{ if (sub(/\\$/, "")) printf "%s", $0; else print }')

has() { printf '%s' "$cmd" | grep -Eq -e "$1"; }
refuse() {
  printf 'remuda guard: %s\n' "$1" >&2
  exit 2
}

# herdr's attach slot is the person's to take, never ours (AGENTS.md, hard
# rules): the flag is refused on a line that could run it (herdr or attach
# beside it); prose that only names it goes through the Write / Edit tools.
if has '(herdr|attach).*--takeover'; then
  refuse "never pass --takeover: herdr allows one attached client per terminal, and stealing the slot is the person's call only. To write the flag into a file use the Write or Edit tool; to search for it use Grep."
fi

# herdr stays on the machines for the rollback alone, and it is the person's: a stop, a restart or a start ends or
# replaces every agent in it. No test starts one by hand any more (a test of remuda starts coretest.Server), so no
# herdr server command that changes it passes here; reading its state does. This is a check of the command text, not a
# shell: quotes are read away, but a path or a variable that spells the command is beyond it.
herdr=$(printf '%s' "$cmd" | tr -d "\"'")
if printf '%s' "$herdr" | grep -Eq -e '(^|[^[:alnum:]_.-])herdr([[:space:]]+-[^[:space:]]+)*[[:space:]]+(server([[:space:]]+-[^[:space:]]+)*[[:space:]]+(stop|restart)|server[[:space:]]*($|[;&|<>)]))'; then
  refuse "herdr is kept for the rollback and belongs to the person: starting, stopping or restarting its server ends or replaces every agent in it. A test starts internal/core/coretest.Server, never a herdr (AGENTS.md, hard rules)."
fi

# herdr ignores HOME for its socket, so a changed HOME still reaches the person's real server: a herdr started under
# one, or pointed at it, is never safe from here.
if printf '%s' "$herdr" | grep -Eq -e '(^|[[:space:];&|(])HOME=[^[:space:]]*[[:space:]].*herdr([[:space:]]|$)'; then
  refuse "herdr ignores HOME for its socket: with HOME changed it still reaches the person's real server. A test of remuda starts coretest.Server; a herdr is the person's."
fi

# remuda-core holds the person's agents once they moved off herdr: `server stop` ends every terminal in it, and a
# `serve` beside it fights it for its socket. Which core a command reaches is the shell's to work out (RC_SOCKET,
# inherited or set, comes first), so no stop or serve of a core passes here: a throwaway core is started and stopped
# by scripts/e2e.sh or internal/core/coretest, the person's own by the person (quotes read away, flags may stand
# between the words).
core='(^|[^[:alnum:]_.-])(remuda-core|rc)([[:space:]]+-[^[:space:]]+)*[[:space:]]+(server([[:space:]]+-[^[:space:]]+)*[[:space:]]+stop|serve)([^[:alnum:]_-]|$)'
if printf '%s' "$cmd" | tr -d "\"'" | grep -Eq -e "$core"; then
  refuse "'remuda-core server stop' and 'remuda-core serve' may reach the person's own core and every agent in it, whatever socket the command names. A throwaway core is started and stopped by scripts/e2e.sh or internal/core/coretest; the person stops their own core themselves."
fi

exit 0
