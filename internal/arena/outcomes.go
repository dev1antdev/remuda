package arena

import "time"

// HistoryKeep is how many lanes of decided arenas the history holds, the oldest dropped first.
const HistoryKeep = 300

// Outcome is one lane of a decided arena: which agent and model ran it and whether the person kept it.
type Outcome struct {
	Arena  int       `json:"arena"`
	Lane   int       `json:"lane"`
	Agent  string    `json:"agent"`
	Model  string    `json:"model,omitempty"` // the model its answers came from when the meter read one, else the one asked for
	Kept   bool      `json:"kept,omitempty"`
	RunID  int64     `json:"run_id,omitempty"` // the task run the lane was, for a task's arena
	TaskID int64     `json:"task_id,omitempty"`
	At     time.Time `json:"at"` // when the arena was decided
}

// outcomesOf are an arena's lanes that ran an agent, with the model read for each lane where there is one.
func outcomesOf(a Arena, models map[int]string, at time.Time) []Outcome {
	var out []Outcome
	for _, lane := range a.Lanes {
		if lane.Pane == "" {
			continue // refused before its agent started: no run of the model
		}
		model := lane.Model
		if read := models[lane.ID]; read != "" {
			model = read
		}
		out = append(out, Outcome{Arena: a.ID, Lane: lane.ID, Agent: lane.Agent, Model: model, Kept: lane.ID == a.Kept, RunID: lane.RunID, TaskID: a.TaskID, At: at})
	}
	return out
}

// decided says whether the history holds the arena; the caller holds the lock.
func (st *Store) decided(arenaID int) bool {
	for _, o := range st.history {
		if o.Arena == arenaID {
			return true
		}
	}
	return false
}

func (st *Store) addOutcomes(outcomes []Outcome) {
	st.history = append(st.history, outcomes...)
	if len(st.history) > HistoryKeep {
		st.history = append([]Outcome(nil), st.history[len(st.history)-HistoryKeep:]...)
	}
}

// Decide notes a decided arena's lanes in the history, once: the models read off the meter by lane, else each lane's own.
func (st *Store) Decide(a Arena, models map[int]string, at time.Time) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if a.Kept == 0 || st.decided(a.ID) {
		return nil
	}
	st.addOutcomes(outcomesOf(a, models, at))
	return st.save()
}

// backfillOutcomes notes the arenas decided before the history, with the models they asked for; true when it added any.
func (st *Store) backfillOutcomes() bool {
	added := false
	for _, a := range st.arenas {
		if a.Kept != 0 && !st.decided(a.ID) {
			st.addOutcomes(outcomesOf(a, nil, a.Created))
			added = true
		}
	}
	return added
}

// Outcomes are the decided lanes of the arenas decided at or after since, the oldest first.
func (st *Store) Outcomes(since time.Time) []Outcome {
	st.mu.Lock()
	defer st.mu.Unlock()
	var out []Outcome
	for _, o := range st.history {
		if !o.At.Before(since) {
			out = append(out, o)
		}
	}
	return out
}
