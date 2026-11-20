package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/recap"
)

// What a conversation came to: read from the transcript on demand.

func (s *Server) recapRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/recap", s.handleRecap)
}

// handleRecap is GET /api/recap?machine&pane[&session] or ?machine&dir&session [&since=RFC3339]: one conversation's recap, and the report as Markdown.
func (s *Server) handleRecap(w http.ResponseWriter, r *http.Request) {
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
	since, ok := sinceParam(w, r)
	if !ok {
		return
	}
	read := s.Recap.Conversation
	if auth.IsViewer(r.Context()) {
		read = s.Recap.ConversationOwn
	}
	summary, err := read(mc, paneID, sessionID, dir, since)
	var notFound *chat.NotFoundError
	switch {
	case errors.As(err, &notFound):
		httpErr(w, 404, err)
		return
	case err != nil:
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"since": sinceText(since), "recap": summary, "markdown": recap.Conversation(summary, since.In(time.Local), time.Now())})
}

// sinceParam reads since=<RFC3339>; zero when absent, a 400 when it does not parse.
func sinceParam(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	v := r.URL.Query().Get("since")
	if v == "" {
		return time.Time{}, true
	}
	since, err := time.Parse(time.RFC3339, v)
	if err != nil {
		httpErr(w, 400, fmt.Errorf("since must be RFC 3339"))
		return time.Time{}, false
	}
	return since, true
}

func sinceText(since time.Time) string {
	if since.IsZero() {
		return ""
	}
	return since.UTC().Format(time.RFC3339)
}
