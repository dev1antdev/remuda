package tasks

import (
	"errors"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestIssueURLAcceptsOnlyGitHubIssues(t *testing.T) {
	owner, repo, n, ok := IssueURL(" https://github.com/cli/cli/issues/14814 ")
	if !ok || owner != "cli" || repo != "cli" || n != 14814 {
		t.Fatalf("got %q %q %d %v", owner, repo, n, ok)
	}
	for _, raw := range []string{"https://github.com/cli/cli/issues/14814/", "https://github.com/cli/cli/issues/14814#issuecomment-1", "https://github.com/cli/cli/issues/14814?x=1"} {
		if _, _, got, ok := IssueURL(raw); !ok || got != 14814 {
			t.Fatalf("%q: %d %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"https://gitlab.com/cli/cli/issues/1", "https://github.com/dev1antdev/remuda/pull/71", "http://github.com/cli/cli/issues/1", "https://github.com/cli/cli/issues/0", "github.com/cli/cli/issues/1", "https://github.com/cli/issues/1", ""} {
		if _, _, _, ok := IssueURL(raw); ok {
			t.Fatalf("%q was accepted", raw)
		}
	}
}

const ghAnswer = `{"author":{"id":"MDQ6VXNlcjM2NzI4OTMx","is_bot":false,"login":"babakks","name":"Babak"},"body":"### Describe the bug\r\n\r\nThe token is only expired.\r\n\r\n` + "```" + `sh\r\ngh auth status --no-refresh\r\n` + "```" + `\r\n\r\nSee <b>above</b>.","labels":[{"id":"L1","name":"bug","description":"","color":"d73a4a"},{"id":"L2","name":"gh-auth","description":"","color":"9A3D69"},{"id":"L3","name":"blank","description":"","color":""}],"number":14814,"state":"OPEN","title":"gh auth status shows a misleading error","url":"https://github.com/cli/cli/issues/14814"}`

func TestFetchIssueReadsGhAnswer(t *testing.T) {
	host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return "A new release of gh is available\n" + ghAnswer + "\n", nil }}
	issue, err := FetchIssue(host, "https://github.com/cli/cli/issues/14814")
	if err != nil {
		t.Fatal(err)
	}
	if issue.Owner != "cli" || issue.Repo != "cli" || issue.Number != 14814 || issue.Author != "babakks" || issue.State != "open" || issue.Title != "gh auth status shows a misleading error" {
		t.Fatalf("issue: %+v", issue)
	}
	if len(issue.Labels) != 3 || issue.Labels[0] != (IssueLabel{Name: "bug", Color: "d73a4a"}) {
		t.Fatalf("labels: %+v", issue.Labels)
	}
	scripts := host.Scripts()
	if len(scripts) != 1 || !strings.Contains(scripts[0], "gh issue view 'https://github.com/cli/cli/issues/14814' --json number,title,body,labels,url,state,author 2>&1") || !strings.Contains(scripts[0], "command -v gh") {
		t.Fatalf("script: %q", scripts)
	}
}

func TestFetchIssueFailures(t *testing.T) {
	cases := []struct{ answer, want string }{
		{"gh: command not found\n", "gh is not installed on box"},
		{"To get started with GitHub CLI, please run:  gh auth login\n", "gh is not logged in on box"},
		{"GraphQL: Could not resolve to an Issue with the number of 99999999. (repository.issue)\n", "no such issue"},
		{"something odd\nlast line here\n", "gh on box: last line here"},
	}
	for _, c := range cases {
		host := &shell.Fake{Name: "box", Answer: func(string) (string, error) { return c.answer, &shell.ExitError{Code: 1} }}
		_, err := FetchIssue(host, "https://github.com/cli/cli/issues/1")
		if !errors.Is(err, ErrIssueFetch) || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%q: %v", c.answer, err)
		}
	}
	host := &shell.Fake{Name: "box"}
	if _, err := FetchIssue(host, "https://github.com/cli/cli/pull/1"); !errors.Is(err, ErrInvalid) || len(host.Scripts()) != 0 {
		t.Fatalf("a pull request url ran a script or passed: %v %q", err, host.Scripts())
	}
}

func TestIssueHTMLKeepsParagraphsCodeAndTheLinkFirst(t *testing.T) {
	issue := Issue{URL: "https://github.com/o/r/issues/7", Author: "ann", Body: "First line\r\nsecond line\r\n\r\n```go\r\nfmt.Println(\"<hi>\")\r\n```\r\n\r\nTail <b>bold</b> & more"}
	got := IssueHTML(issue)
	want := `<p>Issue: <a href="https://github.com/o/r/issues/7">https://github.com/o/r/issues/7</a> · by ann</p>` +
		`<p>First line<br>second line</p>` +
		`<pre>fmt.Println(&#34;&lt;hi&gt;&#34;)</pre>` +
		`<p>Tail &lt;b&gt;bold&lt;/b&gt; &amp; more</p>`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if got := IssueHTML(Issue{URL: "https://github.com/o/r/issues/8"}); got != `<p>Issue: <a href="https://github.com/o/r/issues/8">https://github.com/o/r/issues/8</a></p>` {
		t.Fatalf("empty body: %s", got)
	}
	if text := HTMLToText(got); !strings.Contains(text, "Issue: https://github.com/o/r/issues/7") || strings.Count(text, "https://github.com/o/r/issues/7") != 1 {
		t.Fatalf("as text: %q", text)
	}
}

func TestImportIssueMakesTheTaskWithItsLabelsOnce(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	if _, err := st.CreateLabel("Bug", "ff0000"); err != nil {
		t.Fatal(err)
	}
	issue, err := parseIssue(ghAnswer)
	if err != nil {
		t.Fatal(err)
	}
	task, err := ImportIssue(st, inbox[0].ID, issue)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "#14814 gh auth status shows a misleading error" || !strings.HasPrefix(task.Description, `<p>Issue: <a href="https://github.com/cli/cli/issues/14814">`) || task.Done {
		t.Fatalf("task: %+v", task)
	}
	if len(task.Labels) != 3 {
		t.Fatalf("labels: %+v", task.Labels)
	}
	byName := map[string]Label{}
	for _, l := range task.Labels {
		byName[strings.ToLower(l.Title)] = l
	}
	if byName["bug"].Title != "Bug" || byName["bug"].HexColor != "ff0000" {
		t.Fatalf("the existing label was not reused: %+v", byName["bug"])
	}
	if byName["gh-auth"].HexColor != "9a3d69" || byName["blank"].HexColor != neutralLabelColor {
		t.Fatalf("colours: %+v %+v", byName["gh-auth"], byName["blank"])
	}
	all, _ := st.Labels()
	if len(all) != 3 {
		t.Fatalf("labels in the store: %+v", all)
	}
	prompt := Prompt(task, inbox[0], false)
	if strings.Count(prompt, "https://github.com/cli/cli/issues/14814") != 1 || !strings.Contains(prompt, "The token is only expired.") {
		t.Fatalf("prompt: %q", prompt)
	}
	if _, err := ImportIssue(st, inbox[0].ID, issue); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "already task #") {
		t.Fatalf("second import: %v", err)
	}
	other, _ := st.CreateProject("Other", 0)
	if _, err := ImportIssue(st, other.ID, issue); err != nil {
		t.Fatalf("another project refused it: %v", err)
	}
}
