#!/bin/sh
# Claude Code PostToolUse hook for Edit / Write: a Go file that just changed
# is gofmt'ed in place, and the agent is told so — its next edit of that file
# has to read it again. Nothing happens for other files, or when gofmt is
# missing, or when the file is already formatted (the usual case).
. "$(dirname "$0")/common.sh"

f=$(field .tool_input.file_path)
case "$f" in
  *.go) ;;
  *) exit 0 ;;
esac
[ -f "$f" ] || exit 0
command -v gofmt >/dev/null 2>&1 || exit 0
[ -n "$(gofmt -l "$f" 2>/dev/null)" ] || exit 0
gofmt -w "$f" 2>/dev/null || exit 0

msg="gofmt reformatted $f after your edit; read it again before editing it further."
if command -v jq >/dev/null 2>&1; then
  jq -n --arg m "$msg" '{hookSpecificOutput: {hookEventName: "PostToolUse", additionalContext: $m}}'
else
  printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"%s"}}\n' "$msg"
fi
exit 0
