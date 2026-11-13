package ghissues

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// What gh prints, read into the panel's shapes: gh issue list's JSON for the list, the REST issue, its timeline and
// GraphQL's closing pull requests for one issue.

type ghAuthor struct {
	Login string `json:"login"`
	IsBot bool   `json:"is_bot"`
}

type ghLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type ghListItem struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Author      ghAuthor   `json:"author"`
	State       string     `json:"state"`
	StateReason string     `json:"stateReason"`
	Labels      []ghLabel  `json:"labels"`
	Assignees   []ghAuthor `json:"assignees"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	URL         string     `json:"url"`
}

// jsonFrom decodes the JSON that starts at the first open, past whatever gh printed first; anything else is gh's
// complaint, which the error carries whole.
func jsonFrom(text, open string, v any) error {
	trimmed := strings.TrimSpace(text)
	at := strings.Index(trimmed, open)
	if at < 0 {
		return errors.New(trimmed)
	}
	if err := json.NewDecoder(strings.NewReader(trimmed[at:])).Decode(v); err != nil {
		return errors.New(trimmed)
	}
	return nil
}

func labelsOf(raw []ghLabel) []Label {
	out := []Label{}
	for _, l := range raw {
		out = append(out, Label{Name: l.Name, Color: colorOf(l.Color), Description: l.Description})
	}
	return out
}

// colorOf keeps a colour only as six hex digits, lower case; anything else is no colour.
func colorOf(c string) string {
	c = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(c), "#"))
	if len(c) != 6 {
		return ""
	}
	for _, r := range c {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return ""
		}
	}
	return c
}

func login(name string) string {
	return strings.TrimSuffix(strings.TrimPrefix(name, "app/"), "[bot]")
}

func personOf(a ghAuthor) Person {
	bot := a.IsBot || strings.HasPrefix(a.Login, "app/") || strings.HasSuffix(a.Login, "[bot]")
	return Person{Login: login(a.Login), Bot: bot}
}

func peopleOf(raw []ghAuthor) []Person {
	out := []Person{}
	for _, a := range raw {
		out = append(out, personOf(a))
	}
	return out
}

func stateOf(state, reason string) (string, string) {
	s := strings.ToLower(state)
	if s != "closed" {
		return "open", ""
	}
	if strings.EqualFold(reason, "not_planned") || strings.EqualFold(reason, "not planned") {
		return "closed", "not_planned"
	}
	return "closed", "completed"
}

// parseList reads gh issue list's answer, the most recently updated first unless the search asked for an order of its own.
func parseList(text, query string) ([]Item, error) {
	var raw []ghListItem
	if err := jsonFrom(text, "[", &raw); err != nil {
		return nil, err
	}
	items := []Item{}
	for _, r := range raw {
		state, reason := stateOf(r.State, r.StateReason)
		items = append(items, Item{Number: r.Number, Title: strings.TrimSpace(r.Title), Author: personOf(r.Author), State: state, Reason: reason, Labels: labelsOf(r.Labels), Assignees: peopleOf(r.Assignees), Updated: r.UpdatedAt, URL: r.URL})
	}
	if !strings.Contains(query, "sort:") {
		sort.SliceStable(items, func(i, j int) bool { return items[i].Updated.After(items[j].Updated) })
	}
	return items, nil
}

// Reaction is how many reacted with one of GitHub's eight reactions: +1, -1, laugh, hooray, confused, heart, rocket, eyes.
type Reaction struct {
	Content string `json:"content"`
	Count   int    `json:"count"`
}

var reactionOrder = []string{"+1", "-1", "laugh", "hooray", "confused", "heart", "rocket", "eyes"}

func reactionsOf(raw map[string]json.RawMessage) []Reaction {
	out := []Reaction{}
	for _, content := range reactionOrder {
		var n int
		if v, ok := raw[content]; ok && json.Unmarshal(v, &n) == nil && n > 0 {
			out = append(out, Reaction{Content: content, Count: n})
		}
	}
	return out
}

type restUser struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

func (u *restUser) person() Person {
	if u == nil {
		return Person{Login: "ghost"}
	}
	return Person{Login: login(u.Login), Bot: u.Type == "Bot" || strings.HasSuffix(u.Login, "[bot]")}
}

// Milestone is the milestone an issue belongs to.
type Milestone struct {
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
}

type restMilestone struct {
	Title   string `json:"title"`
	HTMLURL string `json:"html_url"`
}

type restIssue struct {
	Number      int                        `json:"number"`
	Title       string                     `json:"title"`
	Body        string                     `json:"body"`
	User        *restUser                  `json:"user"`
	State       string                     `json:"state"`
	StateReason string                     `json:"state_reason"`
	Labels      []ghLabel                  `json:"labels"`
	Assignees   []restUser                 `json:"assignees"`
	Milestone   *restMilestone             `json:"milestone"`
	Comments    int                        `json:"comments"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
	ClosedAt    *time.Time                 `json:"closed_at"`
	HTMLURL     string                     `json:"html_url"`
	Association string                     `json:"author_association"`
	Reactions   map[string]json.RawMessage `json:"reactions"`
	PullRequest json.RawMessage            `json:"pull_request"`
}
