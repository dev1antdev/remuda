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
VERSION="${CORE_VERSION:-0.2.0}"
BIN="$HOME/.local/bin"
EXE="remuda-core"
EXT="tar.gz"

# checksums of the release assets this script knows (sha256, as the release's
# checksums.txt lists them); another version needs CORE_SHA256 set by hand
case "$(uname -s)-$(uname -m)" in
MINGW*-x86_64 | MSYS*-x86_64 | CYGWIN*-x86_64) TARGET="windows-x86_64"; EXE="remuda-core.exe"; EXT="zip"; KNOWN="d0698b106d54e04f92db2df42b4a3750abdd73b89766db4686043df0682902d2" ;;
Darwin-arm64) TARGET="macos-aarch64"; KNOWN="c5785277a4465ded4e0cbcd1c187ea276619ba6885bbb8dfff681e93b3742392" ;;
Darwin-x86_64) TARGET="macos-x86_64"; KNOWN="af9fbdec770b2840a3d236f6c96314bf1972cc6b44d82236b87d0d2ccabafd92" ;;
Linux-aarch64 | Linux-arm64) TARGET="linux-aarch64"; KNOWN="3360a71a367eefbae62111886fca27044907cca06d9292812da9790c4acd5ece" ;;
Linux-x86_64) TARGET="linux-x86_64"; KNOWN="d86fe5c998ba44f983ed36ef1491f77a6f8c1c0db11f97aa15e47c8730359c3b" ;;
*) echo "remuda-core has no build for $(uname -s) $(uname -m)"; exit 1 ;;
esac
[ "$VERSION" = "0.2.0" ] || KNOWN=""
SHA="${CORE_SHA256:-$KNOWN}"
CORE="$BIN/$EXE"

# installed is the version of the remuda-core here, nothing when there is none.
installed() { if [ -x "$CORE" ]; then "$CORE" --version 2>/dev/null | sed -n 's/^rc //p' | head -1; fi; }

# whole says whether what remuda-core needs beside its binary is there: ConPTY on Windows, copied before the binary.
whole() { case "$TARGET" in windows-*) [ -f "$BIN/conpty/conpty.dll" ] ;; *) true ;; esac; }

# older says whether version $1 is older than $2.
older() {
    awk -v a="$1" -v b="$2" 'BEGIN { split(a, x, /[.-]/); split(b, y, /[.-]/)
        for (i = 1; i <= 3; i++) { if (x[i] + 0 < y[i] + 0) exit 0; if (x[i] + 0 > y[i] + 0) exit 1 }
        exit 1 }'
}

HAVE="$(installed)"
case "$ACTION" in
status)
    if [ -n "$HAVE" ]; then echo "remuda-core $HAVE at $CORE"; else echo "remuda-core is not installed here"; fi
    exit 0
    ;;
install) ;;
*) echo "usage: install-core.sh [install|status]"; exit 2 ;;
esac

if [ -n "$HAVE" ] && ! older "$HAVE" "$VERSION" && whole; then
    echo "remuda-core $HAVE already at $CORE"
    exit 0
fi
[ -n "$SHA" ] || { echo "no known checksum for remuda-core $VERSION $TARGET — set CORE_SHA256"; exit 1; }
mkdir -p "$BIN"
# beside the target rather than in /tmp: a noexec temp mount could not run the check below
TMP="$(mktemp -d "$HOME/.local/remuda-core.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
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
    cp -R "$TMP/conpty" "$BIN/"
    echo "remuda-core $HAVE at $CORE: its ConPTY put back"
else
    case "$TARGET" in
    macos-*) "$NEW" --rc-install-bundle "$NEW" "$CORE" ;;
    windows-*) cp -R "$TMP/conpty" "$BIN/" && cp "$NEW" "$CORE.new" && mv -f "$CORE.new" "$CORE" ;;
    *) cp "$NEW" "$CORE.new" && chmod 755 "$CORE.new" && mv -f "$CORE.new" "$CORE" ;;
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
