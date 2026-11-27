#!/bin/sh
# Installs remuda as a per-user background service on this machine:
#   macOS → launchd LaunchAgent (starts at login, restarts if it exits)
#   Linux → systemd --user unit (enable-linger so it survives logout)
# Usage: install-service.sh [install|uninstall|status]   (run as the user who runs herdr)
# Remote: `remuda setup -host H` feeds it over ssh (or `make service HOST=…`)
# `remuda uninstall` is the way back: this and everything else remuda put here.
set -e

# xml is a value inside a plist <string>: a HOME with & made the whole file invalid, and the old service was already gone
xml() { printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' -e "s/'/\&apos;/g"; }
# sd is a value inside a systemd unit: % doubled (it is a specifier there), and quotes and backslashes escaped where quoted
sd() { printf '%s' "$1" | sed -e 's/%/%%/g' -e 's/\\/\\\\/g' -e 's/"/\\"/g'; }

ACTION="${1:-install}"
BIN="$HOME/.local/bin/remuda"
ADDR="${REMUDA_ADDR:-127.0.0.1:7317}"
# REMUDA_TAILSCALE=1 makes the service join your tailnet by itself and
# serve HTTPS there (see README "From your phone"); the node's name is
# REMUDA_TAILSCALE_HOSTNAME (default remuda). The first start logs a
# login URL — or set TS_AUTHKEY when installing: use a one-off key, it is
# written into the service file (mode 0600) and ignored once the node has
# its state.
umask 077
TS_ARGS=""
if [ "${REMUDA_TAILSCALE:-0}" = "1" ]; then
    # the name goes into a plist and a unit file both: a DNS label is all either takes safely
    TS_HOST="${REMUDA_TAILSCALE_HOSTNAME:-remuda}"
    case "$TS_HOST" in *[!A-Za-z0-9.-]*|"") echo "REMUDA_TAILSCALE_HOSTNAME must be a DNS name (letters, digits, dots, hyphens)"; exit 1 ;; esac
    TS_ARGS="-tailscale -tailscale-hostname $TS_HOST"
fi
# REMUDA_SERVER=1 makes the service serve the page only (remuda -server):
# this machine is none of the cockpit's, no remuda-core is installed there.
if [ "${REMUDA_SERVER:-0}" = "1" ]; then
    TS_ARGS="-server${TS_ARGS:+ $TS_ARGS}"
fi
LABEL="dev.mardeus.remuda"
PATH_FOR_SERVICE="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"

[ -x "$BIN" ] || { echo "remuda not found at $BIN (the installer or remuda setup puts it there)"; exit 1; }
# a remuda older than -server would leave a unit that exits at once, over and over: refused before the running one is replaced
if [ "${REMUDA_SERVER:-0}" = "1" ] && ! "$BIN" -server -version >/dev/null 2>&1; then
    echo "$("$BIN" -version 2>/dev/null || echo "the remuda") at $BIN does not know -server yet: update it first (Download & install in its Updates, or the installer), then run this again; the running service is left as it is"
    exit 1
fi

case "$(uname -s)" in
Darwin)
    PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
    LOG="$HOME/Library/Logs/remuda.log"
    UID_NUM="$(id -u)"
    case "$ACTION" in
    install)
        mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
        cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$(xml "$LABEL")</string>
  <key>ProgramArguments</key>
  <array>
    <string>$(xml "$BIN")</string>
    <string>-addr</string><string>$(xml "$ADDR")</string>$(for a in $TS_ARGS; do printf '\n    <string>%s</string>' "$(xml "$a")"; done)
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>$(xml "$PATH_FOR_SERVICE")</string>
    <key>HOME</key><string>$(xml "$HOME")</string>$([ -n "$TS_AUTHKEY" ] && printf '\n    <key>TS_AUTHKEY</key><string>%s</string>' "$(xml "$TS_AUTHKEY")")
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>5</integer>
  <key>StandardOutPath</key><string>$(xml "$LOG")</string>
  <key>StandardErrorPath</key><string>$(xml "$LOG")</string>
</dict>
</plist>
EOF
        # the new file is checked before the old service is taken down: an invalid plist left nothing running at all
        if command -v plutil >/dev/null 2>&1 && ! plutil -lint "$PLIST" >/dev/null 2>&1; then
            echo "the service file $PLIST does not parse; the running service is left as it is"; exit 1
        fi
        # replace a running instance of the agent; a stray remuda on the port is yours to stop
        # the previous name (herdr-ui): its service stopped and removed, its binary gone
        launchctl bootout "gui/$UID_NUM/dev.mardeus.herdr-ui" 2>/dev/null || true
        rm -f "$HOME/Library/LaunchAgents/dev.mardeus.herdr-ui.plist" "$HOME/.local/bin/herdr-ui"
        launchctl bootout "gui/$UID_NUM/$LABEL" 2>/dev/null || true
        sleep 1
        # listeners only: lsof's tcp:PORT alone names a connection to that port too, a browser's to another machine's remuda
        if command -v lsof >/dev/null 2>&1 && lsof -ti "tcp:${ADDR##*:}" -sTCP:LISTEN >/dev/null 2>&1; then
            echo "something already listens on $ADDR (pid $(lsof -ti "tcp:${ADDR##*:}" -sTCP:LISTEN | head -1)); stop it first"; exit 1
        fi
        launchctl bootstrap "gui/$UID_NUM" "$PLIST" 2>/dev/null || launchctl load -w "$PLIST"
        sleep 2
        echo "installed $PLIST"
        ;;
    uninstall)
        launchctl bootout "gui/$UID_NUM/$LABEL" 2>/dev/null || launchctl unload -w "$PLIST" 2>/dev/null || true
        rm -f "$PLIST"
        echo "removed $LABEL"
        exit 0
        ;;
    esac
    echo "--- status:"
    if launchctl print "gui/$UID_NUM/$LABEL" >/dev/null 2>&1; then launchctl print "gui/$UID_NUM/$LABEL" | grep -E "state =|pid =" | head -2; else echo "not loaded"; fi
    ;;
Linux)
    UNIT_DIR="$HOME/.config/systemd/user"
    UNIT="$UNIT_DIR/remuda.service"
    case "$ACTION" in
    install)
        # linger first: it is what brings the user manager up on a host reached only over ssh
        loginctl --no-ask-password enable-linger "$(id -un)" 2>/dev/null || echo "warning: loginctl enable-linger failed; the service will stop at logout"
        # a container, WSL without systemd: no user manager to hand a unit to
        systemctl --user show-environment >/dev/null 2>&1 || { echo "no systemd user session here, so no service: start $BIN yourself, or set up with remuda setup -no-service"; exit 1; }
        mkdir -p "$UNIT_DIR"
        cat > "$UNIT" <<EOF
[Unit]
Description=remuda — the cockpit for the agents remuda-core runs
After=default.target

[Service]
ExecStart="$(sd "$BIN")" -addr "$(sd "$ADDR")" $TS_ARGS
KillMode=process
Environment="PATH=$(sd "$PATH_FOR_SERVICE")"$([ -n "$TS_AUTHKEY" ] && printf '\nEnvironment=TS_AUTHKEY=%s' "$(sd "$TS_AUTHKEY")")
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
EOF
        # the previous name (herdr-ui): its unit stopped and removed, its binary gone
        systemctl --user disable --now herdr-ui.service 2>/dev/null || true
        rm -f "$HOME/.config/systemd/user/herdr-ui.service" "$HOME/.local/bin/herdr-ui"
        systemctl --user daemon-reload
        systemctl --user enable remuda.service
        systemctl --user restart remuda.service # a re-install (make deploy + make service) replaces the running one
        echo "installed $UNIT"
        ;;
    uninstall)
        systemctl --user disable --now remuda.service 2>/dev/null || true
        rm -f "$UNIT"
        systemctl --user daemon-reload
        echo "removed remuda.service"
        exit 0
        ;;
    esac
    echo "--- status:"
    systemctl --user is-active remuda.service
    ;;
*)
    echo "unsupported OS: $(uname -s)"; exit 1 ;;
esac

echo "--- health:"
curl -s "http://$ADDR/api/health" || echo "(not answering yet)"
echo
