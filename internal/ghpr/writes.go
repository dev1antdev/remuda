package ghpr

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dev1antdev/remuda/internal/shell"
)

// What the panel writes on GitHub, each at the person's press, through gh on the machine as the person: a comment, a
// reply in a thread, a thread resolved or opened again, a review asked for again, and the pull request's state —
// merged, merged when ready, that cancelled, closed, reopened.

// MaxBody is the longest text a comment, a reply or a squash's message may be: GitHub's own limit for a comment.
const MaxBody = 65536

// ErrNewerHead says a push came in after the head the person saw: nothing was merged.
var ErrNewerHead = errors.New("a newer push came in since you looked: nothing was merged; read it, then try again")

func checkBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", errors.New("the text is empty")
	}
	if len(body) > MaxBody || !utf8.ValidString(body) {
		return "", fmt.Errorf("the text is past GitHub's %d characters", MaxBody)
	}
	return body, nil
}

// PostComment adds a comment to the pull request's conversation.
func PostComment(host shell.Shell, repo Repo, number int, body string) error {
	body, err := checkBody(body)
	if err != nil {
		return err
	}
	return rest(host, "POST", fmt.Sprintf("repos/%s/issues/%d/comments", repo, number), map[string]string{"body": body}, nil)
}

var nodeID = regexp.MustCompile(`^[A-Za-z0-9_=-]{4,100}$`)

// Reply answers in a thread on a line.
func Reply(host shell.Shell, thread, body string) error {
	body, err := checkBody(body)
	if err != nil {
		return err
	}
	if !nodeID.MatchString(thread) {
		return errors.New("not a thread's id")
	}
	var data any
	return gql(host, `mutation($id:ID!,$body:String!){addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$id,body:$body}){comment{id}}}`, map[string]any{"id": thread, "body": body}, &data)
}

// Resolve resolves a thread, or opens it again.
func Resolve(host shell.Shell, thread string, resolved bool) error {
	if !nodeID.MatchString(thread) {
		return errors.New("not a thread's id")
	}
	mutation := "resolveReviewThread"
	if !resolved {
		mutation = "unresolveReviewThread"
	}
	var data any
	return gql(host, `mutation($id:ID!){`+mutation+`(input:{threadId:$id}){thread{id isResolved}}}`, map[string]any{"id": thread}, &data)
}

var reviewerName = regexp.MustCompile(`^[A-Za-z0-9_.\[\]-]+(/[A-Za-z0-9_.-]+)?$`)

// AskAgain asks a reviewer for another review: the same call as asking them first; a team goes by org/slug.
func AskAgain(host shell.Shell, repo Repo, number int, reviewer string) error {
	if !reviewerName.MatchString(reviewer) {
		return fmt.Errorf("%q is not a login or a team", reviewer)
	}
	body := map[string][]string{"reviewers": {reviewer}}
	if _, team, ok := strings.Cut(reviewer, "/"); ok {
		body = map[string][]string{"team_reviewers": {team}}
	}
	return rest(host, "POST", fmt.Sprintf("repos/%s/pulls/%d/requested_reviewers", repo, number), body, nil)
}

// Change is a change of the pull request's state the person confirmed.
type Change struct {
	Action  string `json:"action"`  // merge, squash, rebase, auto, cancel-auto, close, reopen
	Head    string `json:"head"`    // the head commit the person saw: a merge of a newer one is refused
	ID      string `json:"id"`      // the pull request's node id, for auto-merge
	Method  string `json:"method"`  // auto-merge's merge method: merge, squash, rebase
	Subject string `json:"subject"` // a squash's commit subject
	Body    string `json:"body"`    // a squash's commit body
}

var commitOid = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ChangeState merges, closes or reopens the pull request. A merge names the head the person saw (--match-head-commit),
// so a push since is refused rather than merged unseen; merge when ready is GitHub's auto-merge through its own mutation
// with the same head, never gh pr merge --auto, which merges at once when nothing is required to wait for.
func ChangeState(host shell.Shell, repo Repo, number int, c Change) error {
	n := strconv.Itoa(number)
	var args []string
	switch c.Action {
	case "close", "reopen":
		args = []string{"pr", c.Action, n, "--repo", repo.String()}
	case "cancel-auto":
		args = []string{"pr", "merge", n, "--repo", repo.String(), "--disable-auto"}
	case "merge", "squash", "rebase":
		if !commitOid.MatchString(c.Head) {
			return errors.New("the head commit you saw is needed to merge")
		}
		args = []string{"pr", "merge", n, "--repo", repo.String(), "--" + c.Action, "--match-head-commit", c.Head}
		if c.Action == "squash" && strings.TrimSpace(c.Subject) != "" {
			if len(c.Subject)+len(c.Body) > MaxBody {
				return fmt.Errorf("the commit message is past %d characters", MaxBody)
			}
			args = append(args, "--subject", strings.TrimSpace(c.Subject), "--body", strings.TrimSpace(c.Body))
		}
	case "auto":
		if !commitOid.MatchString(c.Head) || !nodeID.MatchString(c.ID) {
			return errors.New("the head commit you saw is needed to merge")
		}
		method := strings.ToUpper(c.Method)
		if method != "MERGE" && method != "SQUASH" && method != "REBASE" {
			return fmt.Errorf("unknown merge method %q", c.Method)
		}
		var data any
		err := gql(host, `mutation($id:ID!,$method:PullRequestMergeMethod!,$head:GitObjectID!){enablePullRequestAutoMerge(input:{pullRequestId:$id,mergeMethod:$method,expectedHeadOid:$head}){pullRequest{id}}}`, map[string]any{"id": c.ID, "method": method, "head": c.Head}, &data)
		return newerHead(err)
	default:
		return fmt.Errorf("unknown change %q", c.Action)
	}
	_, err := run(host, args, "")
	return newerHead(err)
}

// newerHead turns GitHub's word for a head that moved into the sentence the page shows.
func newerHead(err error) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "head branch was modified") || strings.Contains(lower, "expected head") || strings.Contains(lower, "head sha") {
		return ErrNewerHead
	}
	return err
}
