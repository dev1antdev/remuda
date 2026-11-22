package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dev1antdev/remuda/internal/scrollback"
)

// The screen a client outside the page reads (/api/screen: the end-to-end checks) and the rows above it for the
// terminal lens's find (/api/scrollback), read through the machine's core on demand.

// MaxScrollbackRows bounds one find's read; the page asks for fewer.
const MaxScrollbackRows = 20000

func (s *Server) screenRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/screen", s.handleScreen)
	api.HandleFunc("GET /api/scrollback", s.handleScrollback)
}

// handleScrollback is GET /api/scrollback?pane&q&lines: the finds in the rows older than the terminal holds.
func (s *Server) handleScrollback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	paneID := q.Get("pane")
	if paneID == "" {
		httpErr(w, 400, fmt.Errorf("pane required"))
		return
	}
	lines, _ := strconv.Atoi(q.Get("lines"))
	if lines <= 0 || lines > MaxScrollbackRows {
		lines = MaxScrollbackRows
	}
	_, client, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	text, truncated, err := client.PaneRecent(paneID, lines)
	if err != nil {
		rpcErr(w, err)
		return
	}
	rows := scrollback.Rows(text)
	matches := scrollback.Find(rows, q.Get("q"))
	if matches == nil {
		matches = []scrollback.Match{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"matches": matches, "rows": len(rows), "truncated": truncated})
}

// handleScreen is GET /api/screen?pane: the pane's screen with its colours, with an ETag over the text.
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
