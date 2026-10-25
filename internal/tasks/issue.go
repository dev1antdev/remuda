package tasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// ErrIssueFetch: gh on the machine could not read the issue; the text says why.
var ErrIssueFetch = errors.New("the issue could not be read")

// Issue is what gh answers about a GitHub issue, enough to make a task of it.
type Issue struct {
	Owner  string
	Repo   string
	Number int
	Title  string
	Body   string
	URL    string
	State  string
	Author string
	Labels []IssueLabel
}

// IssueLabel is a label on the issue with GitHub's colour for it.
type IssueLabel struct {
	Name  string
	Color string
}

var issueURLRe = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/issues/([1-9][0-9]*)/?(?:[?#].*)?$`)

// IssueURL reads a GitHub issue address: owner, repository and number; false for anything else, a pull request included.
func IssueURL(raw string) (owner, repo string, number int, ok bool) {
	m := issueURLRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(m[3])
	if err != nil {
		return "", "", 0, false
	}
	return m[1], m[2], n, true
}

// IssueScript is what the machine runs to read an issue: gh as the user, JSON out, its complaints on the same stream.
func IssueScript(url string) string {
	return script.PATH + "\n" +
		"command -v gh >/dev/null 2>&1 || { echo 'gh: command not found'; exit 127; }\n" +
		"gh issue view " + script.Quote(url) + " --json number,title,body,labels,url,state,author 2>&1"
}

const issueTimeout = 30 * time.Second

// FetchIssue reads an issue through gh on the host.
func FetchIssue(host shell.Host, url string) (Issue, error) {
	owner, repo, _, ok := IssueURL(url)
	if !ok {
		return Issue{}, invalid("a GitHub issue address looks like https://github.com/owner/repo/issues/123")
	}
	out, err := shell.Text(host, IssueScript(strings.TrimSpace(url)), issueTimeout)
	issue, perr := parseIssue(out)
	if perr != nil {
		return Issue{}, issueFailure(host.Label(), out, err)
	}
	issue.Owner, issue.Repo = owner, repo
	return issue, nil
}

// parseIssue reads gh's JSON out of its output; a notice before it is skipped, anything else is an error.
func parseIssue(out string) (Issue, error) {
	text := strings.TrimSpace(out)
	start := strings.Index(text, "{")
	if start < 0 {
		return Issue{}, errors.New("no JSON in gh's answer")
	}
	var raw struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		URL    string `json:"url"`
		State  string `json:"state"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Labels []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"labels"`
	}
	if err := json.NewDecoder(strings.NewReader(text[start:])).Decode(&raw); err != nil {
		return Issue{}, err
	}
	if raw.Number == 0 || raw.URL == "" {
		return Issue{}, errors.New("gh's answer names no issue")
	}
	issue := Issue{Number: raw.Number, Title: strings.TrimSpace(raw.Title), Body: raw.Body, URL: raw.URL, State: strings.ToLower(raw.State), Author: raw.Author.Login}
	for _, l := range raw.Labels {
		issue.Labels = append(issue.Labels, IssueLabel{Name: l.Name, Color: l.Color})
	}
	return issue, nil
}

// issueFailure turns gh's complaint into a sentence naming the machine and what to do there.
func issueFailure(machine, out string, err error) error {
	text := strings.TrimSpace(out)
	if text == "" && err != nil {
		text = err.Error()
	}
	low := strings.ToLower(text)
	var why string
	switch {
	case strings.Contains(low, "command not found"):
		why = "gh is not installed on " + machine
	case strings.Contains(low, "gh auth login"), strings.Contains(low, "not logged in"), strings.Contains(low, "http 401"):
		why = "gh is not logged in on " + machine + " (run gh auth login there)"
	case strings.Contains(low, "could not resolve"), strings.Contains(low, "http 404"), strings.Contains(low, "not found"):
		why = "no such issue, or gh on " + machine + " cannot see it"
	case text == "":
		why = "gh on " + machine + " answered nothing"
	default:
		lines := strings.Split(text, "\n")
		why = "gh on " + machine + ": " + strings.TrimSpace(lines[len(lines)-1])
	}
	return fmt.Errorf("%w: %s", ErrIssueFetch, why)
}

// neutralLabelColor dresses a label GitHub gave no colour.
const neutralLabelColor = "8c8fa1"

// ImportIssue makes a task of an issue in a project: number and title as the title, the body behind a line naming the issue,
// GitHub's labels as the store's (matched by name, made with GitHub's colour when missing). An issue already in the project is refused.
func ImportIssue(st *Store, projectID int64, issue Issue) (Task, error) {
	if issue.URL == "" || issue.Number == 0 {
		return Task{}, invalid("not an issue")
	}
	if id, ok, err := st.taskWithIssue(projectID, issue.URL); err != nil {
		return Task{}, err
	} else if ok {
		return Task{}, invalid(fmt.Sprintf("this issue is already task #%d in the project", id))
	}
	title := fmt.Sprintf("#%d %s", issue.Number, strings.TrimSpace(issue.Title))
	description := IssueHTML(issue)
	task, err := st.Create(projectID, Patch{Title: &title, Description: &description})
	if err != nil {
		return Task{}, err
	}
	for _, l := range issue.Labels {
		label, err := st.CreateLabel(l.Name, labelColor(l.Color))
		if err != nil {
			return Task{}, err
		}
		if err := st.AddLabel(task.ID, label.ID); err != nil {
			return Task{}, err
		}
	}
	return st.Get(task.ID)
}

func labelColor(hex string) string {
	if colorRe.MatchString(strings.TrimPrefix(hex, "#")) {
		return strings.ToLower(strings.TrimPrefix(hex, "#"))
	}
	return neutralLabelColor
}

// taskWithIssue finds the task of a project whose description names the issue, as IssueHTML writes it.
func (st *Store) taskWithIssue(projectID int64, url string) (int64, bool, error) {
	var id int64
	err := st.db.QueryRow(`SELECT id FROM tasks WHERE project_id = ? AND instr(description, ?) > 0 LIMIT 1`, projectID, `href="`+html.EscapeString(url)+`"`).Scan(&id)
	switch {
	case err == nil:
		return id, true, nil
	case strings.Contains(err.Error(), "no rows"):
		return 0, false, nil
	}
	return 0, false, err
}

// IssueHTML is the issue as a description: a first paragraph naming the issue by its address (the agent reads it as text), then
// the body's paragraphs, a fenced code block kept whole.
func IssueHTML(issue Issue) string {
	var b strings.Builder
	address := html.EscapeString(issue.URL)
	b.WriteString(`<p>Issue: <a href="` + address + `">` + address + `</a>`)
	if issue.Author != "" {
		b.WriteString(" · by " + html.EscapeString(issue.Author))
	}
	b.WriteString("</p>")
	body := strings.ReplaceAll(strings.TrimSpace(issue.Body), "\r\n", "\n")
	for _, block := range issueBlocks(body) {
		if block.code {
			b.WriteString("<pre>" + html.EscapeString(block.text) + "</pre>")
		} else {
			b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(block.text), "\n", "<br>") + "</p>")
		}
	}
	return b.String()
}

type issueBlock struct {
	text string
	code bool
}

// issueBlocks splits a Markdown body into paragraphs at blank lines; a fenced code block is one block without its fence.
func issueBlocks(body string) []issueBlock {
	if body == "" {
		return nil
	}
	var out []issueBlock
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, issueBlock{text: strings.Join(para, "\n")})
			para = nil
		}
	}
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			flush()
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				code = append(code, lines[i])
			}
			out = append(out, issueBlock{text: strings.Join(code, "\n"), code: true})
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		para = append(para, line)
	}
	flush()
	return out
}
