package transcript

import (
	"encoding/json"
	"regexp"
	"strconv"
)

// The agent's plan, as its own tool calls wrote it: TodoWrite replaces the whole list, TaskCreate adds an
// item (numbered by Claude Code in its answer, "Task #3 created"), TaskUpdate moves or edits one. Read from
// the transcript alone, so a conversation on a remote machine has its plan too.

// PlanItem is one thing the agent set out to do.
type PlanItem struct {
	ID     string `json:"id"`
	Text   string `json:"text"`             // what to do: TodoWrite's content, TaskCreate's subject
	Active string `json:"active,omitempty"` // what the agent says while on it: activeForm
	Status string `json:"status"`           // pending | in_progress | completed
}

var planStatuses = map[string]bool{"pending": true, "in_progress": true, "completed": true}

var taskCreatedRe = regexp.MustCompile(`^Task #(\d+) created`)

// Plan is the agent's list as it stands after the turns: nil when it never kept one. An input the parser cut
// to a string, a status it does not know and an update to a task it never saw are left out.
func Plan(turns []Turn) []PlanItem {
	var items []PlanItem
	created := 0
	for _, turn := range turns {
		for _, part := range turn.Parts {
			if part.Type != "tool" {
				continue
			}
			switch part.ToolName {
			case "TodoWrite":
				if list, ok := todoList(part.Input); ok {
					items = list
				}
			case "TaskCreate":
				created++
				if item, ok := createdTask(part, created); ok {
					items = append(withoutTask(items, item.ID), item)
				}
			case "TaskUpdate":
				items = updatedTask(items, part.Input)
			case "update_plan": // Codex's: the whole list each time, a step and its status
				if list, ok := codexPlan(part.Input); ok {
					items = list
				}
			}
		}
	}
	return items
}

type todoInput struct {
	Todos []struct {
		ID         string `json:"id"`
		Content    string `json:"content"`
		Status     string `json:"status"`
		ActiveForm string `json:"activeForm"`
	} `json:"todos"`
}

// todoList is TodoWrite's whole list; the ids are the tool's own when it gives them, else each item's place.
func todoList(input json.RawMessage) ([]PlanItem, bool) {
	var in todoInput
	if json.Unmarshal(input, &in) != nil || in.Todos == nil {
		return nil, false
	}
	items := make([]PlanItem, 0, len(in.Todos))
	for i, todo := range in.Todos {
		if todo.Content == "" {
			continue
		}
		id := todo.ID
		if id == "" {
			id = strconv.Itoa(i + 1)
		}
		items = append(items, PlanItem{ID: id, Text: todo.Content, Active: todo.ActiveForm, Status: planStatus(todo.Status)})
	}
	return items, true
}

type codexPlanInput struct {
	Plan []struct {
		Step   string `json:"step"`
		Status string `json:"status"`
	} `json:"plan"`
}

// codexPlan is update_plan's whole list: each step numbered by its place, its status one of Codex's three (the same words).
func codexPlan(input json.RawMessage) ([]PlanItem, bool) {
	var in codexPlanInput
	if json.Unmarshal(input, &in) != nil || in.Plan == nil {
		return nil, false
	}
	items := make([]PlanItem, 0, len(in.Plan))
	for i, step := range in.Plan {
		if step.Step == "" {
			continue
		}
		items = append(items, PlanItem{ID: strconv.Itoa(i + 1), Text: step.Step, Status: planStatus(step.Status)})
	}
	return items, true
}

type taskInput struct {
	TaskID     string `json:"taskId"`
	Subject    string `json:"subject"`
	ActiveForm string `json:"activeForm"`
	Status     string `json:"status"`
}

// createdTask is TaskCreate's item: its id from the tool's answer ("Task #3 created…"), else its place among the creates.
func createdTask(part Part, nth int) (PlanItem, bool) {
	var in taskInput
	if json.Unmarshal(part.Input, &in) != nil || in.Subject == "" {
		return PlanItem{}, false
	}
	id := strconv.Itoa(nth)
	if m := taskCreatedRe.FindStringSubmatch(part.Output); m != nil {
		id = m[1]
	}
	return PlanItem{ID: id, Text: in.Subject, Active: in.ActiveForm, Status: "pending"}, true
}

// updatedTask applies TaskUpdate: a new status, subject or active form; "deleted" takes the task out.
func updatedTask(items []PlanItem, input json.RawMessage) []PlanItem {
	var in taskInput
	if json.Unmarshal(input, &in) != nil || in.TaskID == "" {
		return items
	}
	if in.Status == "deleted" {
		return withoutTask(items, in.TaskID)
	}
	for i := range items {
		if items[i].ID != in.TaskID {
			continue
		}
		if in.Subject != "" {
			items[i].Text = in.Subject
		}
		if in.ActiveForm != "" {
			items[i].Active = in.ActiveForm
		}
		if planStatuses[in.Status] {
			items[i].Status = in.Status
		}
	}
	return items
}

func withoutTask(items []PlanItem, id string) []PlanItem {
	var out []PlanItem
	for _, item := range items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	return out
}

// planStatus is one of the three the page knows; anything else reads as not started.
func planStatus(s string) string {
	if planStatuses[s] {
		return s
	}
	return "pending"
}
