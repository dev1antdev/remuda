#!/bin/sh
# Installs remuda-core (rc), the terminal runtime remuda's machines move to
# (github.com/dev1antdev/remuda-core, Apache-2.0), or updates it:
#   - a pinned release, its checksum verified against the sums below
#   - into ~/.local/bin, where remuda looks for it: remuda-core, with `rc`
#     beside it when no rc is there; on Windows remuda-core.exe and Microsoft's
#     ConPTY beside remuda.exe; on macOS as `remuda core.app`, which
#     remuda-core keeps under ~/.local/share/remuda-core itself
#   - one installed already is swapped with its own `update --from`: its core
#     restarts on the new build and its terminals keep running
#   - a fresh one starts nothing: its core starts when something connects
#   - one as new as this script's, or newer, is left as it is
# Usage: install-core.sh [install|status]
# The binary carries it: `remuda core [-host H] [install|status]`;
# `remuda uninstall -core` takes it away.
set -eu

ACTION="${1:-install}"
VERSION="${CORE_VERSION:-0.2.4}"
BIN="$HOME/.local/bin"
EXE="remuda-core"
EXT="tar.gz"

# checksums of the release assets this script knows (sha256, as the release's
# checksums.txt lists them); another version needs CORE_SHA256 set by hand
case "$(uname -s)-$(uname -m)" in
MINGW*-x86_64 | MSYS*-x86_64 | CYGWIN*-x86_64) TARGET="windows-x86_64"; EXE="remuda-core.exe"; EXT="zip"; KNOWN="558cedec3347b82e809c05ae3d4ea66b3bf160b4fe9759341c606855403d2031" ;;
Darwin-arm64) TARGET="macos-aarch64"; KNOWN="34c45aa4c12128cb0865c07c9b79a67131b778d5ac737e5919dd50327159e615" ;;
Darwin-x86_64) TARGET="macos-x86_64"; KNOWN="3f32148105f0eb10e198c039d7ad0d8eed8b6fdd55a19b3425ec771e66b2cfb1" ;;
Linux-aarch64 | Linux-arm64) TARGET="linux-aarch64"; KNOWN="986c5d00ab07e6eece022f9839ae5c8942ecc9be9a5f3ccad1215b37ec2f0124" ;;
Linux-x86_64) TARGET="linux-x86_64"; KNOWN="bd3453b6e3fbbc9b70c9aec9c75154cda86456a679bfd67ca93f061674964428" ;;
*) echo "remuda-core has no build for $(uname -s) $(uname -m)"; exit 1 ;;
esac
[ "$VERSION" = "0.2.4" ] || KNOWN=""
SHA="${CORE_SHA256:-$KNOWN}"
CORE="$BIN/$EXE"

# installed is the version of the remuda-core here, nothing when there is none.
installed() { if [ -x "$CORE" ]; then "$CORE" --version 2>/dev/null | sed -n 's/^rc //p' | head -1; fi; }

# whole says whether what remuda-core needs beside its binary is there: ConPTY on Windows, put in place before the binary.
whole() { case "$TARGET" in windows-*) [ -f "$BIN/conpty/conpty.dll" ] && [ -f "$BIN/conpty/x64/OpenConsole.exe" ] ;; *) true ;; esac; }

# conpty puts the release's ConPTY in place whole: copied beside it first, checked, then swapped in.
conpty() {
    rm -rf "$BIN/conpty.new" && cp -R "$TMP/conpty" "$BIN/conpty.new" &&
        [ -f "$BIN/conpty.new/conpty.dll" ] && [ -f "$BIN/conpty.new/x64/OpenConsole.exe" ] &&
        rm -rf "$BIN/conpty" && mv "$BIN/conpty.new" "$BIN/conpty"
}

# older says whether version $1 is older than $2.
older() {
    awk -v a="$1" -v b="$2" 'BEGIN { split(a, x, /[.-]/); split(b, y, /[.-]/)
        for (i = 1; i <= 3; i++) { if (x[i] + 0 < y[i] + 0) exit 0; if (x[i] + 0 > y[i] + 0) exit 1 }
        exit 1 }'
}

case "$ACTION" in
status)
    HAVE="$(installed)"
    if [ -n "$HAVE" ]; then echo "remuda-core $HAVE at $CORE"; else echo "remuda-core is not installed here"; fi
    exit 0
    ;;
install) ;;
*) echo "usage: install-core.sh [install|status]"; exit 2 ;;
esac

# one install at a time here, held from the look at what is installed to the end: a folder holding its pid, let go
# however this ends but a kill -9; never taken over, since telling a dead install from one that just began is a race
mkdir -p "$BIN"
LOCK="$BIN/.remuda-core-install"
if ! mkdir "$LOCK" 2>/dev/null; then
    echo "another remuda-core install runs here (pid $(cat "$LOCK/pid" 2>/dev/null || echo unknown)); if none does, remove $LOCK and try again"
    exit 1
fi
TMP=""
trap 'rm -rf "$LOCK" ${TMP:+"$TMP"}' EXIT
trap 'exit 1' INT TERM HUP
echo $$ > "$LOCK/pid"

HAVE="$(installed)"
if [ -n "$HAVE" ] && ! older "$HAVE" "$VERSION" && whole; then
    echo "remuda-core $HAVE already at $CORE"
    exit 0
fi
[ -n "$SHA" ] || { echo "no known checksum for remuda-core $VERSION $TARGET — set CORE_SHA256"; exit 1; }
# beside the target rather than in /tmp: a noexec temp mount could not run the check below
TMP="$(mktemp -d "$HOME/.local/remuda-core.XXXXXX")"
ARCHIVE="$TMP/core.$EXT"
URL="https://github.com/dev1antdev/remuda-core/releases/download/remuda-v$VERSION/remuda-core-$TARGET.$EXT"
echo "downloading $URL"
curl -fsSL -o "$ARCHIVE" "$URL"
if command -v sha256sum >/dev/null 2>&1; then GOT=$(sha256sum "$ARCHIVE" | cut -d' ' -f1); else GOT=$(shasum -a 256 "$ARCHIVE" | cut -d' ' -f1); fi
[ "$GOT" = "$SHA" ] || { echo "checksum mismatch: got $GOT, want $SHA"; exit 1; }
echo "checksum ok"
case "$TARGET" in
windows-*) "$(cygpath -u "$SYSTEMROOT")/System32/tar.exe" -xf "$ARCHIVE" -C "$TMP" ;; # Windows' own tar opens a zip, Git Bash's cannot
*) tar -xzf "$ARCHIVE" -C "$TMP" ;;
esac
case "$TARGET" in
macos-*) NEW="$TMP/remuda core.app/Contents/MacOS/remuda-core"; FROM="$TMP/remuda core.app" ;;
*) NEW="$TMP/$EXE"; FROM="$NEW" ;;
esac
"$NEW" --version >/dev/null 2>&1 || { echo "the downloaded remuda-core does not run on this machine"; exit 1; }

if [ -n "$HAVE" ] && older "$HAVE" "$VERSION"; then
    "$CORE" update --from "$FROM"
    echo "remuda-core updated from $HAVE to $VERSION: its terminals kept running"
elif [ -n "$HAVE" ]; then
    conpty
    echo "remuda-core $HAVE at $CORE: its ConPTY put back"
else
    case "$TARGET" in
    macos-*) "$NEW" --rc-install-bundle "$NEW" "$CORE" ;;
    windows-*) conpty && cp "$NEW" "$CORE.new" && mv -f "$CORE.new" "$CORE" || exit 1 ;;
    *) cp "$NEW" "$CORE.new" && chmod 755 "$CORE.new" && mv -f "$CORE.new" "$CORE" || exit 1 ;;
    esac
    echo "remuda-core $VERSION installed at $CORE"
fi
case "$TARGET" in
windows-*) ;;
*) command -v rc >/dev/null 2>&1 || [ -e "$BIN/rc" ] || ln -s remuda-core "$BIN/rc" ;;
esac
# linger: without it systemd ends the core with the last login, an ssh session's too
if [ "$(uname -s)" = Linux ] && command -v loginctl >/dev/null 2>&1; then
    loginctl --no-ask-password enable-linger "$(id -un)" 2>/dev/null || echo "warning: loginctl enable-linger was refused, so the core stops when your last login ends; run sudo loginctl enable-linger -- $(id -un) on this machine once"
fi
"$CORE" --version
