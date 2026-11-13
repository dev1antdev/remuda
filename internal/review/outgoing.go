package review

import (
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// maxOutgoing is how many commits not on the remote a summary lists; the rest are counted.
const maxOutgoing = 100

// Outgoing is what the branch has that the remote does not, as of the last fetch: the commits HEAD has over its
// upstream, or over the remote's default branch where it left it when there is no upstream, and how many the upstream
// has that HEAD lacks. Nothing fetches.
type Outgoing struct {
	Against  string      `json:"against"`            // origin/feature, or origin/main for a branch never pushed
	Upstream bool        `json:"upstream,omitempty"` // Against is the branch's upstream
	Commits  []OutCommit `json:"commits"`
	Total    int         `json:"total"`
	Behind   int         `json:"behind,omitempty"` // the upstream's commits HEAD lacks: what a pull would bring
}

// OutCommit is one commit not on the remote.
type OutCommit struct {
	SHA      string `json:"sha"`
	Short    string `json:"short"`
	Author   string `json:"author"`
	WhenUnix int64  `json:"when"`
	Subject  string `json:"subject"`
}

// outgoingScript prints the ref compared with and whether it is the upstream, how many commits HEAD has over it, how many
// it has over HEAD (the upstream only), then the newest maxOutgoing of HEAD's own, one a line.
var outgoingScript = `a=; n=$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null); up=1; ` +
	`if [ -n "$n" ]; then a='@{u}'; else up=0; n=$(git symbolic-ref --short -q refs/remotes/origin/HEAD 2>/dev/null); ` +
	`for c in origin/main origin/master; do [ -n "$n" ] && break; git rev-parse --verify -q "refs/remotes/$c" >/dev/null 2>&1 && n=$c; done; ` +
	`[ -n "$n" ] && a=$(git merge-base HEAD "$n" 2>/dev/null); fi; ` +
	`if [ -n "$a" ]; then printf '%s\037%s\n' "$n" "$up"; git rev-list --count "$a"..HEAD 2>/dev/null || echo 0; ` +
	`if [ "$up" = 1 ]; then git rev-list --count HEAD.."$a" 2>/dev/null || echo 0; else echo 0; fi; ` +
	`git log -n ` + strconv.Itoa(maxOutgoing) + ` --format='%H%x1f%h%x1f%an%x1f%ct%x1f%s' "$a"..HEAD 2>/dev/null; fi`

// readOutgoing reads outgoingScript's answer; nil when there is no remote to compare with.
func readOutgoing(section string) *Outgoing {
	lines := script.Lines(section)
	if len(lines) < 3 {
		return nil
	}
	head := script.Fields(lines[0])
	if len(head) != 2 || head[0] == "" {
		return nil
	}
	total, _ := strconv.Atoi(strings.TrimSpace(lines[1]))
	behind, _ := strconv.Atoi(strings.TrimSpace(lines[2]))
	outgoing := &Outgoing{Against: head[0], Upstream: head[1] == "1", Commits: []OutCommit{}, Total: total, Behind: behind}
	for _, line := range lines[3:] {
		fields := script.Fields(line)
		if len(fields) != 5 {
			continue
		}
		when, _ := strconv.ParseInt(fields[3], 10, 64)
		outgoing.Commits = append(outgoing.Commits, OutCommit{SHA: fields[0], Short: fields[1], Author: fields[2], WhenUnix: when, Subject: fields[4]})
	}
	return outgoing
}
