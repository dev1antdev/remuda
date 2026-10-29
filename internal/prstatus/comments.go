package prstatus

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// The reviewers' word handed to the agent: the request's reviews and the comments on its lines, read with gh, in the
// shape the Changes lens sends its own comments.

// MaxComments is how many line comments one message carries.
const MaxComments = 60

// ReviewNote is one review on the request: a verdict with its text.
type ReviewNote struct {
	Author string
	State  string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
	Body   string
}

// LineComment is a reviewer's comment on a line of the request's diff.
type LineComment struct {
	Author  string
	Path    string
	Line    int
	Body    string
	Code    string // the diff row the comment sits on
	ReplyTo int64
}

// Review is what the reviewers said on the request.
type Review struct {
	Reviews  []ReviewNote
	Comments []LineComment
}

var pullURL = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

// URLParts reads the owner, the repository and the number off a GitHub pull request's address.
func URLParts(url string) (owner, repo string, number int, ok bool) {
	m := pullURL.FindStringSubmatch(url)
	if m == nil {
		return "", "", 0, false
	}
	number, _ = strconv.Atoi(m[3])
	return m[1], m[2], number, true
}

// ReviewScript reads the request's reviews and its line comments on the pane's machine, in two sections.
func ReviewScript(url string) string {
	owner, repo, number, ok := URLParts(url)
	if !ok {
		return ""
	}
	comments := fmt.Sprintf("repos/%s/%s/pulls/%d/comments?per_page=100", owner, repo, number)
	return script.PATH + "\ngh pr view " + script.Quote(url) + " --json reviews 2>&1; printf '\\036\\n'; gh api " + script.Quote(comments) + " 2>&1"
}

type ghReview struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	State string `json:"state"`
	Body  string `json:"body"`
}

type ghComment struct {
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Path         string `json:"path"`
	Line         int    `json:"line"`
	OriginalLine int    `json:"original_line"`
	Body         string `json:"body"`
	DiffHunk     string `json:"diff_hunk"`
	InReplyTo    int64  `json:"in_reply_to_id"`
}

// ParseReview reads the script's two sections; a section gh could not answer is the error's text.
func ParseReview(out string) (Review, error) {
	sections := script.Sections(out)
	if len(sections) < 2 {
		return Review{}, errors.New(firstLine(strings.TrimSpace(out), "gh answered nothing"))
	}
	var view struct {
		Reviews []ghReview `json:"reviews"`
	}
	if err := jsonAt(sections[0], "{", &view); err != nil {
		return Review{}, err
	}
	var raw []ghComment
	if err := jsonAt(sections[1], "[", &raw); err != nil {
		return Review{}, err
	}
	var review Review
	for _, r := range view.Reviews {
		if strings.TrimSpace(r.Body) == "" && r.State != "CHANGES_REQUESTED" {
			continue
		}
		review.Reviews = append(review.Reviews, ReviewNote{Author: r.Author.Login, State: r.State, Body: strings.TrimSpace(r.Body)})
	}
	for _, c := range raw {
		line := c.Line
		if line == 0 {
			line = c.OriginalLine
		}
		review.Comments = append(review.Comments, LineComment{Author: c.User.Login, Path: c.Path, Line: line, Body: strings.TrimSpace(c.Body), Code: lastRow(c.DiffHunk), ReplyTo: c.InReplyTo})
	}
	return review, nil
}

// jsonAt decodes the JSON that starts at the first open of the section, past whatever gh printed first.
func jsonAt(section, open string, v any) error {
	trimmed := strings.TrimSpace(section)
	at := strings.Index(trimmed, open)
	if at < 0 {
		return errors.New(firstLine(trimmed, "gh answered nothing"))
	}
	if err := json.Unmarshal([]byte(trimmed[at:]), v); err != nil {
		return errors.New(firstLine(trimmed, err.Error()))
	}
	return nil
}

// lastRow is the diff row a comment sits on: the last line of its hunk.
func lastRow(hunk string) string {
	hunk = strings.TrimRight(hunk, "\n")
	if hunk == "" {
		return ""
	}
	return hunk[strings.LastIndex(hunk, "\n")+1:]
}

// ErrNoReview says nobody has reviewed or commented on the request yet.
var ErrNoReview = errors.New("no review or comment on the request yet")

// ReviewMessage is what the agent is told: each review's verdict and text, then the line comments numbered as the
// Changes lens numbers its own, and what to do with them.
func ReviewMessage(label, url string, review Review) (string, error) {
	if len(review.Reviews) == 0 && len(review.Comments) == 0 {
		return "", ErrNoReview
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Review of %s (%s):", label, url)
	for _, r := range review.Reviews {
		fmt.Fprintf(&b, "\n\n%s %s", r.Author, verdictWords(r.State))
		if r.Body != "" {
			b.WriteString(":\n" + indent(r.Body))
		}
	}
	comments := review.Comments
	if len(comments) > MaxComments {
		comments = comments[:MaxComments]
	}
	if len(comments) > 0 {
		fmt.Fprintf(&b, "\n\nComments (%d):", len(review.Comments))
		for i, c := range comments {
			fmt.Fprintf(&b, "\n\n%d. %s:%d (%s", i+1, c.Path, c.Line, c.Author)
			if c.ReplyTo != 0 {
				b.WriteString(", in reply")
			}
			b.WriteString(")")
			if c.Code != "" {
				b.WriteString("\n   > " + c.Code)
			}
			b.WriteString("\n   " + strings.ReplaceAll(c.Body, "\n", "\n   "))
		}
		if len(review.Comments) > MaxComments {
			fmt.Fprintf(&b, "\n\n…and %d more on the request itself.", len(review.Comments)-MaxComments)
		}
	}
	b.WriteString("\n\nWork on what is asked above, push the branch, and say when it is ready for another look.")
	return b.String(), nil
}

func verdictWords(state string) string {
	switch state {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "requested changes"
	case "DISMISSED":
		return "wrote (the review was dismissed)"
	}
	return "wrote"
}

func indent(text string) string {
	return "   " + strings.ReplaceAll(text, "\n", "\n   ")
}
