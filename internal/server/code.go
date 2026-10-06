package server

import "net/http"

// Code mode: a machine's code-server behind the guard.

// GET /api/code/{machine} → whether it answers (cached), and the port
func (s *Server) handleCodeStatus(w http.ResponseWriter, r *http.Request) {
	mc, err := s.Machines.Get(r.PathValue("machine"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	if r.URL.Query().Get("check") != "" {
		s.Code.Forget(mc.Cfg.ID)
	}
	writeJSON(w, s.Code.Status(mc))
}

// /code/{machine}/… → the editor
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
