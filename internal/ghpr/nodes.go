package ghpr

import (
	"regexp"
	"strconv"
)

// GitHub's GraphQL nodes as the detail's query names them, and what each becomes.

type page struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type commentNode struct {
	ID          string `json:"id"`
	Body        string `json:"body"`
	URL         string `json:"url"`
	CreatedAt   string `json:"createdAt"`
	SubmittedAt string `json:"submittedAt"`
	State       string `json:"state"`
	DiffHunk    string `json:"diffHunk"`
	Author      *login `json:"author"`
}

func (n commentNode) comment() Comment {
	at := n.CreatedAt
	if at == "" {
		at = n.SubmittedAt
	}
	return Comment{ID: n.ID, Author: who(n.Author), Body: n.Body, URL: n.URL, At: at, State: n.State, Hunk: n.DiffHunk}
}

type threadNode struct {
	ID                 string `json:"id"`
	Path               string `json:"path"`
	Line               int    `json:"line"`
	OriginalLine       int    `json:"originalLine"`
	IsResolved         bool   `json:"isResolved"`
	IsOutdated         bool   `json:"isOutdated"`
	ViewerCanResolve   bool   `json:"viewerCanResolve"`
	ViewerCanUnresolve bool   `json:"viewerCanUnresolve"`
	ViewerCanReply     bool   `json:"viewerCanReply"`
	Comments           struct {
		TotalCount int           `json:"totalCount"`
		Nodes      []commentNode `json:"nodes"`
	} `json:"comments"`
}

func (n threadNode) thread() Thread {
	line := n.Line
	if line == 0 {
		line = n.OriginalLine
	}
	t := Thread{ID: n.ID, Path: n.Path, Line: line, Resolved: n.IsResolved, Outdated: n.IsOutdated, CanReply: n.ViewerCanReply, Comments: []Comment{}}
	t.CanResolve = (n.IsResolved && n.ViewerCanUnresolve) || (!n.IsResolved && n.ViewerCanResolve)
	for _, c := range n.Comments.Nodes {
		t.Comments = append(t.Comments, c.comment())
	}
	if n.Comments.TotalCount > len(n.Comments.Nodes) {
		t.MoreReplies = n.Comments.TotalCount - len(n.Comments.Nodes)
	}
	return t
}

type checkNode struct {
	Typename    string `json:"__typename"`
	DatabaseID  int64  `json:"databaseId"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
	DetailsURL  string `json:"detailsUrl"`
	CheckSuite  *struct {
		WorkflowRun *struct {
			DatabaseID int64 `json:"databaseId"`
			Workflow   struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
	CreatedAt string `json:"createdAt"`
}

var runInLink = regexp.MustCompile(`/actions/runs/(\d+)`)

func (n checkNode) check() Check {
	if n.Typename == "StatusContext" {
		return Check{Name: n.Context, Status: n.State, URL: n.TargetURL, StartedAt: n.CreatedAt}
	}
	c := Check{Name: n.Name, Job: n.DatabaseID, Status: n.Status, Result: n.Conclusion, URL: n.DetailsURL, StartedAt: n.StartedAt, DoneAt: n.CompletedAt}
	if n.CheckSuite != nil && n.CheckSuite.WorkflowRun != nil {
		c.Run = n.CheckSuite.WorkflowRun.DatabaseID
		c.Workflow = n.CheckSuite.WorkflowRun.Workflow.Name
	}
	if c.Run == 0 {
		if m := runInLink.FindStringSubmatch(n.DetailsURL); m != nil {
			c.Run, _ = strconv.ParseInt(m[1], 10, 64)
		}
	}
	if c.Run == 0 {
		c.Job = 0 // a check run of another app: no Actions job to read
	}
	return c
}
