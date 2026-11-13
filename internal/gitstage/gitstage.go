// Package gitstage is the Changes lens's git writes, each at the person's press: stage, unstage and discard a file,
// commit what is staged and amend the last commit. A write is refused while an agent works, asks or ends a turn in the
// checkout, never runs two at once in one checkout, skips a conflicted file in a bulk action, and answers git's own
// words when git refuses. Nothing is forced and nothing is cleaned past the one file the person picked.
package gitstage

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// chunk is how many paths one git command takes, well under any system's argument limit.
const chunk = 200

// writeGit is how a write calls git: names read literally, so a file called "[a].txt" or "*.go" is that file.
const writeGit = `git --literal-pathspecs -c core.quotepath=off`

// Service runs the writes. Busy names the agent at work, asking or ending a turn in the checkout — the folder the lens
// shows, and its top folder as git names it in the machine's own spelling — "" when none: a write would land under it.
type Service struct {
	Busy func(machineID, dir, top string) string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// Refusal is a write remuda will not run, and why; Paths names the files it is about.
type Refusal struct {
	Why   string   `json:"error"`
	Paths []string `json:"paths,omitempty"`
}

func (r *Refusal) Error() string { return r.Why }

// GitError is git's own refusal: its words, which the page shows as they are.
type GitError struct{ Output string }

func (e *GitError) Error() string { return e.Output }

// IsRefusal reports whether err is remuda's own refusal rather than git's or the machine's.
func IsRefusal(err error) bool {
	var refusal *Refusal
	return errors.As(err, &refusal)
}

// checkout is what a write learns of the folder before it runs: its repository's top folder, whether there is a commit
// yet, and the files in conflict, relative to the folder.
type checkout struct {
	top       string
	hasHead   bool
	conflicts map[string]bool
}

// probeScript prints the checkout's top folder, whether HEAD names a commit, and the unmerged files relative to the
// folder, NUL-separated.
const probeScript = `top=$(git rev-parse --show-toplevel 2>/dev/null) || { echo none; exit 0; }; printf '%s\n' "$top"; ` +
	`if git rev-parse -q --verify HEAD >/dev/null 2>&1; then echo head; else echo nohead; fi; ` +
	`git --no-optional-locks -c core.quotepath=off ls-files -u -z -- . 2>/dev/null | tr '\0' '\n' | cut -f2- | ` +
	`while IFS= read -r p; do printf '%s\0' "$p"; done`

// prepare reads the checkout and refuses while an agent is busy there, holding the checkout's lock on success: the
// caller releases it.
func (s *Service) prepare(host shell.Host, dir string) (checkout, func(), error) {
	if !rpath.For(host.Home()).IsAbs(dir) {
		return checkout{}, nil, &Refusal{Why: "an absolute folder is required"}
	}
	output, err := shell.Text(host, script.InDir(dir)+probeScript, 30*time.Second)
	if err != nil {
		return checkout{}, nil, err
	}
	top, rest, _ := strings.Cut(output, "\n")
	if top == "none" || top == "" {
		return checkout{}, nil, &Refusal{Why: "the folder is not a git checkout"}
	}
	head, conflicted, _ := strings.Cut(rest, "\n")
	found := checkout{top: rpath.For(host.Home()).Clean(top), hasHead: head == "head", conflicts: map[string]bool{}}
	for _, path := range strings.Split(conflicted, "\x00") { // ls-files names them from the folder, as the lens does
		if path != "" {
			found.conflicts[path] = true
		}
	}
	if s.Busy != nil {
		if agent := s.Busy(host.ID(), dir, found.top); agent != "" {
			return checkout{}, nil, &Refusal{Why: agent + " is working in this checkout: wait until it rests, or ask it to stop"}
		}
	}
	lock := s.lock(host.ID() + "|" + found.top)
	if !lock.TryLock() {
		return checkout{}, nil, &Refusal{Why: "another change to this checkout is still running"}
	}
	return found, lock.Unlock, nil
}

func (s *Service) lock(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks == nil {
		s.locks = map[string]*sync.Mutex{}
	}
	if s.locks[key] == nil {
		s.locks[key] = &sync.Mutex{}
	}
	return s.locks[key]
}

// checkPaths refuses what must not reach git as a file of the folder: nothing, an absolute path, one climbing out, an
// option.
func checkPaths(paths []string) error {
	if len(paths) == 0 {
		return &Refusal{Why: "no file was named"}
	}
	for _, path := range paths {
		if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, "-") || strings.Contains(path, "\x00") || strings.Contains(path, "\n") {
			return &Refusal{Why: "a path relative to the folder is required", Paths: []string{path}}
		}
		for _, part := range strings.Split(strings.ReplaceAll(path, `\`, "/"), "/") {
			if part == ".." {
				return &Refusal{Why: "a path inside the folder is required", Paths: []string{path}}
			}
		}
	}
	return nil
}

// quoted is the paths as one shell list.
func quoted(paths []string) string {
	words := make([]string, len(paths))
	for i, path := range paths {
		words[i] = script.Quote(path)
	}
	return strings.Join(words, " ")
}

// inChunks is one command per chunk of paths, each stopping the script at its first failure.
func inChunks(command string, paths []string) string {
	var out strings.Builder
	for start := 0; start < len(paths); start += chunk {
		end := min(start+chunk, len(paths))
		fmt.Fprintf(&out, "%s -- %s || exit 1; ", command, quoted(paths[start:end]))
	}
	return out.String()
}

// run runs a write script with git's words on its output and answers them as a GitError when it fails.
func run(host shell.Host, dir, body string, timeout time.Duration) (string, error) {
	output, err := shell.Text(host, script.InDir(dir)+"exec 2>&1; "+body, timeout)
	if err == nil {
		return output, nil
	}
	var exit *shell.ExitError
	if errors.As(err, &exit) {
		words := strings.TrimSpace(output)
		if words == "" {
			words = exit.Error()
		}
		return output, &GitError{Output: words}
	}
	return output, err
}
