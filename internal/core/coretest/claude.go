package coretest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// StandInClaude puts on the next core's PATH a `claude` that keeps its session file as Claude Code does and echoes lines.
func StandInClaude(t testing.TB, session string) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
keep() {
  mkdir -p "$HOME/.claude/sessions"
  start=$(LC_ALL=C TZ=UTC ps -o lstart= -p $$ | sed 's/ *$//')
  began=$(( $(date +%s) + 1 ))000 # Claude Code's Date.now() as it begins: never before its process, to the ms
  printf '{"pid":%s,"sessionId":"%s","cwd":"%s","procStart":"%s","startedAt":%s}' $$ "$1" "$(pwd -P)" "$start" "$began" > "$HOME/.claude/sessions/$$.json"
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
