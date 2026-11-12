package review

import (
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/secrets"
)

// SettingsAudit is what the lens says of one of Claude Code's settings files in the folder or its repository's root:
// whether git tracks or ignores it, and what in it looks like a secret — an allowed command that carries a token.
type SettingsAudit struct {
	Path     string            `json:"path"` // relative to the folder, or absolute for the root's
	Local    bool              `json:"local,omitempty"`
	Tracked  bool              `json:"tracked,omitempty"`
	Ignored  bool              `json:"ignored,omitempty"`
	Findings []secrets.Finding `json:"findings,omitempty"`
}

// settingsLocal is the file Claude Code keeps the person's own approvals in, never meant for a commit.
const settingsLocal = "settings.local.json"

// maxSettings is how much of a settings file is read: they are a few kilobytes.
const maxSettings = 65536

// settingsScript prints, for each of .claude/settings.local.json and .claude/settings.json in the folder and in the
// repository's root when that is another folder, its path, whether git tracks it and whether it ignores it, then the
// file itself and a line holding GS alone.
var settingsScript = `for d in . "$root"; do if [ "$d" = "$root" ] && [ "$(pwd -P)" = "$root" ]; then continue; fi; ` +
	`for n in ` + settingsLocal + ` settings.json; do p="$d/.claude/$n"; [ -f "$p" ] || continue; ` +
	`t=no; git ls-files --error-unmatch -- "$p" >/dev/null 2>&1 && t=yes; ` +
	`i=no; git check-ignore -q -- "$p" 2>/dev/null && i=yes; ` +
	`printf '%s\037%s\037%s\n' "$p" "$t" "$i"; head -c ` + strconv.Itoa(maxSettings) + ` "$p"; printf '\n\035\n'; done; done`

// readSettings reads settingsScript's answer.
func readSettings(section string) []SettingsAudit {
	var audits []SettingsAudit
	for _, record := range strings.Split(section, "\n"+script.GS+"\n") {
		head, content, ok := strings.Cut(record, "\n")
		fields := script.Fields(head)
		if !ok || len(fields) != 3 {
			continue
		}
		path := strings.TrimPrefix(fields[0], "./")
		if len(content) > maxSettings {
			content = content[:maxSettings]
		}
		audits = append(audits, SettingsAudit{
			Path:     path,
			Local:    strings.HasSuffix(path, "/"+settingsLocal),
			Tracked:  fields[1] == "yes",
			Ignored:  fields[2] == "yes",
			Findings: secrets.Scan(path, content),
		})
	}
	return audits
}
