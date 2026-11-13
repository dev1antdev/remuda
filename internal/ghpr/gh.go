// Package ghpr is the pull request panel's side of GitHub: a checkout's pull requests and the repository's, one pull
// request in full — its conversation, its checks with their jobs and logs, its files — and what the person writes there
// at a press (a comment, a reply, a thread resolved, a review asked for again, a merge, a close). Everything goes through
// gh on the pane's machine as the person, the way internal/prstatus reads a request: remuda itself asks GitHub nothing.
package ghpr

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

var (
	// Timeout is how long one gh call may take.
	Timeout = 25 * time.Second
	// LogTimeout is how long a job's log may take: gh fetches the run's log archive first.
	LogTimeout = 90 * time.Second
)

// Problem is a gh that cannot be used at all on the machine: not installed, or not signed in. Nothing in the panel
// works until it is fixed, so the page shows it in place of everything.
type Problem struct {
	Kind    string `json:"kind"` // missing, signed-out
	Machine string `json:"machine"`
	Command string `json:"command"` // what to run there
}

func (p *Problem) Error() string {
	if p.Kind == "missing" {
		return "the GitHub CLI (gh) is not installed on " + p.Machine
	}
	return "the GitHub CLI is not signed in on " + p.Machine
}

// AsProblem is the Problem an error carries, if it is one.
func AsProblem(err error) (*Problem, bool) {
	var p *Problem
	ok := errors.As(err, &p)
	return p, ok
}

// answer is what one gh call printed: its stdout, its stderr and its exit status.
type answer struct {
	out  string
	err  string
	code int
}

// missingMark is what the script prints when the machine has no gh.
const missingMark = "\x1enogh"

// ghScript runs gh with the arguments on the machine, its stdout, stderr and exit status printed apart; input, when
// given, goes on its stdin through a here-document (one line of JSON, which never holds the delimiter on a line of its
// own). keep, when given, is the head or tail command the stdout is cut by on the machine, its output landing in a file
// first: GNU tail -c on a pipe was seen handing over more than it was asked for.
func ghScript(args []string, input string, keep string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = script.Quote(a)
	}
	call := "gh " + strings.Join(quoted, " ") + ` 2>"$err"`
	if keep != "" {
		call = `{ out=$(mktemp) && gh ` + strings.Join(quoted, " ") + ` >"$out" 2>"$err"; code=$?; ` + keep + ` "$out"; rm -f "$out"; (exit $code); }`
	}
	if input != "" {
		call += " <<'REMUDA_INPUT'\n" + input + "\nREMUDA_INPUT"
	}
	return script.PATH + `
command -v gh >/dev/null 2>&1 || { printf '\036nogh\n'; exit 0; }
err=$(mktemp 2>/dev/null || echo "${TMPDIR:-/tmp}/remuda-gh.$$")
` + call + `
code=$?
printf '\036\n'
cat "$err" 2>/dev/null; rm -f "$err"
printf '\036\n%s\n' "$code"`
}

// parseAnswer reads ghScript's output from its end: the exit status, then stderr, and stdout before them.
func parseAnswer(text string) (answer, bool) {
	if strings.HasPrefix(text, missingMark) {
		return answer{}, false
	}
	last := strings.LastIndex(text, script.RS+"\n")
	if last < 0 {
		return answer{out: text, code: -1}, true
	}
	code, err := strconv.Atoi(strings.TrimSpace(text[last+2:]))
	if err != nil {
		code = -1
	}
	head := text[:last]
	before := strings.LastIndex(head, script.RS+"\n")
	if before < 0 {
		return answer{out: head, code: code}, true
	}
	return answer{out: head[:before], err: strings.TrimSpace(head[before+2:]), code: code}, true
}

// run is one gh call on the machine, its stdout when it ended well; a gh not installed or signed out is a Problem,
// anything else gh's own words.
func run(host shell.Shell, args []string, input string) (string, error) {
	return runKept(host, args, input, "", Timeout)
}

// lastBytes and firstBytes are the commands runKept cuts a long stdout by.
func lastBytes(n int) string  { return "tail -c " + strconv.Itoa(n) }
func firstBytes(n int) string { return "head -c " + strconv.Itoa(n) }

func runKept(host shell.Shell, args []string, input, keep string, timeout time.Duration) (string, error) {
	text, err := shell.Text(host, ghScript(args, input, keep), timeout)
	got, ok := parseAnswer(text)
	if !ok {
		return "", &Problem{Kind: "missing", Machine: machineOf(host), Command: "gh auth login"}
	}
	if got.code == 0 {
		return got.out, nil
	}
	if signedOut(got.err) {
		return "", &Problem{Kind: "signed-out", Machine: machineOf(host), Command: "gh auth login"}
	}
	if got.code == -1 && err != nil {
		return "", err
	}
	return got.out, &Failed{Words: ghWords(got.err, got.out)}
}

// Failed is gh's refusal, in its own words.
type Failed struct{ Words string }

func (f *Failed) Error() string { return f.Words }

// machineOf names the machine for a Problem's sentence.
func machineOf(host shell.Shell) string {
	if h, ok := host.(interface{ Label() string }); ok && h.Label() != "" {
		return h.Label()
	}
	return "this machine"
}

func signedOut(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "gh auth login") || strings.Contains(lower, "not logged in") || strings.Contains(lower, "authentication required")
}

// ghWords is what gh said, at most a few lines: its stderr, else the message of the JSON it answered with.
func ghWords(stderr, stdout string) string {
	words := strings.TrimSpace(stderr)
	if words == "" {
		var body struct {
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(stdout)), &body) == nil {
			words = body.Message
		}
	}
	if words == "" {
		return "gh failed with nothing said"
	}
	lines := strings.Split(words, "\n")
	if len(lines) > 6 {
		lines = append(lines[:6], "…")
	}
	return strings.TrimPrefix(strings.Join(lines, "\n"), "gh: ")
}

// gql is one GraphQL request through gh api graphql, its data decoded into v. GitHub answers a field it cannot resolve
// with an error beside the rest of the data; such an answer is an error here.
func gql(host shell.Shell, query string, variables map[string]any, v any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	out, runErr := run(host, []string{"api", "graphql", "--input", "-"}, string(payload))
	if _, problem := AsProblem(runErr); problem {
		return runErr
	}
	var body struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if out == "" || json.Unmarshal([]byte(out), &body) != nil {
		if runErr != nil {
			return runErr
		}
		return errors.New("gh answered something that is not GitHub's")
	}
	if len(body.Errors) > 0 {
		return &Failed{Words: "GitHub: " + body.Errors[0].Message}
	}
	if runErr != nil {
		return runErr
	}
	return json.Unmarshal(body.Data, v)
}

// rest is one REST request through gh api, its answer decoded into v when v is not nil.
func rest(host shell.Shell, method, path string, body any, v any) error {
	args := []string{"api", path, "--method", method}
	input := ""
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		args = append(args, "--input", "-")
		input = string(payload)
	}
	out, err := run(host, args, input)
	if err != nil {
		return err
	}
	if v == nil || strings.TrimSpace(out) == "" {
		return nil
	}
	return json.Unmarshal([]byte(out), v)
}

var segment = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Repo is a GitHub repository, owner/name.
type Repo struct{ Owner, Name string }

func (r Repo) String() string { return r.Owner + "/" + r.Name }

// ParseRepo reads owner/name, refusing anything else: it reaches gh's arguments and GraphQL's literals.
func ParseRepo(text string) (Repo, error) {
	owner, name, ok := strings.Cut(text, "/")
	if !ok || !segment.MatchString(owner) || !segment.MatchString(name) || owner == "." || owner == ".." || name == "." || name == ".." {
		return Repo{}, fmt.Errorf("%q is not a GitHub repository (owner/name)", text)
	}
	return Repo{Owner: owner, Name: name}, nil
}

var pullURL = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/([1-9][0-9]*)(?:[/?#].*)?$`)

// FromURL reads a pull request's repository and number off its GitHub address.
func FromURL(url string) (Repo, int, bool) {
	m := pullURL.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return Repo{}, 0, false
	}
	repo, err := ParseRepo(m[1] + "/" + m[2])
	if err != nil {
		return Repo{}, 0, false
	}
	n, _ := strconv.Atoi(m[3])
	return repo, n, true
}
