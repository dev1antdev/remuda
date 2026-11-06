package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/jira"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// Jira: the person's site and token (the token never shown back), a test of them, and an issue made a task. Not an
// agent key's routes: an agent reads the chips on the roster, and Jira is asked only for what the person does here.

func (s *Server) jiraRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/jira", s.handleJira)
	api.HandleFunc("PUT /api/jira", s.handleJiraPut)
	api.HandleFunc("DELETE /api/jira", s.handleJiraDelete)
	api.HandleFunc("POST /api/jira/test", s.handleJiraTest)
	api.HandleFunc("POST /api/jira/import", s.handleJiraImport)
}

func (s *Server) handleJira(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Jira.Store.View())
}

func (s *Server) handleJiraPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Site    string `json:"site"`
		Account string `json:"account"`
		Token   string `json:"token"` // empty keeps the one kept
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.Jira.Replace(req.Site, req.Account, req.Token); err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, s.Jira.Store.View())
}

func (s *Server) handleJiraDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Jira.Clear(); err != nil {
		httpErr(w, http.StatusInternalServerError, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, s.Jira.Store.View())
}

// handleJiraTest is the Settings' Test: who the account is and how many projects it sees, or why not.
func (s *Server) handleJiraTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*jira.Timeout)
	defer cancel()
	who, projects, err := s.Jira.Test(ctx)
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "who": who, "projects": projects})
}

// handleJiraImport is POST /api/jira/import {project_id, key}: an issue read from the site, made a task of the project.
func (s *Server) handleJiraImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProjectID int64  `json:"project_id"`
		Key       string `json:"key"`
	}
	if !decode(w, r, &req) {
		return
	}
	key, ok := jira.KeyOf(req.Key)
	if !ok {
		httpErr(w, http.StatusBadRequest, errors.New("a Jira issue is named by its key, like CRMOPS-6279, or its address"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*jira.Timeout)
	defer cancel()
	full, err := s.Jira.Full(ctx, key)
	if errors.Is(err, jira.ErrNotFound) {
		httpErr(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	task, err := tasks.ImportFrom(s.Tasks, req.ProjectID, tasks.Imported{
		Title:    full.Key + " " + full.Summary,
		URL:      full.URL,
		Lead:     importLead(full),
		Markdown: full.Description,
		Labels:   full.Labels,
	})
	s.taskReply(w, task, err)
}

// importLead is what the task's first line says of the issue beside its address: its key, type, status and reporter.
func importLead(full jira.Full) string {
	parts := []string{full.Key}
	for _, part := range []string{full.Type, full.Status} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if full.Reporter != "" {
		parts = append(parts, "by "+full.Reporter)
	}
	return strings.Join(parts, " · ")
}
