#!/bin/sh
# Installs herdr-ui as a per-user background service on this machine:
#   macOS → launchd LaunchAgent (starts at login, restarts if it exits)
#   Linux → systemd --user unit (enable-linger so it survives logout)
# Usage: install-service.sh [install|uninstall|status]   (run as the user who runs herdr)
# Remote: `herdr-ui setup -host H` feeds it over ssh (or `make service HOST=…`)
set -e

ACTION="${1:-install}"
BIN="$HOME/.local/bin/herdr-ui"
ADDR="${HERDR_UI_ADDR:-127.0.0.1:7317}"
# HERDR_UI_TAILSCALE=1 makes the service join your tailnet by itself and
# serve HTTPS there (see README "From your phone"); the node's name is
# HERDR_UI_TAILSCALE_HOSTNAME (default herdr-ui). The first start logs a
# login URL — or set TS_AUTHKEY when installing: use a one-off key, it is
# written into the service file (mode 0600) and ignored once the node has
# its state.
umask 077
TS_ARGS=""
if [ "${HERDR_UI_TAILSCALE:-0}" = "1" ]; then
    TS_ARGS="-tailscale -tailscale-hostname ${HERDR_UI_TAILSCALE_HOSTNAME:-herdr-ui}"
fi
LABEL="dev.mardeus.herdr-ui"
PATH_FOR_SERVICE="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"

[ -x "$BIN" ] || { echo "herdr-ui not found at $BIN (make deploy HOST=… first)"; exit 1; }

case "$(uname -s)" in
Darwin)
    PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
    LOG="$HOME/Library/Logs/herdr-ui.log"
    UID_NUM="$(id -u)"
    case "$ACTION" in
    install)
        mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
        cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$BIN</string>
    <string>-addr</string><string>$ADDR</string>$(for a in $TS_ARGS; do printf '\n    <string>%s</string>' "$a"; done)
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>$PATH_FOR_SERVICE</string>
    <key>HOME</key><string>$HOME</string>$([ -n "$TS_AUTHKEY" ] && printf '\n    <key>TS_AUTHKEY</key><string>%s</string>' "$TS_AUTHKEY")
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>5</integer>
  <key>StandardOutPath</key><string>$LOG</string>
  <key>StandardErrorPath</key><string>$LOG</string>
</dict>
</plist>
EOF
        # replace a running instance of the agent; a stray herdr-ui on the port is yours to stop
        launchctl bootout "gui/$UID_NUM/$LABEL" 2>/dev/null || true
        sleep 1
        if command -v lsof >/dev/null 2>&1 && lsof -ti "tcp:${ADDR##*:}" >/dev/null 2>&1; then
            echo "something already listens on $ADDR (pid $(lsof -ti "tcp:${ADDR##*:}" | head -1)); stop it first"; exit 1
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
    UNIT="$UNIT_DIR/herdr-ui.service"
    case "$ACTION" in
    install)
        mkdir -p "$UNIT_DIR"
        cat > "$UNIT" <<EOF
[Unit]
Description=herdr-ui — browser/phone client for herdr
After=default.target

[Service]
ExecStart=$BIN -addr $ADDR $TS_ARGS
KillMode=process
Environment=PATH=$PATH_FOR_SERVICE$([ -n "$TS_AUTHKEY" ] && printf '\nEnvironment=TS_AUTHKEY=%s' "$TS_AUTHKEY")
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
EOF
        loginctl enable-linger "$(id -un)" 2>/dev/null || echo "warning: loginctl enable-linger failed; the service will stop at logout"
        systemctl --user daemon-reload
        systemctl --user enable herdr-ui.service
        systemctl --user restart herdr-ui.service # a re-install (make deploy + make service) replaces the running one
        echo "installed $UNIT"
        ;;
    uninstall)
        systemctl --user disable --now herdr-ui.service 2>/dev/null || true
        rm -f "$UNIT"
        systemctl --user daemon-reload
        echo "removed herdr-ui.service"
        exit 0
        ;;
    esac
    echo "--- status:"
    systemctl --user is-active herdr-ui.service
    ;;
*)
    echo "unsupported OS: $(uname -s)"; exit 1 ;;
esac

echo "--- health:"
curl -s "http://$ADDR/api/health" || echo "(not answering yet)"
echo
