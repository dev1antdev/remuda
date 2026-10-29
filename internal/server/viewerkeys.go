package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
)

// viewerKeyRoutes are the viewer links: a key that signs a TV or a colleague in read-only, minted, listed and revoked
// in Settings; the key itself is answered once, as a link, and never again.
func (s *Server) viewerKeyRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/viewer-keys", s.handleViewerKeys)
	api.HandleFunc("POST /api/viewer-keys", s.handleViewerKeyMint)
	api.HandleFunc("DELETE /api/viewer-keys/{id}", s.handleViewerKeyRevoke)
}

// viewerKeyView is a viewer key as the page sees it: its name, its first day, its last use, never the hash.
type viewerKeyView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
}

// handleViewerKeys is GET /api/viewer-keys: every viewer key, the most recently used first.
func (s *Server) handleViewerKeys(w http.ResponseWriter, r *http.Request) {
	store, ok := s.keys(w)
	if !ok {
		return
	}
	out := make([]viewerKeyView, 0)
	for _, key := range store.List() {
		if key.Scope == auth.ScopeViewer {
			out = append(out, viewerKeyView{ID: key.ID, Name: key.Name, Created: key.Created, LastUsed: key.LastUsed})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, out)
}

// handleViewerKeyMint is POST /api/viewer-keys {name}: a new viewer key under that name and the link that signs a
// browser in with it, shown this once.
func (s *Server) handleViewerKeyMint(w http.ResponseWriter, r *http.Request) {
	store, ok := s.keys(w)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		httpErr(w, 400, errors.New("a name for the link, please: whose screen it is"))
		return
	}
	if len(name) > 80 {
		name = name[:80]
	}
	key, rec, err := store.MintViewer(name)
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"id": rec.ID, "name": rec.Name, "created": rec.Created, "link": viewerLink(r, key)})
}

// viewerLink is the page's address with the key, for the browser that should view: the scheme and host the request came through.
func viewerLink(r *http.Request, key string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/?viewer=%s", scheme, r.Host, key)
}

// handleViewerKeyRevoke is DELETE /api/viewer-keys/{id}: the key is refused from then on, and every screen it signed in is signed out.
func (s *Server) handleViewerKeyRevoke(w http.ResponseWriter, r *http.Request) {
	store, ok := s.keys(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !store.Revoke(id) {
		httpErr(w, 404, errors.New("no such link"))
		return
	}
	screens := 0
	if s.Guard != nil && s.Guard.Devices != nil {
		screens = s.Guard.Devices.RevokeByKey(id)
	}
	writeJSON(w, map[string]any{"revoked": id, "screens": screens})
}
