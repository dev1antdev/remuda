package ghpr

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/shell"
)

// Card is a pull request as a list shows it: its state, title, number, author and age, the branch it comes from, and
// how its head commit's checks stand together.
type Card struct {
	ID         string `json:"id"`
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	State      string `json:"state"` // OPEN, MERGED, CLOSED
	Draft      bool   `json:"draft"`
	UpdatedAt  string `json:"updated_at"`
	Head       string `json:"head"`
	Base       string `json:"base"`
	Author     string `json:"author"`
	Checks     string `json:"checks,omitempty"` // the rollup: SUCCESS, FAILURE, ERROR, PENDING, EXPECTED, ""
	Repository string `json:"repository"`
}

const cardFields = `id number title url state isDraft updatedAt headRefName baseRefName author{login} headRepository{nameWithOwner} repository{nameWithOwner} commits(last:1){nodes{commit{statusCheckRollup{state}}}}`

type cardNode struct {
	ID          string `json:"id"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	State       string `json:"state"`
	IsDraft     bool   `json:"isDraft"`
	UpdatedAt   string `json:"updatedAt"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	Author      *login `json:"author"`
	HeadRepo    *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"headRepository"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				Rollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type login struct {
	Login string `json:"login"`
}

// who is an author's login, GitHub's word for one whose account is gone.
func who(l *login) string {
	if l == nil || l.Login == "" {
		return "ghost"
	}
	return l.Login
}

func (n cardNode) card() Card {
	c := Card{ID: n.ID, Number: n.Number, Title: n.Title, URL: n.URL, State: n.State, Draft: n.IsDraft, UpdatedAt: n.UpdatedAt, Head: n.HeadRefName, Base: n.BaseRefName, Author: who(n.Author), Repository: n.Repository.NameWithOwner}
	if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.Rollup != nil {
		c.Checks = n.Commits.Nodes[0].Commit.Rollup.State
	}
	return c
}

// BranchPRs is every pull request from the branch, across the checkout's GitHub remotes in one query; only those whose
// head lives in one of those remotes count (a fork's branch of the same name is someone else's), open ones first.
func BranchPRs(host shell.Shell, c Checkout) ([]Card, error) {
	repos := map[string]bool{}
	var order []Repo
	for _, r := range c.Remotes {
		repo, err := ParseRepo(r.Repo)
		if err != nil || repos[strings.ToLower(r.Repo)] {
			continue
		}
		repos[strings.ToLower(r.Repo)] = true
		order = append(order, repo)
	}
	if c.Branch == "" || len(order) == 0 {
		return []Card{}, nil
	}
	var fields []string
	for i, repo := range order {
		fields = append(fields, fmt.Sprintf(`r%d: repository(owner:%q,name:%q){pullRequests(headRefName:$branch,first:30,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{%s}}}`, i, repo.Owner, repo.Name, cardFields))
	}
	query := `query($branch:String!){` + strings.Join(fields, " ") + `}`
	var data map[string]*struct {
		PullRequests struct {
			Nodes []cardNode `json:"nodes"`
		} `json:"pullRequests"`
	}
	if err := gql(host, query, map[string]any{"branch": c.Branch}, &data); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	cards := []Card{}
	for i := range order {
		r := data["r"+strconv.Itoa(i)]
		if r == nil {
			continue
		}
		for _, n := range r.PullRequests.Nodes {
			if n.HeadRefName != c.Branch || n.HeadRepo == nil || !repos[strings.ToLower(n.HeadRepo.NameWithOwner)] || seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			cards = append(cards, n.card())
		}
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if (cards[i].State == "OPEN") != (cards[j].State == "OPEN") {
			return cards[i].State == "OPEN"
		}
		return cards[i].UpdatedAt > cards[j].UpdatedAt
	})
	return cards, nil
}

// Search is what the Repo list asks: free text with GitHub's qualifiers (author:, label:) passed through, whose pull
// requests (all, mine, involved) and in which state.
type Search struct {
	Text    string            `json:"text"`
	Scope   string            `json:"scope"` // all, mine, involved
	State   string            `json:"state"` // open, draft, merged, closed, all
	Cursors map[string]string `json:"cursors,omitempty"`
	More    bool              `json:"more,omitempty"` // the next page, after Cursors
}

// Found is one page of a search: the cards, the cursors of the next page by search, and GitHub's count — none for
// Involved, which is three searches that overlap.
type Found struct {
	Items   []Card            `json:"items"`
	Cursors map[string]string `json:"cursors"`
	Total   *int              `json:"total,omitempty"`
}

// PageSize is how many cards a page of the Repo list holds.
const PageSize = 25

var numberQuery = regexp.MustCompile(`^#?([1-9][0-9]{0,8})$`)

// scopes are the searches a scope runs, by the key their cursors go under.
func scopes(scope string) (map[string]string, error) {
	switch scope {
	case "", "all":
		return map[string]string{"all": ""}, nil
	case "mine":
		return map[string]string{"mine": "author:@me"}, nil
	case "involved":
		return map[string]string{"involves": "involves:@me", "review": "review-requested:@me", "reviewed": "reviewed-by:@me"}, nil
	}
	return nil, fmt.Errorf("unknown scope %q", scope)
}

func stateQualifier(state string) (string, error) {
	switch state {
	case "", "open":
		return "is:open", nil
	case "draft":
		return "is:open draft:true", nil
	case "merged":
		return "is:merged", nil
	case "closed":
		return "is:closed is:unmerged", nil
	case "all":
		return "", nil
	}
	return "", fmt.Errorf("unknown state %q", state)
}

// SearchPRs reads one page of the repository's pull requests. A number (with or without #) is that pull request
// whatever the filters say, since GitHub's search does not match numbers; a number that is none is searched as text.
func SearchPRs(host shell.Shell, repo Repo, s Search) (Found, error) {
	if m := numberQuery.FindStringSubmatch(strings.TrimSpace(s.Text)); m != nil && !s.More {
		n, _ := strconv.Atoi(m[1])
		var data struct {
			Repository struct {
				PullRequest *cardNode `json:"pullRequest"`
			} `json:"repository"`
		}
		err := gql(host, `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){`+cardFields+`}}}`, map[string]any{"owner": repo.Owner, "name": repo.Name, "number": n}, &data)
		if _, problem := AsProblem(err); problem {
			return Found{}, err
		}
		if err == nil && data.Repository.PullRequest != nil {
			one := 1
			return Found{Items: []Card{data.Repository.PullRequest.card()}, Cursors: map[string]string{}, Total: &one}, nil
		}
	}
	searches, err := scopes(s.Scope)
	if err != nil {
		return Found{}, err
	}
	state, err := stateQualifier(s.State)
	if err != nil {
		return Found{}, err
	}
	base := strings.TrimSpace(strings.Join([]string{"repo:" + repo.String(), "is:pr", state, "sort:updated-desc", strings.TrimSpace(s.Text)}, " "))
	keys := make([]string, 0, len(searches))
	for key := range searches {
		if s.More && s.Cursors[key] == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return Found{Items: []Card{}, Cursors: map[string]string{}}, nil
	}
	vars := map[string]any{}
	var params, fields []string
	for i, key := range keys {
		q := strings.TrimSpace(base + " " + searches[key])
		vars["q"+strconv.Itoa(i)] = q
		params = append(params, fmt.Sprintf("$q%d:String!,$c%d:String", i, i))
		if s.More {
			vars["c"+strconv.Itoa(i)] = s.Cursors[key]
		}
		fields = append(fields, fmt.Sprintf(`s%d: search(type:ISSUE,query:$q%d,first:%d,after:$c%d){issueCount pageInfo{hasNextPage endCursor} nodes{... on PullRequest{%s}}}`, i, i, PageSize, i, cardFields))
	}
	query := "query(" + strings.Join(params, ",") + "){" + strings.Join(fields, " ") + "}"
	var data map[string]*struct {
		IssueCount int `json:"issueCount"`
		PageInfo   struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
		Nodes []cardNode `json:"nodes"`
	}
	if err := gql(host, query, vars, &data); err != nil {
		return Found{}, err
	}
	found := Found{Items: []Card{}, Cursors: map[string]string{}}
	seen := map[string]bool{}
	for i, key := range keys {
		page := data["s"+strconv.Itoa(i)]
		if page == nil {
			continue
		}
		if page.PageInfo.HasNextPage {
			found.Cursors[key] = page.PageInfo.EndCursor
		}
		if len(searches) == 1 {
			total := page.IssueCount
			found.Total = &total
		}
		for _, n := range page.Nodes {
			if n.ID == "" || seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			found.Items = append(found.Items, n.card())
		}
	}
	sort.SliceStable(found.Items, func(i, j int) bool { return found.Items[i].UpdatedAt > found.Items[j].UpdatedAt })
	return found, nil
}
