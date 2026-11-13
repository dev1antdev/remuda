package ghpr

import (
	"errors"

	"github.com/dev1antdev/remuda/internal/shell"
)

// Comment is one thing said on the pull request: a comment of the conversation, a review's text, or a reply in a
// thread on a line.
type Comment struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	At     string `json:"at"`
	State  string `json:"state,omitempty"` // a review's: APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
	Hunk   string `json:"hunk,omitempty"`  // a thread comment's diff hunk
}

// Thread is a conversation on a line of the diff.
type Thread struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	Line        int       `json:"line"`
	Resolved    bool      `json:"resolved"`
	Outdated    bool      `json:"outdated"`
	CanResolve  bool      `json:"can_resolve"`
	CanReply    bool      `json:"can_reply"`
	Comments    []Comment `json:"comments"`
	MoreReplies int       `json:"more_replies,omitempty"` // replies past the hundred read, on GitHub only
}

// Check is one check of the head commit: a GitHub Actions job (its run and job ids) or a status from elsewhere.
type Check struct {
	Name      string `json:"name"`
	Workflow  string `json:"workflow,omitempty"`
	Run       int64  `json:"run,omitempty"`
	Job       int64  `json:"job,omitempty"`
	Status    string `json:"status"`               // QUEUED, IN_PROGRESS, COMPLETED, or a status context's state
	Result    string `json:"conclusion,omitempty"` // SUCCESS, FAILURE, SKIPPED, …
	URL       string `json:"url,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	DoneAt    string `json:"completed_at,omitempty"`
}

// Reviewer is who reviewed or is asked to, as they stand now: their latest opinion, a comment if that is all they left,
// a request waiting over either.
type Reviewer struct {
	Login string `json:"login"`
	State string `json:"state"` // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, REQUESTED
	Team  bool   `json:"team,omitempty"`
}

// Detail is one pull request in full.
type Detail struct {
	ID             string     `json:"id"`
	Number         int        `json:"number"`
	Title          string     `json:"title"`
	URL            string     `json:"url"`
	Body           string     `json:"body"`
	State          string     `json:"state"`
	Draft          bool       `json:"draft"`
	Author         string     `json:"author"`
	Head           string     `json:"head"`
	Base           string     `json:"base"`
	HeadOid        string     `json:"head_oid"`
	ChangedFiles   int        `json:"changed_files"`
	Additions      int        `json:"additions"`
	Deletions      int        `json:"deletions"`
	ReviewDecision string     `json:"review_decision"`
	Mergeable      string     `json:"mergeable"`
	MergeState     string     `json:"merge_state"` // CLEAN, BLOCKED, BEHIND, DIRTY, UNSTABLE, HAS_HOOKS, DRAFT, UNKNOWN
	AutoMerge      bool       `json:"auto_merge"`
	MergeMethods   []string   `json:"merge_methods"` // what the repository allows: merge, squash, rebase
	AutoAllowed    bool       `json:"auto_allowed"`
	Repository     string     `json:"repository"`
	Viewer         string     `json:"viewer"`
	Comments       []Comment  `json:"comments"`
	Reviews        []Comment  `json:"reviews"`
	Threads        []Thread   `json:"threads"`
	Checks         []Check    `json:"checks"`
	Reviewers      []Reviewer `json:"reviewers"`
	ReviewersError string     `json:"reviewers_error,omitempty"`
	Cut            bool       `json:"cut,omitempty"` // the conversation ran past what is read; the rest is on GitHub
}

const pageInfo = `pageInfo{hasNextPage endCursor}`

const detailQuery = `query($owner:String!,$name:String!,$number:Int!,$comments:String,$reviews:String,$threads:String){
viewer{login}
repository(owner:$owner,name:$name){nameWithOwner mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed autoMergeAllowed
pullRequest(number:$number){id number title url body state isDraft headRefName baseRefName headRefOid changedFiles additions deletions author{login} reviewDecision mergeable mergeStateStatus autoMergeRequest{enabledAt}
comments(first:100,after:$comments){` + pageInfo + ` nodes{id body url createdAt author{login}}}
reviews(first:100,after:$reviews){` + pageInfo + ` nodes{id body url submittedAt state author{login}}}
reviewThreads(first:100,after:$threads){` + pageInfo + ` nodes{id path line originalLine isResolved isOutdated viewerCanResolve viewerCanUnresolve viewerCanReply comments(first:100){totalCount nodes{id body url createdAt diffHunk author{login}}}}}
commits(last:1){nodes{commit{statusCheckRollup{contexts(first:100){nodes{__typename ... on CheckRun{databaseId name status conclusion startedAt completedAt detailsUrl checkSuite{workflowRun{databaseId workflow{name}}}} ... on StatusContext{context state targetUrl createdAt}}}}}}}
}}}`

type detailData struct {
	Viewer     login `json:"viewer"`
	Repository *struct {
		NameWithOwner      string `json:"nameWithOwner"`
		MergeCommitAllowed bool   `json:"mergeCommitAllowed"`
		SquashMergeAllowed bool   `json:"squashMergeAllowed"`
		RebaseMergeAllowed bool   `json:"rebaseMergeAllowed"`
		AutoMergeAllowed   bool   `json:"autoMergeAllowed"`
		PullRequest        *struct {
			ID               string `json:"id"`
			Number           int    `json:"number"`
			Title            string `json:"title"`
			URL              string `json:"url"`
			Body             string `json:"body"`
			State            string `json:"state"`
			IsDraft          bool   `json:"isDraft"`
			HeadRefName      string `json:"headRefName"`
			BaseRefName      string `json:"baseRefName"`
			HeadRefOid       string `json:"headRefOid"`
			ChangedFiles     int    `json:"changedFiles"`
			Additions        int    `json:"additions"`
			Deletions        int    `json:"deletions"`
			Author           *login `json:"author"`
			ReviewDecision   string `json:"reviewDecision"`
			Mergeable        string `json:"mergeable"`
			MergeStateStatus string `json:"mergeStateStatus"`
			AutoMergeRequest *struct {
				EnabledAt string `json:"enabledAt"`
			} `json:"autoMergeRequest"`
			Comments struct {
				PageInfo page          `json:"pageInfo"`
				Nodes    []commentNode `json:"nodes"`
			} `json:"comments"`
			Reviews struct {
				PageInfo page          `json:"pageInfo"`
				Nodes    []commentNode `json:"nodes"`
			} `json:"reviews"`
			ReviewThreads struct {
				PageInfo page         `json:"pageInfo"`
				Nodes    []threadNode `json:"nodes"`
			} `json:"reviewThreads"`
			Commits struct {
				Nodes []struct {
					Commit struct {
						Rollup *struct {
							Contexts struct {
								Nodes []checkNode `json:"nodes"`
							} `json:"contexts"`
						} `json:"statusCheckRollup"`
					} `json:"commit"`
				} `json:"nodes"`
			} `json:"commits"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

// MaxPages is how many pages of each of the conversation's lists are read before the rest is left to GitHub.
const MaxPages = 5

// ErrNotFound says the pull request is not there, or this account cannot see it.
var ErrNotFound = errors.New("the pull request is not there, or this GitHub account cannot see it")

// ReadDetail reads one pull request: its fields, the repository's merge methods, the conversation a page of a hundred
// at a time (at most MaxPages of each), the head commit's checks, and its reviewers in a query of their own, since a
// team reviewer can need more access than the pull request and must not cost the whole view.
func ReadDetail(host shell.Shell, repo Repo, number int) (Detail, error) {
	vars := map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}
	var d Detail
	for round := 0; round < MaxPages; round++ {
		var data detailData
		if err := gql(host, detailQuery, vars, &data); err != nil {
			return Detail{}, err
		}
		if data.Repository == nil || data.Repository.PullRequest == nil {
			return Detail{}, ErrNotFound
		}
		pr := data.Repository.PullRequest
		if round == 0 {
			d = Detail{ID: pr.ID, Number: pr.Number, Title: pr.Title, URL: pr.URL, Body: pr.Body, State: pr.State, Draft: pr.IsDraft, Author: who(pr.Author), Head: pr.HeadRefName, Base: pr.BaseRefName, HeadOid: pr.HeadRefOid,
				ChangedFiles: pr.ChangedFiles, Additions: pr.Additions, Deletions: pr.Deletions, ReviewDecision: pr.ReviewDecision, Mergeable: pr.Mergeable, MergeState: pr.MergeStateStatus, AutoMerge: pr.AutoMergeRequest != nil,
				AutoAllowed: data.Repository.AutoMergeAllowed, Repository: data.Repository.NameWithOwner, Viewer: data.Viewer.Login,
				Comments: []Comment{}, Reviews: []Comment{}, Threads: []Thread{}, Checks: []Check{}, Reviewers: []Reviewer{}, MergeMethods: []string{}}
			for _, m := range []struct {
				allowed bool
				name    string
			}{{data.Repository.MergeCommitAllowed, "merge"}, {data.Repository.SquashMergeAllowed, "squash"}, {data.Repository.RebaseMergeAllowed, "rebase"}} {
				if m.allowed {
					d.MergeMethods = append(d.MergeMethods, m.name)
				}
			}
			if len(pr.Commits.Nodes) > 0 && pr.Commits.Nodes[0].Commit.Rollup != nil {
				for _, c := range pr.Commits.Nodes[0].Commit.Rollup.Contexts.Nodes {
					d.Checks = append(d.Checks, c.check())
				}
			}
		}
		if round == 0 || vars["comments"] != nil {
			for _, c := range pr.Comments.Nodes {
				d.Comments = append(d.Comments, c.comment())
			}
		}
		if round == 0 || vars["reviews"] != nil {
			for _, c := range pr.Reviews.Nodes {
				if c.State != "PENDING" { // a review its author has not submitted, which only they can see
					d.Reviews = append(d.Reviews, c.comment())
				}
			}
		}
		if round == 0 || vars["threads"] != nil {
			for _, t := range pr.ReviewThreads.Nodes {
				d.Threads = append(d.Threads, t.thread())
			}
		}
		more := false
		for key, p := range map[string]page{"comments": pr.Comments.PageInfo, "reviews": pr.Reviews.PageInfo, "threads": pr.ReviewThreads.PageInfo} {
			if (round == 0 || vars[key] != nil) && p.HasNextPage {
				vars[key] = p.EndCursor
				more = true
			} else {
				delete(vars, key)
			}
		}
		if !more {
			break
		}
		if round == MaxPages-1 {
			d.Cut = true
		}
	}
	reviewers, err := readReviewers(host, repo, number, d.Reviews, d.Author)
	if err != nil {
		if _, problem := AsProblem(err); problem {
			return Detail{}, err
		}
		d.ReviewersError = err.Error()
	}
	d.Reviewers = reviewers
	return d, nil
}
