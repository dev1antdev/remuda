package coretest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// StandInClaude puts a `claude` on the PATH the next core starts with, in Claude Code's ways that remuda reads: it keeps
// its session file, sessions/<pid>.json under $HOME/.claude, when given a session, and moves it to another on
// "/resume <id>", in the same process; says what it was started with ("args: …"); reports itself idle through OSC 7501;
// answers /restart as a build without it does; leaves on /exit; and echoes any other line ("got: …").
func StandInClaude(t testing.TB, session string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
keep() {
  mkdir -p "$HOME/.claude/sessions"
  printf '{"pid":%s,"sessionId":"%s","cwd":"%s"}' $$ "$1" "$(pwd -P)" > "$HOME/.claude/sessions/$$.json"
}
[ -n "SESSION" ] && keep SESSION
printf 'args: %s\n' "$*"
printf '\033]7501;state=idle:app=claude-code\033\\'
while IFS= read -r line; do
  case "$line" in
  /exit) exit 0 ;;
  /restart) printf '❯ /restart\n  ⎿  Unknown slash command: restart\n' ;;
  "/resume "*) keep "${line#/resume }"; printf 'resumed: %s\n' "${line#/resume }" ;;
  *) printf 'got: %s\n' "$line" ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(strings.ReplaceAll(script, "SESSION", session)), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
