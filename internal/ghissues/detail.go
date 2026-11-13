package ghissues

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Detail is one issue as the viewer shows it: the issue, its timeline, the pull requests that close it and everyone who
// took part.
type Detail struct {
	Number       int        `json:"number"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	URL          string     `json:"url"`
	State        string     `json:"state"`
	Reason       string     `json:"reason,omitempty"`
	Author       Person     `json:"author"`
	Role         string     `json:"role,omitempty"` // the author's tie to the repository: OWNER, MEMBER, COLLABORATOR, CONTRIBUTOR
	Created      time.Time  `json:"created"`
	Updated      time.Time  `json:"updated"`
	Closed       *time.Time `json:"closed,omitempty"`
	Labels       []Label    `json:"labels"`
	Assignees    []Person   `json:"assignees"`
	Milestone    *Milestone `json:"milestone,omitempty"`
	Reactions    []Reaction `json:"reactions"`
	Comments     int        `json:"comments"`
	Timeline     []Event    `json:"timeline"`
	Pulls        []Pull     `json:"pulls"`
	Participants []Person   `json:"participants"`
	Pull         bool       `json:"pull,omitempty"` // the number is a pull request's: the viewer sends it to GitHub
	Me           string     `json:"me"`
	At           time.Time  `json:"at"`
	Source       string     `json:"source"`
}

// Event is one entry of the timeline: a comment, or something done to the issue, a burst of label changes as one.
type Event struct {
	Kind      string     `json:"kind"` // comment, labels, assign, closed, reopened, renamed, milestone, mention, commit
	Actor     Person     `json:"actor"`
	At        time.Time  `json:"at"`
	Body      string     `json:"body,omitempty"`
	Role      string     `json:"role,omitempty"`
	URL       string     `json:"url,omitempty"`
	Reactions []Reaction `json:"reactions,omitempty"`
	Added     []Label    `json:"added,omitempty"`
	Removed   []Label    `json:"removed,omitempty"`
	Who       string     `json:"who,omitempty"` // whom an assignment names
	On        bool       `json:"on,omitempty"`  // assigned, or added to the milestone
	Reason    string     `json:"reason,omitempty"`
	From      string     `json:"from,omitempty"`
	To        string     `json:"to,omitempty"`
	Title     string     `json:"title,omitempty"`
	Ref       *Ref       `json:"ref,omitempty"`
	SHA       string     `json:"sha,omitempty"`
}

// Ref is another issue or pull request that mentioned this one.
type Ref struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
	Pull   bool   `json:"pull,omitempty"`
	Merged bool   `json:"merged,omitempty"`
}

// Pull is a pull request that closes the issue when it merges, or, where GitHub would not say, one that mentioned it.
type Pull struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	State  string `json:"state"` // OPEN, MERGED, CLOSED
	Draft  bool   `json:"draft,omitempty"`
}

// MaxEvents is how many timeline entries one issue carries, the newest kept.
const MaxEvents = 600

// burst is how close in time one person's label changes must be to read as one line.
const burst = time.Minute

type restEvent struct {
	Event       string                     `json:"event"`
	Actor       *restUser                  `json:"actor"`
	User        *restUser                  `json:"user"`
	CreatedAt   time.Time                  `json:"created_at"`
	Body        string                     `json:"body"`
	Association string                     `json:"author_association"`
	HTMLURL     string                     `json:"html_url"`
	Reactions   map[string]json.RawMessage `json:"reactions"`
	Label       *ghLabel                   `json:"label"`
	Assignee    *restUser                  `json:"assignee"`
	StateReason string                     `json:"state_reason"`
	Rename      *struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"rename"`
	Milestone *restMilestone `json:"milestone"`
	Source    *struct {
		Issue *struct {
			Number      int    `json:"number"`
			Title       string `json:"title"`
			State       string `json:"state"`
			HTMLURL     string `json:"html_url"`
			PullRequest *struct {
				MergedAt *time.Time `json:"merged_at"`
			} `json:"pull_request"`
			Repository *struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		} `json:"issue"`
	} `json:"source"`
	CommitID  string `json:"commit_id"`
	CommitURL string `json:"commit_url"`
}

func (e restEvent) who() Person {
	if e.Actor != nil {
		return e.Actor.person()
	}
	return e.User.person()
}

func labelOf(l *ghLabel) Label {
	if l == nil {
		return Label{}
	}
	return Label{Name: l.Name, Color: colorOf(l.Color)}
}

// timelineOf reads the REST timeline's pages, gh's --paginate printing one JSON array a page, into the viewer's events:
// a burst of one person's label changes is one line, an event the viewer does not draw is left out.
func timelineOf(text string) ([]Event, error) {
	trimmed := strings.TrimSpace(text)
	at := strings.Index(trimmed, "[")
	if at < 0 {
		return nil, errors.New(trimmed)
	}
	dec := json.NewDecoder(strings.NewReader(trimmed[at:]))
	var raw []restEvent
	for dec.More() {
		var page []restEvent
		if err := dec.Decode(&page); err != nil {
			return nil, errors.New(trimmed)
		}
		raw = append(raw, page...)
	}
	events := []Event{}
	for _, e := range raw {
		base := Event{Actor: e.who(), At: e.CreatedAt}
		switch e.Event {
		case "commented":
			base.Kind, base.Body, base.Role, base.URL, base.Reactions = "comment", e.Body, e.Association, e.HTMLURL, reactionsOf(e.Reactions)
		case "labeled", "unlabeled":
			added := e.Event == "labeled"
			if last := len(events) - 1; last >= 0 && events[last].Kind == "labels" && events[last].Actor.Login == base.Actor.Login && e.CreatedAt.Sub(events[last].At) < burst {
				if added {
					events[last].Added = append(events[last].Added, labelOf(e.Label))
				} else {
					events[last].Removed = append(events[last].Removed, labelOf(e.Label))
				}
				continue
			}
			base.Kind = "labels"
			if added {
				base.Added = []Label{labelOf(e.Label)}
			} else {
				base.Removed = []Label{labelOf(e.Label)}
			}
		case "assigned", "unassigned":
			base.Kind, base.On = "assign", e.Event == "assigned"
			if e.Assignee != nil {
				base.Who = e.Assignee.person().Login
			}
		case "closed":
			_, base.Reason = stateOf("closed", e.StateReason)
			base.Kind = "closed"
		case "reopened":
			base.Kind = "reopened"
		case "renamed":
			if e.Rename == nil {
				continue
			}
			base.Kind, base.From, base.To = "renamed", e.Rename.From, e.Rename.To
		case "milestoned", "demilestoned":
			base.Kind, base.On = "milestone", e.Event == "milestoned"
			if e.Milestone != nil {
				base.Title = e.Milestone.Title
			}
		case "cross-referenced":
			if e.Source == nil || e.Source.Issue == nil {
				continue
			}
			s := e.Source.Issue
			ref := &Ref{Number: s.Number, Title: s.Title, State: strings.ToLower(s.State), URL: s.HTMLURL, Pull: s.PullRequest != nil}
			if s.PullRequest != nil && s.PullRequest.MergedAt != nil {
				ref.Merged = true
			}
			if s.Repository != nil {
				ref.Repo = s.Repository.FullName
			}
			base.Kind, base.Ref = "mention", ref
		case "referenced":
			repo := commitRepo(e.CommitURL)
			if e.CommitID == "" || repo == "" {
				continue
			}
			base.Kind, base.SHA, base.URL = "commit", e.CommitID, "https://github.com/"+repo+"/commit/"+e.CommitID
		default:
			continue
		}
		events = append(events, base)
	}
	if len(events) > MaxEvents {
		events = events[len(events)-MaxEvents:]
	}
	return events, nil
}

// commitRepo is the owner/repository a commit's API address names.
func commitRepo(url string) string {
	_, rest, ok := strings.Cut(url, "/repos/")
	if !ok {
		return ""
	}
	repo, _, ok := strings.Cut(rest, "/commits/")
	if !ok || !RepoOK(repo) {
		return ""
	}
	return repo
}

// closedByOf reads GraphQL's answer of the pull requests that close the issue; false when GitHub would not say.
func closedByOf(text string) ([]Pull, bool) {
	var answer struct {
		Data struct {
			Repository struct {
				Issue struct {
					ClosedBy struct {
						Nodes []struct {
							Number     int    `json:"number"`
							Title      string `json:"title"`
							URL        string `json:"url"`
							State      string `json:"state"`
							IsDraft    bool   `json:"isDraft"`
							Repository struct {
								NameWithOwner string `json:"nameWithOwner"`
							} `json:"repository"`
						} `json:"nodes"`
					} `json:"closedByPullRequestsReferences"`
				} `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if jsonFrom(text, "{", &answer) != nil || len(answer.Errors) > 0 {
		return nil, false
	}
	pulls := []Pull{}
	for _, n := range answer.Data.Repository.Issue.ClosedBy.Nodes {
		pulls = append(pulls, Pull{Repo: n.Repository.NameWithOwner, Number: n.Number, Title: n.Title, URL: n.URL, State: n.State, Draft: n.IsDraft})
	}
	return pulls, true
}

// mentioningPulls are the pull requests the timeline says mentioned the issue, each once: the linked ones where
// GitHub's closing references cannot be read.
func mentioningPulls(events []Event) []Pull {
	pulls := []Pull{}
	seen := map[string]bool{}
	for _, e := range events {
		if e.Kind != "mention" || e.Ref == nil || !e.Ref.Pull {
			continue
		}
		key := e.Ref.URL
		if seen[key] {
			continue
		}
		seen[key] = true
		state := strings.ToUpper(e.Ref.State)
		if e.Ref.Merged {
			state = "MERGED"
		}
		pulls = append(pulls, Pull{Repo: e.Ref.Repo, Number: e.Ref.Number, Title: e.Ref.Title, URL: e.Ref.URL, State: state})
	}
	return pulls
}

// participantsOf is everyone who wrote or did something on the issue, in order of appearance, people only.
func participantsOf(author Person, events []Event) []Person {
	out := []Person{}
	seen := map[string]bool{}
	add := func(p Person) {
		if p.Bot || p.Login == "" || p.Login == "ghost" || seen[p.Login] {
			return
		}
		seen[p.Login] = true
		out = append(out, p)
	}
	add(author)
	for _, e := range events {
		add(e.Actor)
	}
	return out
}

// detailOf puts the issue's sections together: the issue, the timeline, the closing pull requests.
func detailOf(issueBody, timeline, closedBy string) (Detail, error) {
	var raw restIssue
	if err := jsonFrom(issueBody, "{", &raw); err != nil {
		return Detail{}, err
	}
	if raw.Number == 0 {
		return Detail{}, errors.New("gh's answer names no issue")
	}
	state, reason := stateOf(raw.State, raw.StateReason)
	d := Detail{
		Number: raw.Number, Title: strings.TrimSpace(raw.Title), Body: raw.Body, URL: raw.HTMLURL, State: state, Reason: reason,
		Author: raw.User.person(), Role: raw.Association, Created: raw.CreatedAt, Updated: raw.UpdatedAt, Closed: raw.ClosedAt,
		Labels: labelsOf(raw.Labels), Assignees: []Person{}, Reactions: reactionsOf(raw.Reactions), Comments: raw.Comments,
		Pull: len(raw.PullRequest) > 0 && string(raw.PullRequest) != "null",
	}
	for i := range raw.Assignees {
		d.Assignees = append(d.Assignees, raw.Assignees[i].person())
	}
	if raw.Milestone != nil {
		d.Milestone = &Milestone{Title: raw.Milestone.Title, URL: raw.Milestone.HTMLURL}
	}
	events, err := timelineOf(timeline)
	if err != nil {
		events = []Event{}
	}
	d.Timeline = events
	if pulls, ok := closedByOf(closedBy); ok {
		d.Pulls = pulls
	} else {
		d.Pulls = mentioningPulls(events)
	}
	d.Participants = participantsOf(d.Author, events)
	return d, nil
}
