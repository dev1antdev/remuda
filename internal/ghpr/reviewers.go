package ghpr

import "github.com/dev1antdev/remuda/internal/shell"

// The reviewers are read in a query of their own: a team reviewer can need more access than the pull request.

const reviewersQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){
reviewRequests(first:50){nodes{requestedReviewer{__typename ... on User{login} ... on Bot{login} ... on Mannequin{login} ... on Team{combinedSlug}}}}
latestOpinionatedReviews(first:50){nodes{state author{login}}}
}}}`

// readReviewers is one row per reviewer as they stand now: their latest opinion, a comment if that is all they left
// (the author's own aside), and a request waiting over either, since that is what they owe.
func readReviewers(host shell.Shell, repo Repo, number int, reviews []Comment, author string) ([]Reviewer, error) {
	var data struct {
		Repository struct {
			PullRequest *struct {
				ReviewRequests struct {
					Nodes []struct {
						Requested *struct {
							Typename     string `json:"__typename"`
							Login        string `json:"login"`
							CombinedSlug string `json:"combinedSlug"`
						} `json:"requestedReviewer"`
					} `json:"nodes"`
				} `json:"reviewRequests"`
				Opinions struct {
					Nodes []struct {
						State  string `json:"state"`
						Author *login `json:"author"`
					} `json:"nodes"`
				} `json:"latestOpinionatedReviews"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	if err := gql(host, reviewersQuery, map[string]any{"owner": repo.Owner, "name": repo.Name, "number": number}, &data); err != nil {
		return []Reviewer{}, err
	}
	rows := []Reviewer{}
	at := map[string]int{}
	set := func(name, state string, team bool) {
		if name == "" {
			return
		}
		if i, ok := at[name]; ok {
			rows[i].State = state
			return
		}
		at[name] = len(rows)
		rows = append(rows, Reviewer{Login: name, State: state, Team: team})
	}
	pr := data.Repository.PullRequest
	if pr == nil {
		return rows, nil
	}
	for _, n := range pr.Opinions.Nodes {
		set(who(n.Author), n.State, false)
	}
	for _, r := range reviews {
		if _, ok := at[r.Author]; !ok && r.State == "COMMENTED" && r.Author != author {
			set(r.Author, "COMMENTED", false)
		}
	}
	for _, n := range pr.ReviewRequests.Nodes {
		if n.Requested == nil {
			continue
		}
		if n.Requested.CombinedSlug != "" {
			set(n.Requested.CombinedSlug, "REQUESTED", true)
		} else {
			set(n.Requested.Login, "REQUESTED", false)
		}
	}
	return rows, nil
}
