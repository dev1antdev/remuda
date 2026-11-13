package repos

// The New worktree dialog's read of a repository, in one script on its machine: its local branches and origin's, the
// checkouts git keeps for it, the branch a new one starts from, origin's address and the team's setup script. Every
// git call is a read that touches no index (--no-optional-locks, fsmonitor off).

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/teleport"
)

// ReadTimeout is how long a repository's read may take; FetchTimeout a fetch from origin.
var (
	ReadTimeout  = 30 * time.Second
	FetchTimeout = 10 * time.Minute
)

// Branch is a branch and when it was last committed to (unix seconds).
type Branch struct {
	Name string `json:"name"`
	At   int64  `json:"at"`
}

// Checkout is a folder git keeps checked out for the repository; Main is its main checkout.
type Checkout struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"` // "" when detached
	Main   bool   `json:"main,omitempty"`
}

// Team is the repository's own setup script, from the first of TeamFiles that has one.
type Team struct {
	File   string `json:"file"`
	Script string `json:"script"`
}

// Source is what a repository offers a new worktree.
type Source struct {
	Root      string     `json:"root"`   // the main checkout
	Name      string     `json:"name"`   // its folder's name, herdr's repo_name
	Origin    string     `json:"origin"` // origin's address with no user or token in it, "" without one
	GitHub    string     `json:"github"` // owner/repo when origin is on GitHub
	Base      string     `json:"base"`   // where a new branch starts by default: origin's HEAD, else the main checkout's branch
	Local     []Branch   `json:"local"`  // newest commit first
	Remote    []Branch   `json:"remote"` // origin's, as of the last fetch, without origin/HEAD and named without "origin/"
	Checkouts []Checkout `json:"checkouts"`
	Team      *Team      `json:"team,omitempty"`
}

// TeamFiles are the files a repository keeps its setup script in: herdr's own, else Orca's, of the same format.
var TeamFiles = []string{"herdr.yaml", "orca.yaml"}

// Git is git as remuda's reads run it: no lock taken, no fsmonitor started.
const Git = "git --no-optional-locks -c core.fsmonitor=false"

// SourceScript reads the repository whose main checkout (or any checkout) is the folder.
func SourceScript(folder string) string {
	var b strings.Builder
	b.WriteString(script.PATH + "\ncd " + script.Quote(folder) + " 2>/dev/null || { echo 'the folder is not there'; exit 3; }\n")
	b.WriteString("common=$(" + Git + " rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || { echo 'not a git repository'; exit 3; }\n")
	b.WriteString("case \"$common\" in */.git) root=${common%/.git} ;; *) root=$(" + Git + " rev-parse --show-toplevel) ;; esac\n")
	b.WriteString("cd \"$root\" || exit 3\nprintf '%s\\n' \"$root\"\nprintf '\\036\\n'\n")
	b.WriteString(Git + " for-each-ref --sort=-committerdate --format='%(refname)%09%(committerdate:unix)' refs/heads refs/remotes/origin\nprintf '\\036\\n'\n")
	b.WriteString(Git + " worktree list --porcelain\nprintf '\\036\\n'\n")
	b.WriteString("printf '%s\\n' \"$(" + Git + " symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null)\"\n")
	b.WriteString("printf '%s\\n' \"$(" + Git + " branch --show-current 2>/dev/null)\"\nprintf '\\036\\n'\n")
	b.WriteString(Git + " remote get-url origin 2>/dev/null | sed -E 's#^([a-z+]+://)[^/@]*@#\\1#'\nprintf '\\036\\n'\n")
	b.WriteString("for f in")
	for _, f := range TeamFiles {
		b.WriteString(" " + script.Quote(f))
	}
	b.WriteString("; do if [ -f \"$f\" ] && [ ! -L \"$f\" ]; then printf '%s\\n' \"$f\"; head -c 65536 \"$f\"; break; fi; done\n")
	return b.String()
}

// ParseSource reads the script's answer; paths in the machine's own spelling.
func ParseSource(out string) (Source, error) {
	parts := script.Sections(out)
	if len(parts) < 6 {
		return Source{}, errors.New(lastLine(out, "git answered nothing"))
	}
	root := strings.TrimSpace(parts[0])
	style := rpath.For(root)
	src := Source{Root: style.Clean(root), Local: []Branch{}, Remote: []Branch{}, Checkouts: []Checkout{}}
	src.Name = style.Base(src.Root)
	for _, line := range script.Lines(parts[1]) {
		ref, at, _ := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		when, _ := strconv.ParseInt(strings.TrimSpace(at), 10, 64)
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			src.Local = append(src.Local, Branch{Name: strings.TrimPrefix(ref, "refs/heads/"), At: when})
		case strings.HasPrefix(ref, "refs/remotes/origin/") && ref != "refs/remotes/origin/HEAD":
			src.Remote = append(src.Remote, Branch{Name: strings.TrimPrefix(ref, "refs/remotes/origin/"), At: when})
		}
	}
	src.Checkouts = parseWorktrees(style, parts[2])
	heads := strings.Split(strings.ReplaceAll(parts[3], "\r", ""), "\n")
	originHead, current := "", ""
	if len(heads) > 0 {
		originHead = strings.TrimSpace(heads[0])
	}
	if len(heads) > 1 {
		current = strings.TrimSpace(heads[1])
	}
	switch {
	case originHead != "":
		src.Base = originHead
	case current != "":
		src.Base = current
	default:
		src.Base = "HEAD"
	}
	src.Origin = strings.TrimSpace(parts[4])
	src.GitHub = GitHubRepo(src.Origin)
	team := strings.SplitN(parts[5], "\n", 2)
	if len(team) == 2 && strings.TrimSpace(team[0]) != "" {
		if body := YAMLScript(team[1], "setup"); body != "" {
			src.Team = &Team{File: strings.TrimSpace(team[0]), Script: body}
		}
	}
	return src, nil
}

// parseWorktrees reads git worktree list --porcelain: the main checkout first, a bare or prunable entry left out.
func parseWorktrees(style rpath.Style, text string) []Checkout {
	out := []Checkout{}
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n\n") {
		var c Checkout
		skip := false
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				c.Path = style.Clean(strings.TrimPrefix(line, "worktree "))
			case strings.HasPrefix(line, "branch "):
				c.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
			case line == "bare" || strings.HasPrefix(line, "prunable"):
				skip = true
			}
		}
		if c.Path == "" || skip {
			continue
		}
		c.Main = len(out) == 0
		out = append(out, c)
	}
	return out
}

// GitHubRepo is owner/repo of a remote on github.com, "" for any other.
func GitHubRepo(remote string) string {
	key := teleport.RemoteKey(remote)
	rest, ok := strings.CutPrefix(key, "github.com/")
	if !ok || strings.Count(rest, "/") != 1 {
		return ""
	}
	return rest
}

// Read is the repository the folder is in, read on its machine.
func Read(host shell.Shell, folder string) (Source, error) {
	out, err := shell.Text(host, SourceScript(folder), ReadTimeout)
	if err != nil {
		return Source{}, errors.New(lastLine(out, err.Error()))
	}
	return ParseSource(out)
}

// FetchScript is git fetch --prune origin in the repository: new remote branches show, gone ones go; no checkout moves.
func FetchScript(root string) string {
	return script.PATH + "\ncd " + script.Quote(root) + " 2>/dev/null || { echo 'the repository is not there'; exit 3; }\n" +
		"git fetch --prune origin 2>&1\n"
}

// Fetch runs it; the error is git's own words.
func Fetch(host shell.Shell, root string) error {
	out, err := shell.Text(host, FetchScript(root), FetchTimeout)
	if err != nil {
		return errors.New(lastLine(out, err.Error()))
	}
	return nil
}

// lastLine is a tool's complaint: the last line it printed, else the fallback.
func lastLine(text, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	if i := strings.LastIndex(text, "\n"); i >= 0 {
		text = text[i+1:]
	}
	return strings.TrimSpace(text)
}
