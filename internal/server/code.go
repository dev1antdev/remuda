package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/code"
)

// Code mode: a machine's code-server behind the guard.

func (s *Server) codeRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/code/{machine}", s.handleCodeStatus)
	api.HandleFunc("POST /api/code/{machine}/open", s.handleCodeOpen)
	api.HandleFunc("POST /api/code/{machine}/workspace", s.handleCodeWorkspace)
	api.HandleFunc("POST /api/code/{machine}/theme", s.handleCodeTheme)
	for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		api.HandleFunc(method+" /code/{machine}", s.handleCodeProxy)
		api.HandleFunc(method+" /code/{machine}/", s.handleCodeProxy)
	}
}

// handleCodeStatus is GET /api/code/{machine}: whether it answers (cached), and the port.
func (s *Server) handleCodeStatus(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("machine"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if r.URL.Query().Get("check") != "" {
		s.Code.Forget(mc.ID())
	}
	writeJSON(w, s.Code.Status(mc))
}

// handleCodeOpen is POST /api/code/{machine}/open {path, line}: the file opens in the editor already on screen there.
func (s *Server) handleCodeOpen(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("machine"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	var req struct {
		Path string `json:"path"`
		Line int    `json:"line"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !strings.HasPrefix(req.Path, "/") || strings.ContainsAny(req.Path, "\n\r") || req.Line < 0 {
		httpErr(w, 400, fmt.Errorf("path must be absolute"))
		return
	}
	if err := s.Code.Open(mc, req.Path, req.Line); err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleCodeWorkspace is POST /api/code/{machine}/workspace: the machine's workspace folders written into its one
// .code-workspace file for the editor, and that file's path.
func (s *Server) handleCodeWorkspace(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("machine"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	snap, _ := s.Watch.Snapshot(mc.ID())
	var folders []string
	if snap != nil {
		folders = snap.WorkspaceDirs()
	}
	if len(folders) == 0 {
		httpErr(w, 409, fmt.Errorf("no workspace on %s yet", mc.Label()))
		return
	}
	path, err := s.Code.Workspace(mc, folders)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"path": path, "folders": folders})
}

// handleCodeTheme is POST /api/code/{machine}/theme {theme}: the editor there wears the page's Catppuccin flavour.
func (s *Server) handleCodeTheme(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("machine"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	var req struct {
		Theme string `json:"theme"`
	}
	if !decode(w, r, &req) {
		return
	}
	switch req.Theme {
	case "latte", "frappe", "macchiato", "mocha", "dark", "light":
	default:
		httpErr(w, 400, fmt.Errorf("theme must be latte, frappe, macchiato or mocha"))
		return
	}
	installed, err := s.Code.Theme(mc, req.Theme)
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"theme": code.ThemeName(req.Theme), "installed": installed})
}

// handleCodeProxy serves /code/{machine}/… from the machine's code-server.
func (s *Server) handleCodeProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("machine")
	mc, err := s.Machines.Get(id)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	prefix := "/code/" + id
	if r.URL.Path == prefix {
		http.Redirect(w, r, prefix+"/"+optQuery(r), http.StatusTemporaryRedirect)
		return
	}
	s.Code.Proxy(w, r, mc, prefix)
}

func optQuery(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}
