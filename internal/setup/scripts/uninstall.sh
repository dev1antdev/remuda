#!/bin/sh
# remuda uninstall: what the installer and `remuda setup` put on a machine,
# taken off it again. `plan` only looks and says what would go (exit 10
# when nothing would); `run` does it.
#
# Always: the background service, the binary, the log, the ssh connections
# remuda holds open. Only when asked, by the flags of `remuda uninstall`:
#   REMUDA_RM_DATA=1   the state dir: the token, tasks, schedules, push
#                      keys, the tailnet node's state
#   REMUDA_RM_CODE=1   code-server: its service, the release under
#                      ~/.local/opt, the editor's settings and extensions
#   REMUDA_RM_HERDR=1  herdr: its agent hooks, its server (every pane in
#                      it ends), the binary, its folders
# Nothing needs sudo; what would is named and left where it is. There is
# no `set -e`: a step that fails is said and the rest still goes.

ACTION="${1:-plan}"
PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:$PATH"
export PATH

OS="$(uname -s)"
UID_NUM="$(id -u)"
TMP="${TMPDIR:-/tmp}"
TMP="${TMP%/}"
case "$OS" in
Darwin)
    STATE="${REMUDA_STATE_DIR:-$HOME/Library/Application Support/remuda}"
    OLD_STATE="$HOME/Library/Application Support/herdr-ui"
    SVC_DIR="$HOME/Library/LaunchAgents"
    SVC="dev.mardeus.remuda"
    OLD_SVC="dev.mardeus.herdr-ui"
    CODE_SVC="dev.mardeus.code-server"
    EXT=".plist"
    LOG="$HOME/Library/Logs/remuda.log"
    CODE_LOG="$HOME/Library/Logs/code-server.log"
    ;;
Linux)
    STATE="${REMUDA_STATE_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/remuda}"
    OLD_STATE="${XDG_CONFIG_HOME:-$HOME/.config}/herdr-ui"
    SVC_DIR="$HOME/.config/systemd/user"
    SVC="remuda"
    OLD_SVC="herdr-ui"
    CODE_SVC="code-server"
    EXT=".service"
    LOG="" # journald keeps them
    CODE_LOG=""
    ;;
*)
    echo "unsupported OS: $OS"
    exit 1
    ;;
esac

N=0      # what would go
KEPT=0   # what stays unless its flag is given
FAILED=0 # what could not be removed

say() { printf '%s\n' "$*"; }
tilde() {
    case "$1" in
    "$HOME"/*) printf '~%s' "${1#"$HOME"}" ;;
    *) printf '%s' "$1" ;;
    esac
}
will() {
    N=$((N + 1))
    say "  remove  $*"
}
keep() {
    KEPT=$((KEPT + 1))
    if [ "$ACTION" = plan ]; then say "  keep    $*"; else say "kept     $*"; fi
}
gone() { say "removed  $*"; }
fail() {
    FAILED=1
    say "!!       $*"
}

# A service by its name: the launchd label, or the systemd unit without
# its suffix.
svc_file() { printf '%s' "$SVC_DIR/$1$EXT"; }
svc_up() {
    case "$OS" in
    Darwin) launchctl print "gui/$UID_NUM/$1" >/dev/null 2>&1 ;;
    *) systemctl --user is-active "$1.service" >/dev/null 2>&1 ;;
    esac
}
svc_remove() {
    case "$OS" in
    Darwin) launchctl bootout "gui/$UID_NUM/$1" 2>/dev/null || launchctl unload -w "$(svc_file "$1")" 2>/dev/null || true ;;
    *) systemctl --user disable --now "$1.service" >/dev/null 2>&1 || true ;;
    esac
    rm -f "$(svc_file "$1")"
    if [ "$OS" != Darwin ]; then systemctl --user daemon-reload >/dev/null 2>&1 || true; fi
}

# svc_pid is the process a service runs, nothing when it runs none.
svc_pid() {
    case "$OS" in
    Darwin) launchctl print "gui/$UID_NUM/$1" 2>/dev/null | sed -n 's/^[[:space:]]*pid = \([0-9]*\).*/\1/p' | head -1 ;;
    *) systemctl --user show -p MainPID --value "$1.service" 2>/dev/null | grep -v '^0$' ;;
    esac
}

# by_hand lists the remuda processes that are neither this command nor the
# service's own: one started in a terminal, a `make server`.
by_hand() {
    pgrep -u "$UID_NUM" -x remuda 2>/dev/null | grep -v -e "^$PPID\$" ${SVC_PID:+-e "^$SVC_PID\$"} | tr '\n' ' ' | sed 's/ *$//'
}

# Such a remuda is the person's and stays — except when herdr goes: it
# would start herdr's server again within seconds of its stop.
by_hand_part() {
    o="$(by_hand)"
    [ -n "$o" ] || return 0
    if [ "${REMUDA_RM_HERDR:-0}" != 1 ]; then
        if [ "$ACTION" = plan ]; then
            say "  note    a remuda started by hand runs too (pid $o): it stays, yours to stop"
        else
            say "note     a remuda started by hand still runs (pid $o): it is yours to stop"
        fi
    elif [ "$ACTION" = plan ]; then
        N=$((N + 1))
        say "  stop    a remuda started by hand (pid $o): it would start herdr's server again"
    else
        # shellcheck disable=SC2086 # a list of pids
        kill $o 2>/dev/null
        i=0
        while [ "$i" -lt 20 ] && [ -n "$(by_hand)" ]; do
            sleep 0.2
            i=$((i + 1))
        done
        if [ -n "$(by_hand)" ]; then fail "a remuda started by hand (pid $o) did not stop: it will start herdr's server again"; else say "stopped  a remuda started by hand (pid $o)"; fi
    fi
}

# remuda itself: the service (under the previous name too), the binary,
# the log, the ssh masters it left for its remote machines.
remuda_part() {
    SVC_PID="$(svc_pid "$SVC")"
    for s in "$SVC" "$OLD_SVC"; do
        up=""
        if svc_up "$s"; then up=" (running)"; fi
        if [ -e "$(svc_file "$s")" ] || [ -n "$up" ]; then
            if [ "$ACTION" = plan ]; then
                will "the service $(tilde "$(svc_file "$s")")$up"
            else
                svc_remove "$s"
                gone "the service $s"
            fi
        fi
    done
    for f in "$HOME/.local/bin/remuda" "$HOME/.local/bin/remuda.new" "$HOME/.local/bin/remuda.sums" "$HOME/.local/bin/herdr-ui" "$LOG"; do
        [ -f "$f" ] || continue # an empty name too
        if [ "$ACTION" = plan ]; then
            will "$(tilde "$f")"
        elif rm -f "$f"; then
            gone "$(tilde "$f")"
        else
            fail "could not remove $f"
        fi
    done
    n=0
    for f in "$STATE"/ssh/remuda-ssh-*.ctl "$TMP"/remuda-ssh-*.ctl; do
        [ -S "$f" ] || continue
        n=$((n + 1))
        if [ "$ACTION" = run ]; then
            ssh -S "$f" -O exit remuda >/dev/null 2>&1
            rm -f "$f"
        fi
    done
    if [ "$n" -gt 0 ]; then
        if [ "$ACTION" = plan ]; then will "$n ssh connection(s) remuda holds open to your other machines"; else gone "$n ssh connection(s)"; fi
    fi
    if [ "$ACTION" = run ]; then
        rm -f "$STATE"/ssh/remuda-fw-*.sock "$TMP"/remuda-fw-*.sock 2>/dev/null
        # the service's process, on its way out
        i=0
        while [ -n "$SVC_PID" ] && kill -0 "$SVC_PID" 2>/dev/null && [ "$i" -lt 20 ]; do
            sleep 0.2
            i=$((i + 1))
        done
    fi
    by_hand_part
}

# The state dir. Only a folder that holds remuda's own files is ever
# deleted: REMUDA_STATE_DIR can name anything.
data_part() {
    for d in "$STATE" "$OLD_STATE"; do
        [ -d "$d" ] || continue
        if [ ! -f "$d/token" ] && [ ! -f "$d/tasks.db" ] && [ ! -f "$d/vapid.json" ] && [ ! -f "$d/activity.json" ] && [ -n "$(ls -A "$d" 2>/dev/null)" ]; then
            keep "$(tilde "$d"): none of remuda's files in it, so not for this to delete"
            continue
        fi
        what="your data in $(tilde "$d"): the token, tasks, schedules, push keys"
        if [ "${REMUDA_RM_DATA:-0}" != 1 ]; then
            keep "$what  (-data removes it)"
        elif [ "$ACTION" = plan ]; then
            will "$what"
        else
            ts=0
            if [ -d "$d/tailscale" ]; then ts=1; fi
            if rm -rf "$d"; then
                gone "your data in $(tilde "$d")"
                if [ "$ts" = 1 ]; then say "note     the tailnet node stays listed in your Tailscale admin console until you remove it there"; fi
            else
                fail "could not remove $d"
            fi
        fi
    done
}

# code-server as install-code.sh put it. A systemd unit of that name is
# ours only when it says so; ~/.config/code-server only with our config.
code_part() {
    f="$(svc_file "$CODE_SVC")"
    ours=0
    if [ "$OS" = Darwin ]; then
        if [ -f "$f" ] || svc_up "$CODE_SVC"; then ours=1; fi
    elif grep -q "for remuda" "$f" 2>/dev/null; then
        ours=1
    fi
    if [ "$ours" = 0 ] && ! ls -d "$HOME/.local/opt"/code-server* >/dev/null 2>&1; then
        return 0
    fi
    what="code-server: its service, ~/.local/opt/code-server, the editor's settings and extensions in ~/.local/share/code-server"
    if [ "${REMUDA_RM_CODE:-0}" != 1 ]; then
        keep "$what  (-code removes it)"
    elif [ "$ACTION" = plan ]; then
        will "$what"
    else
        if [ "$ours" = 1 ]; then svc_remove "$CODE_SVC"; fi
        rm -rf "$HOME/.local/opt"/code-server-* "$HOME/.local/opt/code-server" "$HOME/.local/share/code-server"
        rmdir "$HOME/.local/opt" 2>/dev/null
        if grep -q "written by remuda" "$HOME/.config/code-server/config.yaml" 2>/dev/null; then rm -rf "$HOME/.config/code-server"; fi
        if [ -n "$CODE_LOG" ]; then rm -f "$CODE_LOG"; fi
        gone "code-server"
    fi
}

# herdr, last: stopping its server ends every pane, and this script may be
# running in one. The hooks first (they call the binary), then the server,
# the binary by the way it came, the folders.
herdr_part() {
    hb="$(command -v herdr 2>/dev/null || true)"
    if [ -z "$hb" ] && [ ! -d "$HOME/.config/herdr" ] && [ ! -d "$HOME/.local/state/herdr" ]; then
        return 0
    fi
    if [ "${REMUDA_RM_HERDR:-0}" != 1 ]; then
        keep "herdr${hb:+ at $(tilde "$hb")}, its server and the agents running in it  (-herdr removes it)"
        return 0
    fi
    up=0
    hooks=""
    brewed=0
    if [ -n "$hb" ]; then
        if herdr status server 2>/dev/null | grep -q "status: running"; then up=1; fi
        hooks=$(herdr integration status 2>/dev/null | grep -v "not installed" | sed -n 's/^\([a-z0-9_-]*\):.*/\1/p' | tr '\n' ' ' | sed 's/ *$//')
        if command -v brew >/dev/null 2>&1 && brew list --versions herdr >/dev/null 2>&1; then brewed=1; fi
    fi
    if [ "$ACTION" = plan ]; then
        how=""
        if [ "$brewed" = 1 ]; then how=" (brew uninstall herdr)"; fi
        will "herdr${hb:+ at $(tilde "$hb")}$how, ~/.config/herdr${hooks:+, its hooks for: $hooks}"
        if [ "$up" = 1 ]; then
            here=""
            if [ -n "${HERDR_PANE_ID:-}" ]; then here=", this terminal with them"; fi
            say "          its server is running: every agent and shell in its panes ends$here"
        fi
        return 0
    fi
    for a in $hooks; do
        if herdr integration uninstall "$a" >/dev/null 2>&1; then gone "herdr's hook for $a"; else fail "could not remove herdr's hook for $a: herdr integration uninstall $a"; fi
    done
    if [ "$up" = 1 ]; then
        if [ -n "${HERDR_PANE_ID:-}" ]; then
            say "stopping herdr's server: this terminal ends with it, the rest goes without it"
            exec >/dev/null 2>&1
        fi
        herdr server stop >/dev/null 2>&1
        i=0
        while [ "$i" -lt 20 ] && herdr status server 2>/dev/null | grep -q "status: running"; do
            sleep 0.5
            i=$((i + 1))
        done
        if herdr status server 2>/dev/null | grep -q "status: running"; then fail "herdr's server did not stop"; else gone "herdr's server"; fi
    fi
    if [ "$brewed" = 1 ]; then
        if brew uninstall herdr >/dev/null 2>&1; then gone "herdr (Homebrew)"; else fail "brew uninstall herdr did not work: run it yourself"; fi
    elif [ -n "$hb" ]; then
        case "$hb" in
        "$HOME"/*) if rm -f "$hb"; then gone "$(tilde "$hb")"; else fail "could not remove $hb"; fi ;;
        *) say "left     $hb: not under your home and not Homebrew's, so not for this to delete" ;;
        esac
    fi
    if rm -rf "$HOME/.config/herdr" "$HOME/.local/state/herdr"; then gone "$(tilde "$HOME/.config/herdr")"; else fail "could not remove herdr's folders"; fi
}

case "$ACTION" in
plan) say "remuda on $(uname -n):" ;;
run) trap '' HUP TERM ;; # a terminal that goes away half way must not leave half of it
*)
    echo "usage: uninstall.sh plan|run"
    exit 2
    ;;
esac

remuda_part
data_part
code_part
herdr_part

if [ "$ACTION" = plan ]; then
    if [ "$N" -gt 0 ]; then exit 0; fi
    if [ "$KEPT" -gt 0 ]; then
        say "  nothing else of remuda is here: what is kept goes with its flag, all of it with -all"
    else
        say "  nothing of remuda is here"
    fi
    exit 10
fi
say "what this cannot reach: the page's sign-in and preferences saved in your browser, and the files you attached in chats (each project's .remuda/uploads)"
exit "$FAILED"
