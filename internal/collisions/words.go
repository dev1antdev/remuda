package collisions

import (
	"fmt"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// whyWords is a reason one work lands first, as the push says it.
var whyWords = map[string]string{
	WhyPR:      "it has a pull request open",
	WhyRest:    "its agents have finished",
	WhySmaller: "its change is the smaller one",
}

// Words is the push for two works whose lines touch: who and where, then which lands first and why.
func Words(first, second string, collision Collision) (string, string) {
	title := fmt.Sprintf("%s and %s change the same lines", first, second)
	file := collision.Files[0]
	where := file.Path
	switch {
	case file.From == 0:
		where += " (the whole file)"
	case file.From == file.To:
		where += fmt.Sprintf(", line %d", file.From)
	default:
		where += fmt.Sprintf(", lines %d–%d", file.From, file.To)
	}
	if others := len(collision.Files) - 1 + collision.More; others == 1 {
		where += ", and 1 more file"
	} else if others > 1 {
		where += fmt.Sprintf(", and %d more files", others)
	}
	body := where + ". Land " + first + " first"
	if why := whyWords[collision.Why]; why != "" {
		body += ": " + why
	}
	return title, body + "; " + second + " rebases on it."
}

// Lookup is what the radar needs of a machine beside its snapshot: the repository a folder belongs to, its
// uncommitted files and branch as the review sweep read them, and whether a pane's agent named a pull request still
// open.
type Lookup struct {
	Repo    func(dir string) string
	Changed func(dir string) int
	Branch  func(dir string) string
	OpenPR  func(paneID string) bool
}

// WorkspacesOf lists a machine's workspaces as the radar weighs them.
func WorkspacesOf(machineID string, snap *herdr.Snapshot, lookup Lookup) []Workspace {
	var out []Workspace
	for i := range snap.Workspaces {
		ws := &snap.Workspaces[i]
		dir := snap.WorkspaceDir(ws)
		if dir == "" {
			continue
		}
		entry := Workspace{Machine: machineID, ID: ws.WorkspaceID, Label: ws.Label, Dir: dir, Repo: lookup.Repo(dir), Branch: lookup.Branch(dir), Changed: lookup.Changed(dir)}
		for j := range snap.Panes {
			pane := &snap.Panes[j]
			if pane.WorkspaceID != ws.WorkspaceID || pane.Agent == "" {
				continue
			}
			if !entry.Agents {
				entry.Agents, entry.Pane = true, pane.PaneID
			}
			entry.Working = entry.Working || pane.AgentStatus == herdr.StatusWorking
			entry.PR = entry.PR || lookup.OpenPR(pane.PaneID)
		}
		out = append(out, entry)
	}
	return out
}
