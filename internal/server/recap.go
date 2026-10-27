package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/recap"
)

// What a conversation came to, and the standup over every agent: read from the transcripts on demand.

func (s *Server) recapRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/recap", s.handleRecap)
	api.HandleFunc("GET /api/standup", s.handleStandup)
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
	summary, err := s.Recap.Conversation(mc, paneID, sessionID, dir, since)
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

// handleStandup is GET /api/standup?tz=<IANA>[&since=RFC3339]: every agent on every connected machine with its recap since local midnight, and the report as Markdown.
func (s *Server) handleStandup(w http.ResponseWriter, r *http.Request) {
	loc := time.Local
	if tz := r.URL.Query().Get("tz"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	since, ok := sinceParam(w, r)
	if !ok {
		return
	}
	now := time.Now().In(loc)
	if since.IsZero() {
		since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}
	items := s.Recap.Standup(since)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"since": sinceText(since), "items": items, "markdown": recap.Markdown(items, since.In(loc), now)})
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
