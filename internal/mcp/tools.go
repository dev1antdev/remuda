package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/tasks"
	"github.com/dev1antdev/remuda/internal/tasksapi"
)

// The tools: each takes its JSON arguments and answers Markdown the agent reads. `remuda tasks` prints the same.

var errNoTool = errors.New("no such tool")

// priorities are the words the page uses for 0–5.
var priorities = []string{"none", "low", "medium", "high", "urgent", "do now"}

// Tools is the task list as the tools see it.
type Tools struct {
	Client  *tasksapi.Client
	Source  string           // what a task made here is marked with: the agent's pane, from HERDR_PANE_ID
	Trouble error            // no door on this machine: every call answers the reason, so the agent can say so
	Now     func() time.Time // the clock, for the spans agents_list says; nil is time.Now
}

// Spec is one tool as tools/list describes it.
type Spec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func prop(kind, description string) map[string]any {
	return map[string]any{"type": kind, "description": description}
}

func schema(required []string, props map[string]any) map[string]any {
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// Specs lists the tools in the order an agent should think of them.
func Specs() []Spec {
	return []Spec{
		{Name: "tasks_list", Description: "List the person's tasks: what is open, due today or upcoming, in one project or all, optionally searched. Use it first, to see what already exists before creating anything, and to find a task's id.",
			InputSchema: schema(nil, map[string]any{
				"view":    map[string]any{"type": "string", "enum": []string{"open", "today", "upcoming"}, "description": "open (the default): every task not done; today: due today or overdue; upcoming: due in the coming days"},
				"project": prop("string", "a project's name or id; omit for every project"),
				"q":       prop("string", "words to search titles and descriptions for"),
				"done":    prop("boolean", "true includes the finished tasks of the project named (they are kept per project)"),
				"page":    prop("integer", "the page, from 1; the answer says how many there are"),
			})},
		{Name: "task_get", Description: "One task in full: description, column, due date, priority, labels, fields, comments. Use it before updating or commenting on a task.",
			InputSchema: schema([]string{"id"}, map[string]any{"id": prop("integer", "the task's id (the number after # in the list)")})},
		{Name: "task_create", Description: "Create a task: a piece of work to do, with a Markdown description saying what is wrong or missing, where, and what done looks like. One task per improvement, the title an imperative sentence. Check tasks_list first so nothing is filed twice.",
			InputSchema: schema([]string{"title", "project"}, map[string]any{
				"title":       prop("string", "a short imperative sentence"),
				"project":     prop("string", "the project's name or id (projects_list has them)"),
				"description": prop("string", "Markdown: the problem, where in the code, what done looks like"),
				"column":      prop("string", "the workflow column to put it in, by title; the project's new-task column when omitted"),
				"due":         prop("string", "a due date, YYYY-MM-DD or RFC 3339"),
				"priority":    prop("integer", "0 none, 1 low, 2 medium, 3 high, 4 urgent, 5 do now"),
				"labels":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "label names; a missing one is created"},
				"fields":      map[string]any{"type": "object", "description": "the project's fields by title, each with its value", "additionalProperties": true},
			})},
		{Name: "task_update", Description: "Change a task: move it to another column (or mark it done), retitle it, rewrite its description, set its due date or priority. Only the fields given change.",
			InputSchema: schema([]string{"id"}, map[string]any{
				"id":          prop("integer", "the task's id"),
				"title":       prop("string", "a new title"),
				"description": prop("string", "a new Markdown description, replacing the old"),
				"column":      prop("string", "the column to move it to, by title"),
				"done":        prop("boolean", "true marks it done, false reopens it"),
				"due":         prop("string", "YYYY-MM-DD or RFC 3339; an empty string clears it"),
				"priority":    prop("integer", "0–5"),
			})},
		{Name: "task_comment", Description: "Leave a note on a task: progress, a finding, a question for the person. Markdown.",
			InputSchema: schema([]string{"id", "body"}, map[string]any{"id": prop("integer", "the task's id"), "body": prop("string", "the note, Markdown")})},
		{Name: "projects_list", Description: "The projects with their workflow columns and fields: what task_create's project, column and fields may name.",
			InputSchema: schema(nil, map[string]any{})},
		{Name: "labels_list", Description: "The labels that exist, for task_create's labels.",
			InputSchema: schema(nil, map[string]any{})},
		{Name: "agents_list", Description: "Every coding agent on every machine the person runs: its workspace, state (working, waiting for the person, finished, idle, and for how long), the first line of its last answer, the pull request it named, what its conversation has cost. Look here before filing or taking a task, to see who is already on what; your own pane is marked (you).",
			InputSchema: schema(nil, map[string]any{})},
		{Name: "agent_recap", Description: "What one agent's conversation came to — prompts and answers, time, tokens and cost, the tools it called, the commands it ran, the files it edited and read, its last answer — as the person's Recap shows it.",
			InputSchema: schema([]string{"pane"}, map[string]any{
				"machine": prop("string", "the machine by name or id as agents_list shows it; omit for the cockpit's own"),
				"pane":    prop("string", "the pane as agents_list lists it, like w3:p1"),
				"since":   prop("string", "the period: midnight, 24h, 7d, a date YYYY-MM-DD or an RFC 3339 moment; omit for the whole conversation"),
			})},
		{Name: "standup", Description: "The standup over every agent: what each did and answered in the period, as the person's Standup panel shows it.",
			InputSchema: schema(nil, map[string]any{
				"since": prop("string", "the period: midnight (the default), 24h, 7d, a date YYYY-MM-DD or an RFC 3339 moment"),
			})},
	}
}

func specNames() map[string]bool {
	out := map[string]bool{}
	for _, s := range Specs() {
		out[s.Name] = true
	}
	return out
}

// Call runs the tool named with the arguments given.
func (t *Tools) Call(name string, args json.RawMessage) (string, error) {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	if !specNames()[name] {
		return "", fmt.Errorf("%w: %s", errNoTool, name)
	}
	if t.Trouble != nil {
		return "", t.Trouble
	}
	switch name {
	case "tasks_list":
		var a ListArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}
		return t.List(a)
	case "task_get":
		var a struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}
		return t.Get(a.ID)
	case "task_create":
		var a CreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}
		made, err := t.Create(a)
		if err != nil {
			return "", err
		}
		return Created(made), nil
	case "task_update":
		var a UpdateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}
		task, err := t.Update(a)
		if err != nil {
			return "", err
		}
		return t.Get(task.ID)
	case "task_comment":
		var a struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}
		return t.Comment(a.ID, a.Body)
	case "projects_list":
		return t.Projects()
	case "agents_list":
		return t.Agents()
	case "agent_recap":
		var a RecapArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}
		return t.AgentRecap(a)
	case "standup":
		var a StandupArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}
		return t.Standup(a)
	}
	return t.Labels()
}

type ListArgs struct {
	View    string `json:"view"`
	Project string `json:"project"`
	Q       string `json:"q"`
	Done    bool   `json:"done"`
	Page    int    `json:"page"`
}

// Query is the list query the arguments mean, the project resolved by name or id, with the projects read for it.
func (t *Tools) Query(a ListArgs) (tasksapi.Query, []tasks.Project, error) {
	projects, err := t.Client.Projects()
	if err != nil {
		return tasksapi.Query{}, nil, err
	}
	q := tasksapi.Query{View: a.View, Search: a.Q, Done: a.Done, Page: a.Page, TZ: time.Local.String()}
	if q.View == "" {
		q.View = "open"
	}
	if a.Done && a.Project == "" {
		return tasksapi.Query{}, nil, errors.New("the finished tasks are listed per project: name one")
	}
	if a.Project != "" {
		p, err := findProject(projects, a.Project)
		if err != nil {
			return tasksapi.Query{}, nil, err
		}
		q.Project, q.View = p.ID, "project"
	}
	return q, projects, nil
}

// List is tasks_list: a Markdown table of the page asked for.
func (t *Tools) List(a ListArgs) (string, error) {
	q, projects, err := t.Query(a)
	if err != nil {
		return "", err
	}
	list, pages, err := t.Client.List(q)
	if err != nil {
		return "", err
	}
	columns := columnNames(projects)
	names := projectNames(projects)
	var b strings.Builder
	fmt.Fprintf(&b, "%d task%s", len(list), plural(len(list)))
	if q.View == "project" {
		fmt.Fprintf(&b, " in %s", names[q.Project])
	} else {
		fmt.Fprintf(&b, " (%s)", q.View)
	}
	if pages > 1 {
		fmt.Fprintf(&b, ", page %d of %d", max(a.Page, 1), pages)
	}
	b.WriteString("\n\n")
	if len(list) == 0 {
		return b.String(), nil
	}
	b.WriteString("| id | project | column | title | due | priority | labels | source |\n|---|---|---|---|---|---|---|---|\n")
	for _, task := range list {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s | %s |\n", task.ID, cell(names[task.ProjectID]), cell(columns[task.StatusID]), cell(task.Title), day(task.DueDate), priority(task.Priority), cell(labelNames(task.Labels)), cell(task.Source))
	}
	return b.String(), nil
}

// Get is task_get: the task in full.
func (t *Tools) Get(id int64) (string, error) {
	if id <= 0 {
		return "", errors.New("task_get needs the task's id")
	}
	task, err := t.Client.Get(id)
	if err != nil {
		return "", err
	}
	projects, err := t.Client.Projects()
	if err != nil {
		return "", err
	}
	project, _ := findProject(projects, strconv.FormatInt(task.ProjectID, 10))
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s\n\n", task.Identifier, task.Title)
	fmt.Fprintf(&b, "- Project: %s\n- Column: %s%s\n", project.Title, columnNames(projects)[task.StatusID], done(task.Done))
	if task.DueDate != "" {
		fmt.Fprintf(&b, "- Due: %s\n", day(task.DueDate))
	}
	fmt.Fprintf(&b, "- Priority: %s\n", priority(task.Priority))
	if len(task.Labels) > 0 {
		fmt.Fprintf(&b, "- Labels: %s\n", labelNames(task.Labels))
	}
	if task.Source != "" {
		fmt.Fprintf(&b, "- Created by: %s\n", task.Source)
	}
	for _, f := range project.Fields {
		if v, ok := task.Fields[strconv.FormatInt(f.ID, 10)]; ok && v != nil {
			fmt.Fprintf(&b, "- %s: %v\n", f.Title, v)
		}
	}
	fmt.Fprintf(&b, "- Created: %s\n", day(task.Created))
	if text := tasks.HTMLToText(task.Description); text != "" {
		b.WriteString("\n" + text + "\n")
	}
	if len(task.Comments) > 0 {
		b.WriteString("\n## Comments\n\n")
		for _, c := range task.Comments {
			fmt.Fprintf(&b, "- %s (%s): %s\n", day(c.Created), c.Author, strings.ReplaceAll(strings.TrimSpace(c.Body), "\n", "\n  "))
		}
	}
	return b.String(), nil
}

type CreateArgs struct {
	Title       string         `json:"title"`
	Project     string         `json:"project"`
	Description string         `json:"description"`
	Column      string         `json:"column"`
	Due         string         `json:"due"`
	Priority    *int           `json:"priority"`
	Labels      []string       `json:"labels"`
	Fields      map[string]any `json:"fields"`
}

// Made is what task_create did: the task, the column it landed in, and a note per label that could not be put on it.
type Made struct {
	Task   tasksapi.Task
	Column *tasks.Status
	Notes  []string
}

// Create is task_create: the task made in the column asked for, else where the project puts an agent's tasks.
func (t *Tools) Create(a CreateArgs) (Made, error) {
	if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Project) == "" {
		return Made{}, errors.New("task_create needs a title and a project")
	}
	projects, err := t.Client.Projects()
	if err != nil {
		return Made{}, err
	}
	project, err := findProject(projects, a.Project)
	if err != nil {
		return Made{}, err
	}
	title := strings.TrimSpace(a.Title)
	w := tasksapi.Write{Title: &title, Source: t.Source}
	if a.Description != "" {
		w.Markdown = a.Description // the server renders it; a plain description beside it would win over the Markdown
	}
	if a.Column != "" {
		status, err := findColumn(project, a.Column)
		if err != nil {
			return Made{}, err
		}
		w.StatusID = &status.ID
	}
	if a.Due != "" {
		due, err := dueOf(a.Due)
		if err != nil {
			return Made{}, err
		}
		w.DueDate = &due
	}
	if a.Priority != nil {
		if *a.Priority < 0 || *a.Priority > 5 {
			return Made{}, errors.New("priority runs from 0 (none) to 5 (do now)")
		}
		w.Priority = a.Priority
	}
	if len(a.Fields) > 0 {
		w.Fields = map[string]json.RawMessage{}
		for name, value := range a.Fields {
			f, ok := findField(project, name)
			if !ok {
				return Made{}, fmt.Errorf("project %s has no field %q; its fields: %s", project.Title, name, fieldNames(project))
			}
			raw, _ := json.Marshal(value)
			w.Fields[strconv.FormatInt(f.ID, 10)] = raw
		}
	}
	task, err := t.Client.Create(project.ID, w)
	if err != nil {
		return Made{}, err
	}
	made := Made{Task: task, Column: columnOf(project, task.StatusID)}
	for _, name := range a.Labels {
		label, err := t.label(name)
		if err != nil {
			made.Notes = append(made.Notes, fmt.Sprintf("label %q: %v", name, err))
			continue
		}
		if _, err := t.Client.AddLabel(task.ID, label.ID); err != nil {
			made.Notes = append(made.Notes, fmt.Sprintf("label %q: %v", name, err))
		}
	}
	return made, nil
}

// Created is what task_create answers: the task and its column, and that a proposal waits for the person's word.
func Created(m Made) string {
	out := fmt.Sprintf("Created task %s (id %d) \"%s\"", m.Task.Identifier, m.Task.ID, m.Task.Title)
	if m.Column != nil {
		out += " in " + m.Column.Title
		if m.Column.OnAgent {
			out += " — the person accepts or declines it there; leave it to them"
		}
	}
	if len(m.Notes) > 0 {
		out += "\n" + strings.Join(m.Notes, "\n")
	}
	return out
}

// columnOf is the project's column with that id, nil for one it does not know.
func columnOf(p tasks.Project, id int64) *tasks.Status {
	for i := range p.Statuses {
		if p.Statuses[i].ID == id {
			return &p.Statuses[i]
		}
	}
	return nil
}

type UpdateArgs struct {
	ID          int64   `json:"id"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Column      string  `json:"column"`
	Done        *bool   `json:"done"`
	Due         *string `json:"due"`
	Priority    *int    `json:"priority"`
}

// Update is task_update: the task as it stands afterwards.
func (t *Tools) Update(a UpdateArgs) (tasksapi.Task, error) {
	if a.ID <= 0 {
		return tasksapi.Task{}, errors.New("task_update needs the task's id")
	}
	task, err := t.Client.Get(a.ID)
	if err != nil {
		return tasksapi.Task{}, err
	}
	w := tasksapi.Write{Title: a.Title, Done: a.Done, Priority: a.Priority}
	if a.Description != nil {
		w.Markdown = *a.Description
	}
	if a.Column != "" {
		projects, err := t.Client.Projects()
		if err != nil {
			return tasksapi.Task{}, err
		}
		project, err := findProject(projects, strconv.FormatInt(task.ProjectID, 10))
		if err != nil {
			return tasksapi.Task{}, err
		}
		status, err := findColumn(project, a.Column)
		if err != nil {
			return tasksapi.Task{}, err
		}
		w.StatusID = &status.ID
	}
	if a.Due != nil {
		due := ""
		if *a.Due != "" {
			if due, err = dueOf(*a.Due); err != nil {
				return tasksapi.Task{}, err
			}
		}
		w.DueDate = &due
	}
	if a.Priority != nil && (*a.Priority < 0 || *a.Priority > 5) {
		return tasksapi.Task{}, errors.New("priority runs from 0 (none) to 5 (do now)")
	}
	return t.Client.Patch(a.ID, w)
}

// Comment is task_comment.
func (t *Tools) Comment(id int64, body string) (string, error) {
	if id <= 0 || strings.TrimSpace(body) == "" {
		return "", errors.New("task_comment needs the task's id and a body")
	}
	c, err := t.Client.Comment(id, strings.TrimSpace(body))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Commented on task %d (comment %d)", id, c.ID), nil
}

// Projects is projects_list.
func (t *Tools) Projects() (string, error) {
	projects, err := t.Client.Projects()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var walk func(parent int64, depth int)
	walk = func(parent int64, depth int) {
		for _, p := range projects {
			if p.ParentID != parent || p.Archived {
				continue
			}
			fmt.Fprintf(&b, "%s- **%s** (id %d): columns %s", strings.Repeat("  ", depth), p.Title, p.ID, columnList(p))
			if len(p.Fields) > 0 {
				fmt.Fprintf(&b, "; fields %s", fieldNames(p))
			}
			b.WriteString("\n")
			walk(p.ID, depth+1)
		}
	}
	walk(0, 0)
	if b.Len() == 0 {
		return "No projects yet.", nil
	}
	return b.String(), nil
}

// Labels is labels_list.
func (t *Tools) Labels() (string, error) {
	labels, err := t.Client.Labels()
	if err != nil {
		return "", err
	}
	if len(labels) == 0 {
		return "No labels yet; task_create makes the ones it names.", nil
	}
	var b strings.Builder
	for _, l := range labels {
		fmt.Fprintf(&b, "- %s (id %d)\n", l.Title, l.ID)
	}
	return b.String(), nil
}

// label is the label named, made when missing.
func (t *Tools) label(name string) (tasks.Label, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return tasks.Label{}, errors.New("an empty name")
	}
	labels, err := t.Client.Labels()
	if err != nil {
		return tasks.Label{}, err
	}
	for _, l := range labels {
		if strings.EqualFold(l.Title, name) {
			return l, nil
		}
	}
	return t.Client.CreateLabel(name, "")
}

// --- naming things ---

func findProject(projects []tasks.Project, name string) (tasks.Project, error) {
	name = strings.TrimSpace(name)
	if id, err := strconv.ParseInt(strings.TrimPrefix(name, "#"), 10, 64); err == nil {
		for _, p := range projects {
			if p.ID == id {
				return p, nil
			}
		}
	}
	for _, p := range projects {
		if strings.EqualFold(p.Title, name) {
			return p, nil
		}
	}
	var titles []string
	for _, p := range projects {
		if !p.Archived {
			titles = append(titles, fmt.Sprintf("%s (id %d)", p.Title, p.ID))
		}
	}
	return tasks.Project{}, fmt.Errorf("no project %q; the projects: %s", name, strings.Join(titles, ", "))
}

func findColumn(p tasks.Project, title string) (tasks.Status, error) {
	for _, s := range p.Statuses {
		if strings.EqualFold(s.Title, strings.TrimSpace(title)) {
			return s, nil
		}
	}
	return tasks.Status{}, fmt.Errorf("project %s has no column %q; its columns: %s", p.Title, title, columnList(p))
}

func findField(p tasks.Project, title string) (tasks.Field, bool) {
	for _, f := range p.Fields {
		if strings.EqualFold(f.Title, strings.TrimSpace(title)) {
			return f, true
		}
	}
	return tasks.Field{}, false
}

func columnList(p tasks.Project) string {
	statuses := append([]tasks.Status(nil), p.Statuses...)
	sort.SliceStable(statuses, func(i, j int) bool { return statuses[i].Position < statuses[j].Position })
	var names []string
	for _, s := range statuses {
		names = append(names, s.Title)
	}
	return strings.Join(names, " → ")
}

func fieldNames(p tasks.Project) string {
	var names []string
	for _, f := range p.Fields {
		names = append(names, fmt.Sprintf("%s (%s)", f.Title, f.Kind))
	}
	return strings.Join(names, ", ")
}

func columnNames(projects []tasks.Project) map[int64]string {
	out := map[int64]string{}
	for _, p := range projects {
		for _, s := range p.Statuses {
			out[s.ID] = s.Title
		}
	}
	return out
}

func projectNames(projects []tasks.Project) map[int64]string {
	out := map[int64]string{}
	for _, p := range projects {
		out[p.ID] = p.Title
	}
	return out
}

func labelNames(labels []tasks.Label) string {
	var names []string
	for _, l := range labels {
		names = append(names, l.Title)
	}
	return strings.Join(names, ", ")
}

func priority(n int) string {
	if n >= 0 && n < len(priorities) {
		return priorities[n]
	}
	return strconv.Itoa(n)
}

func done(d bool) string {
	if d {
		return " (done)"
	}
	return ""
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// cell is a table cell: one line, the pipes escaped.
func cell(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "|", "\\|")
	return strings.TrimSpace(s)
}

// day is an RFC 3339 moment as a local date, or the text as it came.
func day(s string) string {
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Local().Format("2006-01-02")
	}
	return s
}

// dueOf is a due date as the list stores it: a date means the end of that day here, a moment is kept.
func dueOf(s string) (string, error) {
	s = strings.TrimSpace(s)
	if d, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return d.Add(24*time.Hour - time.Second).UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("a due date is YYYY-MM-DD or RFC 3339, not %q", s)
}
