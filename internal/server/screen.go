package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/scrollback"
)

// The wall's screen view: a pane's terminal as text with its colours, read from herdr on demand at the tiles' pace.
// The terminal lens's find over the scrollback reads the pane's rows here too, and scrolls the pane to a match.

// MaxScrollbackRows bounds one find's read; the page asks for fewer.
const MaxScrollbackRows = 20000

func (s *Server) screenRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/screen", s.handleScreen)
	api.HandleFunc("GET /api/scrollback", s.handleScrollback)
	api.HandleFunc("POST /api/scroll", s.handleScroll)
}

// handleScrollback is GET /api/scrollback?machine&pane&q&lines: where q occurs in the pane's last rows (herdr's
// scrollback and the screen), how many rows were read, whether earlier ones were left out, and where the viewport
// stands by the watcher's last snapshot.
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
	mc, client, _, ok := s.ready(w, r, "")
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
	var scroll herdr.Scroll
	if snap, err := s.Watch.Snapshot(mc.ID()); err == nil && snap != nil {
		for _, pane := range snap.Panes {
			if pane.PaneID == paneID {
				scroll = pane.Scroll
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"matches": matches, "rows": len(rows), "truncated": truncated, "scroll": scroll})
}

// handleScroll is POST /api/scroll {machine, pane, offset}: the pane's viewport moved to offset rows above the bottom —
// every viewer's, as the wheel in the lens does — answering where it stands.
func (s *Server) handleScroll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Machine string `json:"machine"`
		Pane    string `json:"pane"`
		Offset  int    `json:"offset"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Pane == "" || body.Offset < 0 {
		httpErr(w, 400, fmt.Errorf("pane and an offset of zero or more required"))
		return
	}
	_, client, _, ok := s.ready(w, r, body.Machine)
	if !ok {
		return
	}
	scroll, err := client.PaneScroll(body.Pane, body.Offset)
	if err != nil {
		rpcErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"scroll": scroll})
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
