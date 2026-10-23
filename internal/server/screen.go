package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
)

// The wall's screen view: a pane's terminal as text with its colours, read from herdr on demand at the tiles' pace.

func (s *Server) screenRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/screen", s.handleScreen)
}

// handleScreen is GET /api/screen?machine&pane: the pane's visible rows with their SGR colours, an ETag over the text so an unchanged screen is a 304.
func (s *Server) handleScreen(w http.ResponseWriter, r *http.Request) {
	paneID := r.URL.Query().Get("pane")
	if paneID == "" {
		httpErr(w, 400, fmt.Errorf("pane required"))
		return
	}
	_, client, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	screen, err := client.PaneScreen(paneID)
	if err != nil {
		rpcErr(w, err)
		return
	}
	sum := sha256.Sum256([]byte(screen.Text))
	etag := `"` + hex.EncodeToString(sum[:12]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"text": screen.Text, "truncated": screen.Truncated})
}
