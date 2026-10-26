#!/bin/sh
# Registers remuda's MCP server (`remuda mcp`: the person's task list for any
# coding agent) with each agent found on this machine, through the agent's
# own command, so nothing of theirs is edited by hand:
#   - Claude Code   claude mcp add --scope user remuda -- <remuda> mcp
#   - Codex         codex mcp add remuda -- <remuda> mcp
#   - Gemini CLI    gemini mcp add -s user remuda <remuda> mcp
#   - Kimi Code     kimi mcp add remuda -- <remuda> mcp, when its CLI has mcp
# Usage: install-mcp.sh [install|status|uninstall]
# The binary carries it: `remuda mcp install [-host H]` (also the Updates row
# "Connect the agents to your tasks").
set -e

ACTION="${1:-install}"
PATH="$HOME/.local/bin:$HOME/.kimi-code/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"; export PATH
NAME=remuda
BIN="$(command -v remuda 2>/dev/null || true)"
[ -n "$BIN" ] || BIN="$HOME/.local/bin/remuda"
[ -x "$BIN" ] || { echo "remuda is not installed here ($BIN is missing): curl -fsSL https://github.com/dev1antdev/remuda/releases/latest/download/install.sh | sh"; exit 1; }

case "$ACTION" in
install | status | uninstall) ;;
*) echo "usage: install-mcp.sh [install|status|uninstall]"; exit 2 ;;
esac

rc=0
seen=0

# registered says whether the agent's own list names the server.
registered() {
    "$1" mcp list 2>/dev/null | grep -q "$NAME"
}

# forget takes the server out of the agent's config, quietly when it was not there.
forget() {
    case "$1" in
    claude) claude mcp remove -s user "$NAME" >/dev/null 2>&1 || true ;;
    codex) codex mcp remove "$NAME" >/dev/null 2>&1 || true ;;
    gemini) gemini mcp remove -s user "$NAME" >/dev/null 2>&1 || true ;;
    kimi) kimi mcp remove "$NAME" >/dev/null 2>&1 || true ;;
    esac
}

# register adds the server: the absolute path of remuda, so the agent needs no PATH of its own.
register() {
    case "$1" in
    claude) claude mcp add --scope user "$NAME" -- "$BIN" mcp >/dev/null ;;
    codex) codex mcp add "$NAME" -- "$BIN" mcp >/dev/null ;;
    gemini) gemini mcp add -s user "$NAME" "$BIN" mcp >/dev/null ;;
    kimi) kimi mcp add "$NAME" -- "$BIN" mcp >/dev/null ;;
    esac
}

for agent in claude codex gemini kimi; do
    if ! command -v "$agent" >/dev/null 2>&1; then
        echo "$agent: not installed here"
        continue
    fi
    seen=1
    if ! "$agent" mcp --help >/dev/null 2>&1; then
        echo "$agent: its command has no mcp subcommand; add the server by hand (the README shows the lines): $BIN mcp"
        continue
    fi
    case "$ACTION" in
    install)
        forget "$agent"
        if register "$agent"; then
            echo "$agent: registered ($BIN mcp)"
        else
            echo "!! $agent: could not register; add the server by hand (the README shows the lines): $BIN mcp"
            rc=1
        fi
        ;;
    uninstall)
        forget "$agent"
        echo "$agent: the server is removed"
        ;;
    status)
        if registered "$agent"; then echo "$agent: registered"; else echo "$agent: not registered (remuda mcp install)"; fi
        ;;
    esac
done

[ "$seen" = 1 ] || echo "no coding agent is installed here (claude, codex, gemini, kimi)"
exit $rc
