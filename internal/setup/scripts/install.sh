#!/bin/sh
# remuda's installer: the one line for a machine with nothing on it.
#
#   curl -fsSL https://github.com/dev1antdev/remuda/releases/latest/download/install.sh | sh
#
# It downloads the release binary for this OS and CPU, checks it against
# the release's checksums.txt, puts it at ~/.local/bin/remuda and runs
# `remuda setup`: herdr installed if missing, its server started, the
# agent hooks, the background service, the page opened. Nothing asks for
# sudo and nothing but curl (or wget) has to be there first. Run again, it
# is the upgrade. Arguments go to `remuda setup`:
#
#   curl -fsSL …/install.sh | sh -s -- -tailscale
#
# Env: REMUDA_INSTALL_VERSION=v0.5.1  that release instead of the newest
#      REMUDA_INSTALL_ONLY=1          the binary only, no setup
#      REMUDA_INSTALL_MISSING=1       nothing at all when a remuda is there already
#      REMUDA_INSTALL_BASE=https://…  where remuda-<os>-<arch> and checksums.txt
#                                     are served from (a mirror, a test)
# `remuda setup -host H` runs this very script on H when no remuda is there.
set -e

REPO="dev1antdev/remuda"

say() { printf '%s\n' "$*"; }
die() {
    printf 'remuda install: %s\n' "$*" >&2
    exit 1
}

fetch() { # fetch <url> <file>
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --retry 3 -o "$2" "$1"
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$2" "$1"
    else
        die "curl or wget is needed"
    fi
}

sha256() {
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | cut -d' ' -f1
    elif command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    fi
}

# Everything happens in main, called on the last line: a download cut off
# half way defines half a function and runs nothing.
main() {
    case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) die "no release for $(uname -s): macOS and Linux only" ;;
    esac
    case "$(uname -m)" in
    arm64 | aarch64) arch=arm64 ;;
    x86_64 | amd64) arch=amd64 ;;
    *) die "no release for a $(uname -m) CPU: arm64 and amd64 only" ;;
    esac
    asset="remuda-$os-$arch"
    bin="$HOME/.local/bin"

    if [ "${REMUDA_INSTALL_MISSING:-0}" = "1" ] && [ -x "$bin/remuda" ]; then
        say "remuda: $("$bin/remuda" -version 2>/dev/null || echo "there already") at $bin/remuda"
        return 0
    fi

    base="${REMUDA_INSTALL_BASE:-}"
    if [ -z "$base" ]; then
        if [ -n "${REMUDA_INSTALL_VERSION:-}" ]; then
            base="https://github.com/$REPO/releases/download/$REMUDA_INSTALL_VERSION"
        else
            base="https://github.com/$REPO/releases/latest/download"
        fi
    fi

    mkdir -p "$bin"
    new="$bin/remuda.new"
    sums="$bin/remuda.sums"
    trap 'rm -f "$new" "$sums"' EXIT
    say "remuda: downloading $base/$asset"
    fetch "$base/$asset" "$new" || die "could not download $base/$asset"
    fetch "$base/checksums.txt" "$sums" || die "could not download $base/checksums.txt, so the binary cannot be checked"
    want=$(awk -v n="$asset" '{ f=$NF; sub(/^\*/, "", f); if (f == n) print $1 }' "$sums" | head -1)
    have=$(sha256 "$new")
    [ -n "$have" ] || die "shasum or sha256sum is needed to check the download"
    if [ -z "$want" ] || [ "$want" != "$have" ]; then
        die "checksum mismatch: the download is not what the release says (want ${want:-nothing listed for $asset}, got $have)"
    fi
    say "remuda: checksum ok"
    chmod +x "$new"
    version=$("$new" -version) || die "the downloaded binary does not run here"
    mv "$new" "$bin/remuda" # a rename: safe while an older one runs
    rm -f "$sums"
    trap - EXIT
    say "remuda: $version at $bin/remuda"

    if [ "${REMUDA_INSTALL_ONLY:-0}" = "1" ]; then
        return 0
    fi
    say ""
    # stdin is the pipe this script came through: setup must not read it
    "$bin/remuda" setup -open "$@" </dev/null
    say ""
    case ":$PATH:" in
    *":$bin:"*) say "to take it off this machine again: remuda uninstall" ;;
    *) say "to take it off this machine again: $bin/remuda uninstall" ;;
    esac
}

main "$@"
