package ghissues

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/teleport"
)

// The repositories a machine's checkouts belong to, by their origin, and the branches a checkout links to an issue
// (git config branch.<name>.remuda-issue owner/repo#N), read in one script for every folder asked about.

// LinkKey is the git config entry, per branch, that names the issue the branch works on.
const LinkKey = "remuda-issue"

// Repo is a GitHub repository with the folders of the machine that are checkouts of it.
type Repo struct {
	Repo   string   `json:"repo"`   // owner/name
	Folder string   `json:"folder"` // the main checkout, as git names it
	Dirs   []string `json:"dirs"`   // the folders asked about that are checkouts of it, as they were asked
	Links  []Link   `json:"links"`
}

// Link is a branch a checkout's config ties to an issue of the repository.
type Link struct {
	Branch string `json:"branch"`
	Number int    `json:"number"`
}

// Here is what one folder is: a checkout of a GitHub repository, or why not.
type Here struct {
	Dir    string `json:"dir"`
	Repo   string `json:"repo,omitempty"`
	Folder string `json:"folder,omitempty"`
	Why    string `json:"why,omitempty"`  // no-folder, no-git, no-origin, not-github
	Host   string `json:"host,omitempty"` // where a not-github origin is
}

// Repos is a machine's GitHub repositories, and what the folder asked about is.
type Repos struct {
	Repos []Repo `json:"repos"`
	Here  *Here  `json:"here,omitempty"`
}

// GitHubRepo is the owner/name a remote's address names on github.com; false for another host or no address.
func GitHubRepo(remote string) (repo, host string, ok bool) {
	key := teleport.RemoteKey(remote)
	if key == "" {
		return "", "", false
	}
	host, rest, found := strings.Cut(key, "/")
	if !found {
		return "", "", false
	}
	host = strings.TrimPrefix(host, "www.")
	if host != "github.com" {
		return "", host, false
	}
	if !RepoOK(rest) {
		return "", host, false
	}
	return rest, host, true
}

const git = "git --no-optional-locks -c core.fsmonitor=false"

// originsScript prints, for each folder, a D line — the folder, its checkout's top, its repository's common git dir and
// its origin's address — and an L line for each branch its config links to an issue. A folder gone or outside git has
// its D line alone.
func originsScript(dirs []string) string {
	var b strings.Builder
	b.WriteString(script.PATH + "\n")
	b.WriteString("for d in")
	for _, d := range dirs {
		b.WriteString(" " + script.Quote(d))
	}
	b.WriteString("; do\n")
	b.WriteString("  cd \"$d\" 2>/dev/null || { printf 'D\\037%s\\037-\\n' \"$d\"; continue; }\n")
	b.WriteString("  top=$(" + git + " rev-parse --show-toplevel 2>/dev/null) || { printf 'D\\037%s\\037\\n' \"$d\"; continue; }\n")
	b.WriteString("  common=$(cd \"$(" + git + " rev-parse --git-common-dir 2>/dev/null)\" 2>/dev/null && pwd -P)\n")
	b.WriteString("  origin=$(" + git + " config --get remote.origin.url 2>/dev/null)\n")
	b.WriteString("  printf 'D\\037%s\\037%s\\037%s\\037%s\\n' \"$d\" \"$top\" \"$common\" \"$origin\"\n")
	b.WriteString("  " + git + " config --get-regexp '^branch\\..*\\." + LinkKey + "$' 2>/dev/null | while IFS= read -r line; do printf 'L\\037%s\\037%s\\n' \"$d\" \"$line\"; done\n")
	b.WriteString("done\n")
	return b.String()
}

type checkout struct {
	dir, top, common, origin string
	gone                     bool
	links                    []string // the config's lines: branch.<name>.remuda-issue <value>
}

func parseOrigins(out string) map[string]*checkout {
	byDir := map[string]*checkout{}
	for _, line := range strings.Split(out, "\n") {
		f := script.Fields(strings.TrimRight(line, "\r"))
		switch {
		case len(f) >= 2 && f[0] == "D":
			c := &checkout{dir: f[1]}
			if len(f) == 3 && f[2] == "-" {
				c.gone = true
			}
			if len(f) >= 5 {
				c.top, c.common, c.origin = f[2], f[3], f[4]
			}
			byDir[c.dir] = c
		case len(f) == 3 && f[0] == "L":
			if c := byDir[f[1]]; c != nil {
				c.links = append(c.links, f[2])
			}
		}
	}
	return byDir
}

// mainFolder is the repository's main checkout: the folder its common .git sits in, else the checkout's own top.
func (c *checkout) mainFolder(style rpath.Style) string {
	if c.common != "" && path.Base(strings.ReplaceAll(c.common, "\\", "/")) == ".git" {
		return style.Clean(style.Dir(style.Clean(c.common)))
	}
	return style.Clean(c.top)
}

// linksOf reads the config's lines for the branches that name an issue of this repository: owner/repo#N, or #N and N
// for its own, several comma-separated.
func linksOf(lines []string, repo string) []Link {
	var out []Link
	for _, line := range lines {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		branch := strings.TrimSuffix(strings.TrimPrefix(key, "branch."), "."+LinkKey)
		if branch == key || branch == "" {
			continue
		}
		for _, ref := range strings.Split(value, ",") {
			ref = strings.TrimSpace(ref)
			owner, number, found := strings.Cut(ref, "#")
			if !found {
				owner, number = "", ref
			}
			if owner != "" && !strings.EqualFold(owner, repo) {
				continue
			}
			if n, err := strconv.Atoi(number); err == nil && n > 0 {
				out = append(out, Link{Branch: branch, Number: n})
			}
		}
	}
	return out
}

// reposOf groups the folders read by the GitHub repository their origin names; here is the one folder asked about.
func reposOf(byDir map[string]*checkout, dirs []string, style rpath.Style, asked string) Repos {
	answer := Repos{Repos: []Repo{}}
	index := map[string]int{}
	for _, d := range dirs {
		c := byDir[d]
		var here *Here
		if d == asked && asked != "" {
			here = &Here{Dir: d}
			answer.Here = here
		}
		switch {
		case c == nil || c.gone:
			if here != nil {
				here.Why = "no-folder"
			}
			continue
		case c.top == "":
			if here != nil {
				here.Why = "no-git"
			}
			continue
		}
		repo, host, ok := GitHubRepo(c.origin)
		folder := c.mainFolder(style)
		if !ok {
			if here != nil {
				here.Folder = folder
				if c.origin == "" {
					here.Why = "no-origin"
				} else {
					here.Why, here.Host = "not-github", host
				}
			}
			continue
		}
		if here != nil {
			here.Repo, here.Folder = repo, folder
		}
		key := strings.ToLower(repo)
		at, seen := index[key]
		if !seen {
			at = len(answer.Repos)
			index[key] = at
			answer.Repos = append(answer.Repos, Repo{Repo: repo, Folder: folder, Dirs: []string{}, Links: []Link{}})
		}
		r := &answer.Repos[at]
		r.Dirs = append(r.Dirs, d)
		for _, l := range linksOf(c.links, repo) {
			if !hasLink(r.Links, l) {
				r.Links = append(r.Links, l)
			}
		}
	}
	sort.SliceStable(answer.Repos, func(i, j int) bool {
		return strings.ToLower(answer.Repos[i].Repo) < strings.ToLower(answer.Repos[j].Repo)
	})
	return answer
}

func hasLink(links []Link, l Link) bool {
	for _, x := range links {
		if x == l {
			return true
		}
	}
	return false
}
