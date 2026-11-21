package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dev1antdev/remuda/internal/scrollback"
)

// A pane's terminal as text with its colours, read from herdr on demand: a viewer link's terminal lens.
// The terminal lens's find over the scrollback reads the pane's rows here too, and scrolls the pane to a match. The
// screen terminal — a pane with no attach, a Windows machine's — is that screen read again and again, typed into here.

// MaxScrollbackRows bounds one find's read; the page asks for fewer.
const MaxScrollbackRows = 20000

func (s *Server) screenRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/screen", s.handleScreen)
	api.HandleFunc("GET /api/scrollback", s.handleScrollback)
}

// handleInput is POST /api/input {machine, pane, text?, paste?, keys?}: the screen terminal's typing, text then keys
// under the pane's lock; the activity gets a row per line it sends (sent.Lines), not one per keystroke.
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

// handleScroll is POST /api/scroll {machine, pane, offset}: the pane's viewport moved to offset rows above the bottom —
// every viewer's, as the wheel in the lens does — answering where it stands.
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
