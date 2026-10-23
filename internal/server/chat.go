package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// The chat lens: a pane's conversation, or a transcript on its own by folder and session id.

func (s *Server) chatRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/chat", s.handleChat)
}

var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,80}$`)

const maxLastTurns = 200

// handleChat is GET /api/chat?machine&pane[&session] or ?machine&dir&session, with an ETag per file version; last=N answers the last N turns only (a wall tile).
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	paneID, sessionID, dir := r.URL.Query().Get("pane"), r.URL.Query().Get("session"), r.URL.Query().Get("dir")
	if sessionID != "" && !sessionIDRe.MatchString(sessionID) {
		httpErr(w, 400, fmt.Errorf("bad session id"))
		return
	}
	if paneID == "" && (dir == "" || sessionID == "") {
		httpErr(w, 400, fmt.Errorf("pane, or dir and session, required"))
		return
	}
	last := 0
	if v := r.URL.Query().Get("last"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLastTurns {
			httpErr(w, 400, fmt.Errorf("last must be 1..%d", maxLastTurns))
			return
		}
		last = n
	}
	conv, err := s.Chat.Open(mc, paneID, sessionID, dir, last > 0)
	var notFound *chat.NotFoundError
	switch {
	case errors.As(err, &notFound):
		httpErr(w, 404, err)
		return
	case err != nil:
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	if paneID != "" && last == 0 { // a tile's tail is not the person's look at the pane: the lens is
		s.Radar.Seen(mc.ID(), paneID)
	}
	if conv.Path == "" {
		writeJSON(w, map[string]any{"sessions": []any{}, "turns": []any{}})
		return
	}
	etag := conv.ETag()
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	var turns []transcript.Turn
	if last > 0 {
		turns, err = conv.LastTurns(last)
	} else {
		turns, err = conv.Turns()
	}
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	sessions := conv.Sessions
	if sessions == nil {
		sessions = []transcript.SessionInfo{}
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"session_id": conv.ID, "sessions": sessions, "turns": turns})
}
