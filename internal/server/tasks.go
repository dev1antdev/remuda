package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
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
	api.HandleFunc("POST /api/tasks/import", s.handleTaskImport)
}

// handleTaskList is GET /api/tasks?view=today|upcoming|open|project&project=&done=1&q=&page=&tz=.
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

// handleTaskStart is POST /api/tasks/{id}/start {machine, workspace, branch, base, agent, groom, prompt}:
// a worktree off the workspace, the agent in it, the task's prompt handed over in the background.
func (s *Server) handleTaskStart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Machine string `json:"machine"`
		tasks.StartRequest
	}
	if !decode(w, r, &req) {
		return
	}
	task, err := s.Tasks.Get(id)
	if err != nil {
		s.taskReply(w, nil, err)
		return
	}
	if err := req.StartRequest.Normalize(); err != nil {
		s.taskReply(w, nil, err)
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	task, err = s.Launcher.Start(mc, client, task, req.StartRequest)
	var refusal *herdr.Error
	switch {
	case errors.Is(err, tasks.ErrAgentStart):
		httpErr(w, http.StatusBadGateway, err)
	case errors.As(err, &refusal):
		rpcErr(w, err)
	default:
		s.taskReply(w, task, err)
	}
}

// handleTaskImport is POST /api/tasks/import {machine, project_id, url}: a GitHub issue read with gh on the machine, made a task of the project.
func (s *Server) handleTaskImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine   string `json:"machine"`
		ProjectID int64  `json:"project_id"`
		URL       string `json:"url"`
	}
	if !decode(w, r, &req) {
		return
	}
	if _, _, _, ok := tasks.IssueURL(req.URL); !ok {
		httpErr(w, http.StatusBadRequest, errors.New("a GitHub issue address looks like https://github.com/owner/repo/issues/123"))
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	issue, err := tasks.FetchIssue(mc, req.URL)
	if errors.Is(err, tasks.ErrIssueFetch) {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	if err != nil {
		s.taskReply(w, nil, err)
		return
	}
	task, err := tasks.ImportIssue(s.Tasks, req.ProjectID, issue)
	s.taskReply(w, task, err)
}
