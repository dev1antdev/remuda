package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
)

// agentKeyRoutes are the agents' keys; `/api/keys` itself is the keystroke route of the chat lens.
func (s *Server) agentKeyRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/agent-keys", s.handleAgentKeys)
	api.HandleFunc("DELETE /api/agent-keys/{id}", s.handleAgentKeyRevoke)
}

// keyView is an agent key as the page sees it: the machine it opens tasks for and when it was used, never the hash.
type keyView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Scope    string    `json:"scope"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
}

// keys is the store behind the guard, or a 503 when this remuda keeps none.
func (s *Server) keys(w http.ResponseWriter) (*auth.Keys, bool) {
	if s.Keys == nil {
		httpErr(w, http.StatusServiceUnavailable, errors.New("this remuda keeps no agent keys"))
		return nil, false
	}
	return s.Keys, true
}

// handleAgentKeys is GET /api/agent-keys: every agent key, the most recently used first. A key caller never gets here: the guard keeps it to tasks.
func (s *Server) handleAgentKeys(w http.ResponseWriter, r *http.Request) {
	store, ok := s.keys(w)
	if !ok {
		return
	}
	list := store.List()
	out := make([]keyView, 0, len(list))
	for _, key := range list {
		out = append(out, keyView{ID: key.ID, Name: key.Name, Scope: key.Scope, Created: key.Created, LastUsed: key.LastUsed})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, out)
}

// handleAgentKeyRevoke is DELETE /api/agent-keys/{id}: the key is refused from then on; the machine gets a new one when remuda next connects to it.
func (s *Server) handleAgentKeyRevoke(w http.ResponseWriter, r *http.Request) {
	store, ok := s.keys(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !store.Revoke(id) {
		httpErr(w, 404, errors.New("no such key"))
		return
	}
	writeJSON(w, map[string]any{"revoked": id})
}
