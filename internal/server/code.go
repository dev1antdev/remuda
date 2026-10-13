package server

import "net/http"

// Code mode: a machine's code-server behind the guard.

func (s *Server) codeRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/code/{machine}", s.handleCodeStatus)
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
