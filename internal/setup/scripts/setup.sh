#!/bin/sh
# remuda setup: everything a machine needs, idempotent.
#   1. herdr — installed if missing (Homebrew when there is one, else
#      herdr's own installer into ~/.local/bin)
#   2. its headless server, started if none answers
#   3. herdr's agent integrations (the status hooks) for the agents found
# The remuda service itself is installed by install-service.sh after this.
# Env: HERDR_INSTALL=brew|installer (default: brew if present, else installer)
set -e
PATH="$HOME/.local/bin:$HOME/.kimi-code/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"; export PATH

say() { printf '%s\n' "$*"; }

# 1. herdr
if command -v herdr >/dev/null 2>&1; then
    say "herdr: $(herdr --version 2>/dev/null | head -1) at $(command -v herdr)"
else
    how="${HERDR_INSTALL:-}"
    if [ -z "$how" ]; then
        if command -v brew >/dev/null 2>&1; then how=brew; else how=installer; fi
    fi
    case "$how" in
    brew)
        say "herdr: installing with Homebrew…"
        brew install herdr
        ;;
    *)
        say "herdr: installing with herdr's installer into ~/.local/bin…"
        mkdir -p "$HOME/.local/bin"
        if command -v curl >/dev/null 2>&1; then
            curl -fsSL https://herdr.dev/install.sh | HERDR_INSTALL_DIR="$HOME/.local/bin" sh
        else
            wget -qO- https://herdr.dev/install.sh | HERDR_INSTALL_DIR="$HOME/.local/bin" sh
        fi
        ;;
    esac
    command -v herdr >/dev/null 2>&1 || { say "herdr did not install; see above"; exit 1; }
    say "herdr: $(herdr --version 2>/dev/null | head -1)"
fi

# 2. the server
sock="${HERDR_SOCKET:-$HOME/.config/herdr/herdr.sock}"
if herdr status server 2>/dev/null | grep -q "status: running"; then
    say "herdr server: running"
else
    say "herdr server: starting…"
    mkdir -p "$(dirname "$sock")"
    nohup herdr server >> "$(dirname "$sock")/herdr-server.out" 2>&1 < /dev/null &
    i=0; while [ $i -lt 30 ]; do
        if herdr status server 2>/dev/null | grep -q "status: running"; then break; fi
        sleep 0.5; i=$((i+1))
    done
    if herdr status server 2>/dev/null | grep -q "status: running"; then say "herdr server: running"; else say "herdr server did not come up; see $(dirname "$sock")/herdr-server.out"; exit 1; fi
fi

# 3. integrations for the agents present
agents=0
for a in claude codex gemini kimi; do
    command -v "$a" >/dev/null 2>&1 || continue
    agents=$((agents + 1))
    st=$(herdr integration status 2>/dev/null | grep "^$a:" || true)
    case "$st" in
    *"not installed"*)
        say "herdr integration for $a: installing…"
        herdr integration install "$a" || say "  (could not install the $a integration; agent status will be guessed)"
        ;;
    "") ;;
    *) say "herdr integration for $a: $(printf '%s' "$st" | sed "s/^$a: //")" ;;
    esac
done
if [ "$agents" = 0 ]; then
    say "no coding agent on this machine yet: install Claude Code, Codex, Gemini CLI or Kimi Code and sign in to it,"
    say "  then run remuda setup once more, for the hooks that tell remuda what the agent is doing"
fi
say "setup: done"
