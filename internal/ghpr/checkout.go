package ghpr

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// Remote is one of the checkout's remotes and the GitHub repository it names; Repo is "" for one that is not GitHub's.
type Remote struct {
	Name string `json:"name"`
	Repo string `json:"repo,omitempty"`
}

// Checkout is what the panel needs of a folder: its branch, its remotes, and the repository its Repo list reads —
// upstream, else the branch's tracking remote, else origin, else the first GitHub remote.
type Checkout struct {
	Branch  string   `json:"branch"`
	Remotes []Remote `json:"remotes"`
	Repo    string   `json:"repo"`
}

// gitRead is how every git read here starts: no optional lock, no file-system monitor, so a read never touches the index.
const gitRead = "git --no-optional-locks -c core.fsmonitor=false"

// CheckoutScript reads the folder's branch, its branch's tracking remote and every remote's address, one a line.
func CheckoutScript(dir string) string {
	return script.InDir(dir) + `
b=$(` + gitRead + ` symbolic-ref --quiet --short HEAD 2>/dev/null)
printf 'branch %s\n' "$b"
[ -n "$b" ] && printf 'tracking %s\n' "$(` + gitRead + ` config --get "branch.$b.remote" 2>/dev/null)"
` + gitRead + ` remote 2>/dev/null | while IFS= read -r r; do printf 'remote %s %s\n' "$r" "$(` + gitRead + ` remote get-url "$r" 2>/dev/null)"; done`
}

// ReadCheckout reads the folder on its machine.
func ReadCheckout(host shell.Shell, dir string) (Checkout, error) {
	out, err := shell.Text(host, CheckoutScript(dir), 15*time.Second)
	if err != nil && strings.TrimSpace(out) == "" {
		return Checkout{}, err
	}
	return ParseCheckout(out), nil
}

// ParseCheckout reads CheckoutScript's lines.
func ParseCheckout(out string) Checkout {
	var c Checkout
	tracking := ""
	for _, line := range strings.Split(out, "\n") {
		word, rest, _ := strings.Cut(line, " ")
		switch word {
		case "branch":
			c.Branch = strings.TrimSpace(rest)
		case "tracking":
			tracking = strings.TrimSpace(rest)
		case "remote":
			name, url, _ := strings.Cut(rest, " ")
			if name == "" {
				continue
			}
			c.Remotes = append(c.Remotes, Remote{Name: name, Repo: RepoOfRemote(url)})
		}
	}
	sort.Slice(c.Remotes, func(i, j int) bool { return c.Remotes[i].Name < c.Remotes[j].Name })
	c.Repo = preferred(c.Remotes, tracking)
	return c
}

func preferred(remotes []Remote, tracking string) string {
	for _, name := range []string{"upstream", tracking, "origin"} {
		for _, r := range remotes {
			if name != "" && r.Name == name && r.Repo != "" {
				return r.Repo
			}
		}
	}
	for _, r := range remotes {
		if r.Repo != "" {
			return r.Repo
		}
	}
	return ""
}

var remoteURL = regexp.MustCompile(`^(?:(?:https?|git|ssh)://(?:[^@/]+@)?([^/:]+)(?::[0-9]+)?/|[^@/:]+@([^:/]+):)(.+?)(?:\.git)?/?$`)

// RepoOfRemote is the GitHub repository a remote's address names, "" when it is not github.com's; an ssh alias such as
// github-work or ssh.github.com counts as github.com.
func RepoOfRemote(url string) string {
	m := remoteURL.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return ""
	}
	host := strings.ToLower(m[1] + m[2])
	if strings.HasPrefix(host, "github-") || strings.HasPrefix(host, "github.com-") || host == "ssh.github.com" {
		host = "github.com"
	}
	repo, err := ParseRepo(m[3])
	if host != "github.com" || err != nil {
		return ""
	}
	return repo.String()
}
