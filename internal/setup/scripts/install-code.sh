#!/bin/sh
# Installs code-server (VS Code in the browser, MIT) as a per-user background
# service on this machine, for remuda's Code mode:
#   - a pinned release from github.com/coder/code-server, checksum verified
#   - unpacked under ~/.local/opt, no root needed
#   - bound to 127.0.0.1:$CODE_PORT with no auth of its own: remuda's token
#     guards it, and nothing else can reach a loopback port
#   - macOS → launchd LaunchAgent, Linux → systemd --user unit
# Usage: install-code.sh [install|uninstall|status]
# The binary carries it: `remuda code [-host H] [install|uninstall|status]`
# (`make code [HOST=…]` from a checkout is the same script).
set -e

ACTION="${1:-install}"
VERSION="${CODE_VERSION:-4.140.0}"
PORT="${CODE_PORT:-7320}"
LABEL="dev.mardeus.code-server"
OPT="$HOME/.local/opt"
DEST="$OPT/code-server-$VERSION"
LINK="$OPT/code-server"
CONF_DIR="$HOME/.config/code-server"
PATH_FOR_SERVICE="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"

# checksums of the release assets this script knows (sha256, as GitHub
# lists them for the release); another version needs CODE_SHA256 set by hand
case "$(uname -s)-$(uname -m)" in
Darwin-arm64) ASSET="macos-arm64"; KNOWN="82c7144406ac31c373acfa786b6705c7c5463d895f728fde2cb94402b945b301" ;;
Darwin-x86_64) ASSET="macos-amd64"; KNOWN="a5393b6eed4aa68b084e724c3c565f805abd996c609356043119f0323e40cf52" ;;
Linux-aarch64 | Linux-arm64) ASSET="linux-arm64"; KNOWN="ae4b07153f2037b06d24749bc8004221fcbf3ffe317038401be0452f541bf200" ;;
Linux-x86_64) ASSET="linux-amd64"; KNOWN="864c5d01c808ade57e4d12c708717be7a187219fded60428f263b9e2da9f6b48" ;;
*) echo "unsupported platform: $(uname -s) $(uname -m)"; exit 1 ;;
esac
[ "$VERSION" = "4.140.0" ] || KNOWN=""
SHA="${CODE_SHA256:-$KNOWN}"

install_release() {
    if [ -x "$DEST/bin/code-server" ]; then
        echo "code-server $VERSION already at $DEST"
    else
        [ -n "$SHA" ] || { echo "no known checksum for code-server $VERSION $ASSET — set CODE_SHA256"; exit 1; }
        TGZ="$(mktemp -t code-server.XXXXXX)"
        trap 'rm -f "$TGZ"' EXIT
        URL="https://github.com/coder/code-server/releases/download/v$VERSION/code-server-$VERSION-$ASSET.tar.gz"
        echo "downloading $URL"
        curl -fsSL -o "$TGZ" "$URL"
        if command -v shasum >/dev/null 2>&1; then GOT=$(shasum -a 256 "$TGZ" | cut -d' ' -f1); else GOT=$(sha256sum "$TGZ" | cut -d' ' -f1); fi
        [ "$GOT" = "$SHA" ] || { echo "checksum mismatch: got $GOT, want $SHA"; exit 1; }
        mkdir -p "$OPT"
        rm -rf "$DEST.tmp"; mkdir -p "$DEST.tmp"
        tar -xzf "$TGZ" -C "$DEST.tmp" --strip-components=1
        rm -f "$TGZ"
        mv "$DEST.tmp" "$DEST"
        echo "unpacked to $DEST"
    fi
    ln -sfn "$DEST" "$LINK"
    # first-run editor settings, only when the user has none yet: a dark
    # theme like remuda's, no telemetry, no Copilot chat in the way
    USER_DIR="$HOME/.local/share/code-server/User"
    if [ ! -f "$USER_DIR/settings.json" ]; then
        mkdir -p "$USER_DIR"
        cat > "$USER_DIR/settings.json" <<JSON
{
  "workbench.colorTheme": "Default Dark Modern",
  "workbench.startupEditor": "none",
  "telemetry.telemetryLevel": "off",
  "chat.commandCenter.enabled": false,
  "workbench.secondarySideBar.defaultVisibility": "hidden",
  "security.workspace.trust.enabled": false
}
JSON
        echo "seeded $USER_DIR/settings.json"
    fi
    mkdir -p "$CONF_DIR"
    cat > "$CONF_DIR/config.yaml" <<EOF
# written by remuda (remuda code) — loopback only, remuda's token is the door
bind-addr: 127.0.0.1:$PORT
auth: none
cert: false
disable-telemetry: true
disable-update-check: true
EOF
}

case "$(uname -s)" in
Darwin)
    PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
    LOG="$HOME/Library/Logs/code-server.log"
    UID_NUM="$(id -u)"
    case "$ACTION" in
    install)
        install_release
        mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
        cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$LINK/bin/code-server</string>
    <string>--config</string><string>$CONF_DIR/config.yaml</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>$PATH_FOR_SERVICE</string>
    <key>HOME</key><string>$HOME</string>
  </dict>
  <key>WorkingDirectory</key><string>$HOME</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>5</integer>
  <key>StandardOutPath</key><string>$LOG</string>
  <key>StandardErrorPath</key><string>$LOG</string>
</dict>
</plist>
EOF
        launchctl bootout "gui/$UID_NUM/$LABEL" 2>/dev/null || true
        sleep 1
        launchctl bootstrap "gui/$UID_NUM" "$PLIST" 2>/dev/null || launchctl load -w "$PLIST"
        sleep 3
        echo "installed $PLIST"
        ;;
    uninstall)
        launchctl bootout "gui/$UID_NUM/$LABEL" 2>/dev/null || launchctl unload -w "$PLIST" 2>/dev/null || true
        rm -f "$PLIST"
        echo "removed $LABEL (its files under $OPT and $CONF_DIR stay: remuda uninstall -code takes them too)"
        exit 0
        ;;
    esac
    echo "--- status:"
    if launchctl print "gui/$UID_NUM/$LABEL" >/dev/null 2>&1; then launchctl print "gui/$UID_NUM/$LABEL" | grep -E "state =|pid =" | head -2; else echo "not loaded"; fi
    ;;
Linux)
    UNIT_DIR="$HOME/.config/systemd/user"
    UNIT="$UNIT_DIR/code-server.service"
    case "$ACTION" in
    install)
        install_release
        mkdir -p "$UNIT_DIR"
        cat > "$UNIT" <<EOF
[Unit]
Description=code-server — VS Code in the browser, for remuda
After=default.target

[Service]
ExecStart=$LINK/bin/code-server --config $CONF_DIR/config.yaml
Environment=PATH=$PATH_FOR_SERVICE
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
EOF
        # linger first: it is what brings the user manager up on a host reached only over ssh
        loginctl enable-linger "$(id -un)" 2>/dev/null || echo "warning: loginctl enable-linger failed; the service will stop at logout"
        systemctl --user daemon-reload
        systemctl --user enable code-server.service
        systemctl --user restart code-server.service # a re-install replaces a running one too
        echo "installed $UNIT"
        ;;
    uninstall)
        systemctl --user disable --now code-server.service 2>/dev/null || true
        rm -f "$UNIT"
        systemctl --user daemon-reload
        echo "removed code-server.service"
        exit 0
        ;;
    esac
    echo "--- status:"
    systemctl --user is-active code-server.service
    ;;
*)
    echo "unsupported OS: $(uname -s)"; exit 1 ;;
esac

echo "--- health:"
curl -s "http://127.0.0.1:$PORT/healthz" || echo "(not answering yet)"
echo
