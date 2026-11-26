package herdr

// herdr's event stream (events.subscribe, protocol 22): one request on a connection that stays open, its subscriptions
// fixed for the connection's life — a second subscribe on it closes it, so a new set is a new connection. herdr answers
// subscription_started, then one event a line; a pane a subscription names that is gone refuses the whole request
// (pane_not_found), an output match whose regex herdr cannot read refuses it too (invalid_regex), and a reader too slow
// for a burst gets events_lost, or the connection closed under it.

// Subscription is one entry of events.subscribe: a global kind ("pane.created"), or one bound to a pane
// ("pane.agent_status_changed", "pane.output_matched" with its match).
type Subscription struct {
	Type   string       `json:"type"`
	PaneID string       `json:"pane_id,omitempty"`
	Source string       `json:"source,omitempty"` // an output match's rows: recent (the screen and the scrollback above it)
	Lines  int          `json:"lines,omitempty"`  // how many of them herdr reads
	Match  *OutputMatch `json:"match,omitempty"`
}

// OutputMatch is what herdr looks for in a pane's rows: a substring, case and all, or a regex in Rust's dialect, read a
// line at a time.
type OutputMatch struct {
	Type  string `json:"type"` // substring | regex
	Value string `json:"value"`
}

// Event is one line of the stream, the fields remuda reads. herdr names the global kinds with an underscore
// ("pane_created") and the per-pane ones with a dot ("pane.agent_status_changed"); Kind is as herdr sent it.
type Event struct {
	Kind        string
	PaneID      string       // the pane it concerns, when it names one
	AgentStatus string       // pane.agent_status_changed
	MatchedLine string       // pane.output_matched: the first row that matched
	Text        string       // pane.output_matched: the rows herdr read
	Standing    *bool        // pane.output_matched: whether the row stood before the watch began, as remuda-core says; nil from herdr
	Pos         uint64       // pane.output_matched: the row's place in the terminal, as remuda-core says; 0 from herdr
	Watch       *OutputMatch // pane.output_matched: the watch that told it, as remuda-core says; nil from herdr, which names none
}

// Event kinds remuda tells apart.
const (
	EventAgentStatus   = "pane.agent_status_changed"
	EventOutputMatched = "pane.output_matched"
)

// ErrEventsLost is herdr saying the stream fell behind and dropped events.
const ErrEventsLost = "events_lost"
