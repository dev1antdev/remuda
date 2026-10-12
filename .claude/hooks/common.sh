# Shared by the hooks in this folder: the tool call arrives as JSON on
# stdin; `field .tool_input.command` reads one string out of it with jq,
# else python3, else nothing — a hook that cannot read its input lets the
# call through rather than break every tool call on a machine without jq.
input=$(cat)

field() {
  if command -v jq >/dev/null 2>&1; then
    printf '%s' "$input" | jq -r "$1 // empty" 2>/dev/null
  elif command -v python3 >/dev/null 2>&1; then
    printf '%s' "$input" | python3 -I -c '
import json, sys
d = json.load(sys.stdin)
for k in sys.argv[1].strip(".").split("."):
    d = d.get(k, "") if isinstance(d, dict) else ""
print(d if isinstance(d, str) else "")' "$1" 2>/dev/null
  fi
}
