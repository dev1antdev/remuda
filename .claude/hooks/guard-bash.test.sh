#!/bin/sh
# The Bash guard's cases: each line is what guard-bash.sh must do with the command after the tab. Run by CI's check job;
# no command here runs, the hook only reads it.
set -eu
hook="$(dirname "$0")/guard-bash.sh"
command -v jq >/dev/null 2>&1 || { echo "guard-bash.test: needs jq"; exit 1; }
bad=0
while IFS='	' read -r want cmd; do
  [ -n "$want" ] || continue
  code=0
  printf '%s' "$cmd" | jq -Rs '{tool_input: {command: .}}' | sh "$hook" >/dev/null 2>&1 || code=$?
  case $code in 0) got=allow ;; 2) got=refuse ;; *) got="exit $code" ;; esac
  [ "$got" = "$want" ] || { echo "guard-bash.test: wanted $want, got $got: $cmd"; bad=1; }
done <<'CASES'
refuse	herdr server stop
refuse	HOME=/tmp/x herdr server stop
allow	HERDR_SOCKET_PATH=/tmp/hs/herdr.sock herdr server stop
refuse	herdr terminal attach w1:p1 --takeover
refuse	remuda-core server stop
refuse	rc server stop
refuse	~/.local/bin/remuda-core server stop
refuse	"/Users/me/.local/bin/remuda-core" server stop
refuse	'remuda-core' serve
refuse	remuda-core "server" "stop"
refuse	remuda-core serve
refuse	rc  serve &
refuse	HOME=/tmp/x remuda-core serve
refuse	ls; remuda-core server   stop
refuse	rc --json server stop
refuse	remuda-core server --json stop
refuse	remuda-core serve;
refuse	remuda-core serve>log
refuse	RC_SESSION=scratch remuda-core server stop
refuse	RC_SESSION=e2e XDG_STATE_HOME=/tmp/e/s rc serve
refuse	RC_SOCKET= remuda-core serve
refuse	RC_SOCKET="" rc server stop
refuse	echo RC_SOCKET=x; rc server stop
refuse	RC_SOCKET=/tmp/e/rc.sock true && remuda-core server stop
refuse	RC_SOCKET="$scratch_socket" remuda-core server stop
refuse	RC_SOCKET=$(mktemp -u) rc serve
refuse	RC_SOCKET=`mktemp -u` rc serve
refuse	RC_SOCKET=~/rc.sock rc server stop
refuse	RC_SOCKET=rc.sock rc server stop
refuse	rc server stop # RC_SOCKET=/tmp/e/rc.sock
refuse	rc server stop RC_SOCKET=/tmp/e/rc.sock
refuse	RC_SOCKET=/tmp/e/rc.sock RC_SOCKET= rc server stop
refuse	LABEL="test RC_SOCKET=/tmp/e/rc.sock" remuda-core server stop
refuse	if true; then RC_SOCKET=/tmp/e/rc.sock true; rc server stop; fi
refuse	RC_SOCKET=/tmp/e/rc.sock remuda-core server stop
refuse	RC_SOCKET="/tmp/e/rc.sock" remuda-core server stop
refuse	RC_SOCKET=/tmp/e/rc.sock HOME=/tmp/e "/opt/rc/remuda-core" serve
refuse	env RC_SOCKET=/tmp/e/rc.sock RC_SESSION=e2e rc --json server stop
refuse	RC_SOCKET=/tmp/e/rc.sock remuda-core serve >/tmp/e/core.log 2>&1 &
allow	remuda-core server status
allow	remuda-core --version
allow	remuda-core serve-docs
allow	src server stop
allow	herdr-rc server stop
allow	echo rc serves
allow	go test ./internal/core/...
allow	git status; ls *.go
CASES
[ "$bad" = 0 ] && echo "guard-bash.test: ok"
exit "$bad"
