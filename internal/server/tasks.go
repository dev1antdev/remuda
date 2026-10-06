package server

import (
	"errors"
	"net/http"
	"strings"
)

// Tasks: the Vikunja connection. The status and the connect/disconnect
// routes are herdr-ui's; everything under /api/tasks/v1/ is Vikunja's own
// API, proxied with the user's token added (PATCH merges, see tasks.Proxy).

func (s *Server) handleTasksStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("check") != "" {
		writeJSON(w, s.Tasks.Check())
		return
	}
	writeJSON(w, s.Tasks.Status())
}

func (s *Server) handleTasksConnect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		return
	}
	st, err := s.Tasks.Configure(req.URL, req.Token)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, st)
}

func (s *Server) handleTasksDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.Tasks.Disconnect(); err != nil {
		httpErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, s.Tasks.Status())
}

func (s *Server) handleTasksProxy(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/v1/")
	if path == "" || strings.Contains(path, "..") {
		httpErr(w, http.StatusBadRequest, errors.New("bad path"))
		return
	}
	s.Tasks.Proxy(w, r, path)
}
