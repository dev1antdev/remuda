package schedule

import "github.com/dev1antdev/remuda/internal/sessions"

// Denies says whether the schedule's agent is started to deny what would wait on a permission prompt: an agent schedule
// with the switch on, for an agent that knows how.
func (s Schedule) Denies() bool {
	return s.Kind == KindAgent && s.DenyPrompts != nil && *s.DenyPrompts && sessions.CanDenyPrompts(s.Agent)
}

// Start is how the schedule's agent starts: its model, the person's arguments split as a shell would, and the deny
// setting merged in when the switch is on; an error says what its command line cannot take.
func (s Schedule) Start() (sessions.Start, error) {
	args, err := sessions.SplitArgs(s.Args)
	if err != nil {
		return sessions.Start{}, err
	}
	start := sessions.Start{Model: s.Model, Args: args, DenyPrompts: s.Denies()}
	if _, err := sessions.StartArgs(s.Agent, start); err != nil {
		return sessions.Start{}, err
	}
	return start, nil
}

// denyPromptsOf is the switch a saved agent schedule keeps: what the form said, else what the agent schedule had (nil,
// off, for one from before the switch), else on for a new one — a schedule of another kind made an agent one included.
func denyPromptsOf(asked *bool, old *Schedule) *bool {
	switch {
	case asked != nil:
		on := *asked
		return &on
	case old != nil && old.Kind == KindAgent:
		return old.DenyPrompts
	}
	on := true
	return &on
}
