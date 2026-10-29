#!/bin/sh
# make e2e — the end-to-end smoke: a throwaway herdr server on a socket of
# its own, a remuda of its own on top of it and headless Chrome, driven by
# e2e/smoke.mjs over the DevTools protocol. Nothing of the person's is
# touched: herdr and remuda get a HOME, a socket and a state dir under one
# temp folder, which goes when the run passes (E2E_KEEP=1 keeps it).
#
# Env: E2E_PORT (7431), E2E_CDP_PORT (9361), E2E_BIN (built when unset),
# E2E_HERDR (herdr), CHROME_BIN (found when unset), E2E_OUT (screenshots,
# <tmp>/shots when unset), E2E_KEEP.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
port=${E2E_PORT:-7431}
cdp=${E2E_CDP_PORT:-9361}
bin=${E2E_BIN:-$root/bin/remuda}
herdr=${E2E_HERDR:-herdr}

say() { printf 'e2e: %s\n' "$1"; }
fail() { say "FAIL: $1"; exit 1; }

command -v "$herdr" >/dev/null 2>&1 || fail "no herdr on PATH (E2E_HERDR names one)"
command -v node >/dev/null 2>&1 || fail "no node on PATH"
command -v curl >/dev/null 2>&1 || fail "no curl on PATH"
chrome=${CHROME_BIN:-}
if [ -z "$chrome" ]; then
  for candidate in "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" google-chrome google-chrome-stable chromium chromium-browser; do
    if command -v "$candidate" >/dev/null 2>&1; then chrome=$candidate; break; fi
  done
fi
[ -n "$chrome" ] || fail "no Chrome found: set CHROME_BIN"

if [ -z "${E2E_BIN:-}" ]; then
  say "building bin/remuda"
  (cd "$root" && make -s build >/dev/null) || fail "make build"
fi

# A unix socket path must stay short, so the run lives under /tmp, not the system's long temp dir.
tmp=$(mktemp -d /tmp/e2e.XXXXXX)
sock=$tmp/herdr.sock
home=$tmp/home
out=${E2E_OUT:-$tmp/shots}
mkdir -p "$home" "$tmp/ws" "$tmp/state" "$tmp/chrome" "$out"
export HERDR_SOCKET_PATH=$sock HERDR_SOCKET=$sock

stop() {
  status=$?
  trap - EXIT INT TERM
  [ -n "${rpid:-}" ] && kill "$rpid" 2>/dev/null
  [ -n "${cpid:-}" ] && kill "$cpid" 2>/dev/null
  HOME=$home HERDR_SOCKET_PATH=$sock "$herdr" server stop >/dev/null 2>&1 || true
  wait 2>/dev/null || true
  chrome_gone
  if [ "$status" -ne 0 ]; then
    say "logs in $tmp (herdr.log, remuda.log, chrome.log), screenshots in $out"
  elif [ -z "${E2E_KEEP:-}" ]; then
    rm -rf "$tmp" 2>/dev/null || rm -rf "$tmp" 2>/dev/null || say "left $tmp behind"
  fi
  exit "$status"
}
# Chrome's helpers outlive the pid that was killed and keep writing the profile for a moment; the rm raced them once.
chrome_gone() {
  i=0
  while [ "$i" -lt 40 ] && pgrep -f -- "--user-data-dir=$tmp/chrome" >/dev/null 2>&1; do sleep 0.25; i=$((i+1)); done
}
trap stop EXIT INT TERM

up() { HOME=$home HERDR_SOCKET_PATH=$sock "$herdr" status server 2>/dev/null | grep -q "status: running"; }

say "starting a throwaway herdr at $sock"
(cd "$home" && HOME=$home HERDR_SOCKET_PATH=$sock HERDR_SOCKET=$sock nohup "$herdr" server >"$tmp/herdr.log" 2>&1 </dev/null &)
i=0
while ! up; do
  i=$((i + 1))
  [ "$i" -lt 60 ] || fail "herdr did not start: $(cat "$tmp/herdr.log")"
  sleep 0.5
done

say "starting remuda on 127.0.0.1:$port"
HOME=$home REMUDA_STATE_DIR=$tmp/state "$bin" -addr "127.0.0.1:$port" -socket "$sock" -herdr-bin "$herdr" -no-version-check >"$tmp/remuda.log" 2>&1 &
rpid=$!
i=0
while ! curl -sf "http://127.0.0.1:$port/api/health" >/dev/null 2>&1; do
  i=$((i + 1))
  [ "$i" -lt 60 ] || fail "remuda did not answer: $(cat "$tmp/remuda.log")"
  sleep 0.5
done
token=$(HOME=$home REMUDA_STATE_DIR=$tmp/state "$bin" -print-token)

say "starting Chrome"
sandbox=
[ -n "${CI:-}" ] && sandbox=--no-sandbox
"$chrome" --headless=new $sandbox --disable-gpu --disable-dev-shm-usage --no-first-run --no-default-browser-check \
  --remote-debugging-port="$cdp" --user-data-dir="$tmp/chrome" about:blank >"$tmp/chrome.log" 2>&1 &
cpid=$!
i=0
while ! curl -sf "http://127.0.0.1:$cdp/json/version" >/dev/null 2>&1; do
  i=$((i + 1))
  [ "$i" -lt 60 ] || fail "Chrome did not answer: $(cat "$tmp/chrome.log")"
  sleep 0.5
done

E2E_URL="http://127.0.0.1:$port" E2E_CDP="http://127.0.0.1:$cdp" E2E_TOKEN=$token E2E_OUT=$out E2E_WS_DIR=$tmp/ws \
  node "$root/e2e/smoke.mjs"
say "ok"
