package ghissues

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// The scripts gh runs in on the machine. Each prints its answer in sections (script.RS): the signed-in login when it is
// asked for, then the conditional request's status and headers, then the reads it decided are needed. A 304 ends the
// script there: nothing moved, so nothing more is asked.

const listFields = "number,title,author,state,stateReason,labels,assignees,updatedAt,url"

// prelude is every script's start: the package managers' bin dirs, and a missing gh said in words gh's absence would not.
func prelude() string {
	return script.PATH + "\ncommand -v gh >/dev/null 2>&1 || { echo 'gh: command not found'; exit 127; }\n"
}

// loginSection prints gh's signed-in login, or nothing; the section is there either way so the others keep their places.
func loginSection(want bool) string {
	if !want {
		return "printf '\\036\\n'\n"
	}
	return "gh api user --jq .login 2>/dev/null; printf '\\036\\n'\n"
}

// probe asks one address conditionally and prints the status line and headers (the body too when whole), then ends the
// script on a 304.
func probe(path, etag string, whole bool) string {
	var b strings.Builder
	b.WriteString("probe=$(gh api -i " + script.Quote(path))
	if etag != "" {
		b.WriteString(" -H " + script.Quote("If-None-Match: "+etag))
	}
	b.WriteString(" 2>&1)\n")
	if whole {
		b.WriteString("printf '%s\\n' \"$probe\" | tr -d '\\r'\n")
	} else {
		b.WriteString("printf '%s\\n' \"$probe\" | tr -d '\\r' | awk '/^$/ { exit } { print }'\n")
	}
	b.WriteString("printf '\\036\\n'\n")
	b.WriteString("case \"$(printf '%s\\n' \"$probe\" | head -n 1)\" in *' 304'*) exit 0 ;; esac\n")
	return b.String()
}

// listScript reads the issues a search finds, after asking whether anything in the repository moved since etag.
func listScript(repo, query, etag string, login bool) string {
	newest := "repos/" + repo + "/issues?state=all&sort=updated&direction=desc&per_page=1"
	return prelude() + loginSection(login) + probe(newest, etag, false) +
		"gh issue list -R " + script.Quote(repo) + " --state all --search " + script.Quote(query) +
		" --limit " + strconv.Itoa(Limit) + " --json " + listFields + " 2>&1\n"
}

const closedByQuery = `query($owner: String!, $name: String!, $number: Int!) { repository(owner: $owner, name: $name) { issue(number: $number) { closedByPullRequestsReferences(first: 10, includeClosedPrs: true) { nodes { number title url state isDraft repository { nameWithOwner } } } } } }`

// detailScript reads one issue whole — the issue itself conditionally, its timeline, and the pull requests that close it.
func detailScript(repo string, number int, etag string, login bool) string {
	owner, name, _ := strings.Cut(repo, "/")
	issue := fmt.Sprintf("repos/%s/issues/%d", repo, number)
	return prelude() + loginSection(login) + probe(issue, etag, true) +
		"gh api --paginate " + script.Quote(issue+"/timeline?per_page=100") + " 2>&1\n" +
		"printf '\\036\\n'\n" +
		"gh api graphql -f owner=" + script.Quote(owner) + " -f name=" + script.Quote(name) + " -F number=" + strconv.Itoa(number) +
		" -f query=" + script.Quote(closedByQuery) + " 2>&1\n"
}

// withBody runs gh calls with the comment the person wrote in the file put on the machine, the file removed whatever
// happens; with no file, the calls alone.
func withBody(file string, calls ...string) string {
	var b strings.Builder
	b.WriteString(prelude())
	if file != "" {
		b.WriteString("f=" + script.Quote(file) + "\n")
		b.WriteString("trap 'rm -f \"$f\"' EXIT\n")
	}
	for _, call := range calls {
		b.WriteString(call + " 2>&1 || exit $?\n")
	}
	return b.String()
}

func ghIssue(verb, repo string, number int, args ...string) string {
	line := "gh issue " + verb + " " + strconv.Itoa(number) + " -R " + script.Quote(repo)
	for _, a := range args {
		line += " " + a
	}
	return line
}

func commentCall(repo string, number int) string {
	return ghIssue("comment", repo, number, `--body-file "$f"`)
}

// commentScript posts the file's text as a comment.
func commentScript(repo string, number int, file string) string {
	return withBody(file, commentCall(repo, number))
}

// closeScript closes the issue for the reason, the comment posted first when there is one, as GitHub's Close with comment does.
func closeScript(repo string, number int, reason, file string) string {
	closing := ghIssue("close", repo, number, "--reason", script.Quote(reason))
	if file == "" {
		return withBody("", closing)
	}
	return withBody(file, commentCall(repo, number), closing)
}

// reopenScript reopens the issue, the comment posted first when there is one.
func reopenScript(repo string, number int, file string) string {
	reopening := ghIssue("reopen", repo, number)
	if file == "" {
		return withBody("", reopening)
	}
	return withBody(file, commentCall(repo, number), reopening)
}

// assignScript adds or removes the signed-in account among the issue's assignees.
func assignScript(repo string, number int, me bool) string {
	flag := "--remove-assignee"
	if me {
		flag = "--add-assignee"
	}
	return withBody("", ghIssue("edit", repo, number, flag, "@me"))
}

var (
	statusRe = regexp.MustCompile(`(?m)^HTTP/[0-9.]+ ([0-9]{3})`)
	etagRe   = regexp.MustCompile(`(?im)^etag:[ \t]*(.+?)[ \t]*$`)
)

// response is what `gh api -i` printed: the status, the ETag, and the body after the headers when it was printed.
type response struct {
	status int
	etag   string
	body   string
	text   string // everything, for a complaint
}

func parseResponse(section string) response {
	text := strings.TrimSpace(section)
	r := response{text: text}
	m := statusRe.FindStringSubmatchIndex(text)
	if m == nil {
		return r
	}
	r.status, _ = strconv.Atoi(text[m[2]:m[3]])
	head := text[m[0]:]
	if end := strings.Index(head, "\n\n"); end >= 0 {
		r.body = strings.TrimSpace(head[end+2:])
		head = head[:end]
	}
	if e := etagRe.FindStringSubmatch(head); e != nil {
		r.etag = e[1]
	}
	return r
}
