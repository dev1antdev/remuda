package server

import (
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/chat"
)

// An agent's question as its screen shows it, for the chat's choice buttons: read from herdr on demand while the pane waits.

func (s *Server) questionRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/question", s.handleQuestion)
}

// handleQuestion is GET /api/question?machine&pane: the dialog on the pane's screen with its choices, null when the screen shows
// none, and the permission mode Claude Code's footer names, left out when no footer is on screen.
func (s *Server) handleQuestion(w http.ResponseWriter, r *http.Request) {
	paneID := r.URL.Query().Get("pane")
	if paneID == "" {
		httpErr(w, 400, fmt.Errorf("pane required"))
		return
	}
	_, client, _, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	screen, err := client.PaneRead(paneID)
	if err != nil {
		rpcErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	answer := map[string]any{"question": nil}
	if q, ok := chat.ParseQuestion(screen); ok {
		answer["question"] = q
	}
	if mode, ok := chat.ParseMode(screen); ok {
		answer["mode"] = mode
	}
	writeJSON(w, answer)
}
