package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// Tasks: remuda's own list (internal/tasks), plain JSON over SQLite.

func (s *Server) tasksRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/tasks/projects", func(w http.ResponseWriter, r *http.Request) {
		ps, err := s.Tasks.Projects()
		s.taskReply(w, ps, err)
	})
	api.HandleFunc("POST /api/tasks/projects", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title  string `json:"title"`
			Parent int64  `json:"parent_project_id"`
		}
		if !decode(w, r, &req) {
			return
		}
		p, err := s.Tasks.CreateProject(req.Title, req.Parent)
		s.taskReply(w, p, err)
	})
	api.HandleFunc("PATCH /api/tasks/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var p tasks.ProjectPatch
		if !decode(w, r, &p) {
			return
		}
		out, err := s.Tasks.UpdateProject(id, p)
		s.taskReply(w, out, err)
	})
	api.HandleFunc("DELETE /api/tasks/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			s.taskReply(w, map[string]bool{"ok": true}, s.Tasks.DeleteProject(id))
		}
	})
	// the workflow, replaced whole: {statuses: [...], moves: {"<old id>": <new id>}}
	api.HandleFunc("PUT /api/tasks/projects/{id}/statuses", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var req struct {
			Statuses []tasks.Status  `json:"statuses"`
			Moves    map[int64]int64 `json:"moves"`
		}
		if !decode(w, r, &req) {
			return
		}
		if _, err := s.Tasks.SetStatuses(id, req.Statuses, req.Moves); err != nil {
			s.taskReply(w, nil, err)
			return
		}
		p, err := s.Tasks.Project(id)
		s.taskReply(w, p, err)
	})
	// the project's own fields, replaced whole
	api.HandleFunc("PUT /api/tasks/projects/{id}/fields", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var req struct {
			Fields []tasks.Field `json:"fields"`
		}
		if !decode(w, r, &req) {
			return
		}
		if _, err := s.Tasks.SetFields(id, req.Fields); err != nil {
			s.taskReply(w, nil, err)
			return
		}
		p, err := s.Tasks.Project(id)
		s.taskReply(w, p, err)
	})
	api.HandleFunc("GET /api/tasks/labels", func(w http.ResponseWriter, r *http.Request) {
		ls, err := s.Tasks.Labels()
		s.taskReply(w, ls, err)
	})
	api.HandleFunc("POST /api/tasks/labels", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title    string `json:"title"`
			HexColor string `json:"hex_color"`
		}
		if !decode(w, r, &req) {
			return
		}
		l, err := s.Tasks.CreateLabel(req.Title, req.HexColor)
		s.taskReply(w, l, err)
	})
	api.HandleFunc("DELETE /api/tasks/labels/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			s.taskReply(w, map[string]bool{"ok": true}, s.Tasks.DeleteLabel(id))
		}
	})
	api.HandleFunc("GET /api/tasks", s.handleTaskList)
	api.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ProjectID int64 `json:"project_id"`
			tasks.Patch
		}
		if !decode(w, r, &req) {
			return
		}
		t, err := s.Tasks.Create(req.ProjectID, req.Patch)
		s.taskReply(w, t, err)
	})
	api.HandleFunc("GET /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			t, err := s.Tasks.Get(id)
			s.taskReply(w, t, err)
		}
	})
	api.HandleFunc("PATCH /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var p tasks.Patch
		if !decode(w, r, &p) {
			return
		}
		t, err := s.Tasks.Update(id, p)
		s.taskReply(w, t, err)
	})
	api.HandleFunc("DELETE /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			s.taskReply(w, map[string]bool{"ok": true}, s.Tasks.Delete(id))
		}
	})
	api.HandleFunc("POST /api/tasks/{id}/start", s.handleTaskStart)
	api.HandleFunc("POST /api/tasks/{id}/comments", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var req struct {
			Body string `json:"body"`
		}
		if !decode(w, r, &req) {
			return
		}
		c, err := s.Tasks.AddComment(id, "you", req.Body)
		s.taskReply(w, c, err)
	})
	api.HandleFunc("DELETE /api/tasks/comments/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			s.taskReply(w, map[string]bool{"ok": true}, s.Tasks.DeleteComment(id))
		}
	})
	api.HandleFunc("PUT /api/tasks/{id}/labels", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var req struct {
			LabelID int64 `json:"label_id"`
		}
		if !decode(w, r, &req) {
			return
		}
		if err := s.Tasks.AddLabel(id, req.LabelID); err != nil {
			s.taskReply(w, nil, err)
			return
		}
		t, err := s.Tasks.Get(id)
		s.taskReply(w, t, err)
	})
	api.HandleFunc("DELETE /api/tasks/{id}/labels/{label}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		label, ok := pathID(w, r, "label")
		if !ok {
			return
		}
		if err := s.Tasks.RemoveLabel(id, label); err != nil {
			s.taskReply(w, nil, err)
			return
		}
		t, err := s.Tasks.Get(id)
		s.taskReply(w, t, err)
	})
}

// GET /api/tasks?view=today|upcoming|open|project&project=&done=1&q=&page=&tz=
func (s *Server) handleTaskList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	project, _ := strconv.ParseInt(q.Get("project"), 10, 64)
	page, _ := strconv.Atoi(q.Get("page"))
	loc := time.Local
	if tz := q.Get("tz"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	view := q.Get("view")
	if view == "" {
		view = "open"
	}
	list, pages, err := s.Tasks.List(tasks.Query{View: view, Project: project, Done: q.Get("done") == "1", Search: q.Get("q"), Page: page, Loc: loc})
	if err != nil {
		s.taskReply(w, nil, err)
		return
	}
	writeJSON(w, map[string]any{"tasks": list, "pages": pages})
}

func (s *Server) taskReply(w http.ResponseWriter, v any, err error) {
	switch {
	case err == nil:
		writeJSON(w, v)
	case errors.Is(err, tasks.ErrNotFound):
		httpErr(w, http.StatusNotFound, err)
	case errors.Is(err, tasks.ErrInvalid):
		httpErr(w, http.StatusBadRequest, err)
	default:
		httpErr(w, http.StatusInternalServerError, err)
	}
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		httpErr(w, http.StatusBadRequest, errors.New("bad id"))
		return 0, false
	}
	return id, true
}

// POST /api/tasks/{id}/start {machine, workspace, branch, base, agent, groom, prompt}
// — a worktree off the workspace, the agent in it, the task's prompt once
// the agent is up (in the background: a fresh agent takes seconds, and may
// stop at a question of its own first). The run is on the task from now on.
func (s *Server) handleTaskStart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Machine   string `json:"machine"`
		Workspace string `json:"workspace"`
		Branch    string `json:"branch"`
		Base      string `json:"base"`
		Agent     string `json:"agent"`
		Groom     bool   `json:"groom"`
		Prompt    string `json:"prompt"` // the text as edited; empty = composed here
	}
	if !decode(w, r, &req) {
		return
	}
	task, err := s.Tasks.Get(id)
	if err != nil {
		s.taskReply(w, nil, err)
		return
	}
	if req.Agent == "" {
		req.Agent = "claude"
	}
	if req.Agent == "shell" || !agentOK(req.Agent) {
		httpErr(w, http.StatusBadRequest, fmt.Errorf("an agent is required, not %q", req.Agent))
		return
	}
	req.Branch = strings.TrimSpace(req.Branch)
	if req.Workspace == "" || req.Branch == "" {
		httpErr(w, http.StatusBadRequest, errors.New("a workspace and a branch are required"))
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	res, err := client.WorktreeCreate(req.Workspace, req.Branch, strings.TrimSpace(req.Base), "")
	if err != nil {
		rpcErr(w, err)
		return
	}
	pane := res.RootPane.PaneID
	if err := startAgent(client, pane, req.Agent); err != nil {
		s.Watch.Refresh(mc)
		httpErr(w, http.StatusBadGateway, fmt.Errorf("the worktree %s is there, but the agent did not start: %w", res.Worktree.Branch, err))
		return
	}
	s.Watch.Refresh(mc)
	run, err := s.Tasks.StartRun(tasks.Run{TaskID: id, Machine: mc.Cfg.ID, Workspace: res.Workspace.WorkspaceID, Pane: pane, Dir: res.Worktree.Path, Branch: res.Worktree.Branch, Agent: req.Agent})
	if err != nil {
		s.taskReply(w, nil, err)
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		project, _ := s.Tasks.Project(task.ProjectID) // its title and fields; a missing one leaves them out
		prompt = tasks.Prompt(task, project, req.Groom)
	} else if req.Groom && !strings.HasPrefix(prompt, "/grill-me") {
		prompt = "/grill-me " + prompt
	}
	go s.deliverTaskPrompt(mc, client, run, task, prompt)
	t, err := s.Tasks.Get(id)
	s.taskReply(w, t, err)
}

// deliverTaskPrompt waits for the fresh agent and hands it the task. Claude
// Code asks whether to trust a folder it has not seen — this worktree was
// made a moment ago from the person's own repository, so that one question
// is answered yes here (and noted on the task). Any other question of the
// agent's is pushed to the person once and waited for; after five minutes
// the run is failed.
func (s *Server) deliverTaskPrompt(mc *machines.Machine, client *herdr.Client, run tasks.Run, task tasks.Task, prompt string) {
	lock := s.paneLock(push.PaneRef{Machine: mc.Cfg.ID, Pane: run.Pane})
	deadline := time.Now().Add(5 * time.Minute)
	nudged, trusted := false, false
	var last error
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		lock.Lock()
		err := client.AgentPrompt(run.Pane, prompt)
		lock.Unlock()
		switch {
		case err == nil:
			_ = s.Tasks.SetRun(run.ID, tasks.RunRunning, "")
			return
		case herdr.IsCode(err, "agent_not_ready") || herdr.IsCode(err, "agent_not_found"):
			last = err // still coming up
		case herdr.IsCode(err, "agent_blocked"):
			last = err
			if !trusted {
				if screen, rerr := client.PaneRead(run.Pane); rerr == nil && isTrustPrompt(screen, run.Dir, mc.Home()) {
					trusted = true
					lock.Lock()
					if !cursorOnYes(screen) { // "No, exit" sits first and focused; older builds put Yes first
						_ = client.PaneSendKeys(run.Pane, []string{"down"})
						time.Sleep(150 * time.Millisecond)
					}
					_ = client.PaneSendKeys(run.Pane, []string{"enter"})
					lock.Unlock()
					_, _ = s.Tasks.AddComment(run.TaskID, "remuda", "Answered Claude Code's trust question for the new worktree `"+run.Dir+"`.")
					continue
				}
			}
			if !nudged && s.Push != nil {
				nudged = true
				s.Push.Notify(push.Payload{
					Machine: mc.Cfg.ID, Pane: run.Pane, Kind: "input",
					Title:   "Task " + task.Identifier + ": the agent asks something first",
					Body:    "Answer it in the pane and the task follows.",
					Tag:     fmt.Sprintf("task|%d", run.TaskID),
					Actions: []push.Action{{Action: "open", Title: "Open"}},
				})
			}
		default:
			s.failTaskRun(run, "The task could not be handed to the agent: "+err.Error())
			return
		}
	}
	msg := "the agent did not take the prompt within five minutes"
	if last != nil {
		msg += " (" + last.Error() + ")"
	}
	s.failTaskRun(run, "The task could not be handed to the agent: "+msg+". The worktree is there; send it by hand.")
}

// failTaskRun closes a starting run as failed — unless the sweep closed it
// meanwhile (a pane gone while starting is one comment, not two).
func (s *Server) failTaskRun(run tasks.Run, note string) {
	if cur, err := s.Tasks.Run(run.ID); err != nil || cur.State != tasks.RunStarting {
		return
	}
	_ = s.Tasks.SetRun(run.ID, tasks.RunFailed, note)
	_, _ = s.Tasks.AddComment(run.TaskID, "remuda", note)
}

// isTrustPrompt recognises Claude Code's "do you trust this folder" screen
// for the given folder — nothing else gets an answer on the person's
// behalf. The dialog is drawn in a box and wrapped at the pane's width, so
// the path may be split over lines between border runes: whitespace and
// box drawing are dropped before matching, and the path may be shown with
// the home folder as "~".
func isTrustPrompt(screen, dir, home string) bool {
	flat := squash(screen)
	if !strings.Contains(flat, "trustthisfolder") || !strings.Contains(flat, "Accessingworkspace") {
		return false
	}
	if dir == "" {
		return true
	}
	want := []string{squash(dir)}
	if home != "" && (dir == home || strings.HasPrefix(dir, home+"/")) {
		want = append(want, squash("~"+strings.TrimPrefix(dir, home)))
	}
	for _, w := range want {
		if strings.Contains(flat, w) {
			return true
		}
	}
	return false
}

// cursorOnYes says whether the dialog's cursor already sits on the yes line.
func cursorOnYes(screen string) bool {
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "❯") && strings.Contains(line, "Yes, I trust") {
			return true
		}
	}
	return false
}

// squash drops whitespace and box-drawing runes.
func squash(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' || (r >= 0x2500 && r <= 0x257F) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
