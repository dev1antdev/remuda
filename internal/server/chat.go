package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// The chat lens: a pane's conversation, or a transcript on its own by folder and session id.

func (s *Server) chatRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/chat", s.handleChat)
}

var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,80}$`)

// looksAtPane says whether a read of a pane's conversation is the person's look at it, which brings its finish down and
// keeps that finish's push from going out: the chat lens on a page they see. A tail is none, nor a viewer's screen,
// nor a page whose own document is hidden and says so with look=0 — the Mac app polls on with its window closed, for
// its menu bar.
func looksAtPane(r *http.Request, paneID string, last int) bool {
	return paneID != "" && last == 0 && r.URL.Query().Get("look") != "0" && !auth.IsViewer(r.Context())
}

const maxLastTurns = 200

// handleChat is GET /api/chat?machine&pane[&session] or ?machine&dir&session, with an ETag per file version; last=N answers the
// last N turns only (Code's follow); subagent=<id> answers one of the conversation's subagents on its own, read-only.
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
	subagent := r.URL.Query().Get("subagent")
	if subagent != "" && !transcript.SubagentID(subagent) {
		httpErr(w, 400, fmt.Errorf("bad subagent id"))
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
	open := s.Chat.Open
	if auth.IsViewer(r.Context()) {
		open = s.Chat.OpenOwn
	}
	conv, err := open(mc, paneID, sessionID, dir, last > 0 || subagent != "")
	var notFound *chat.NotFoundError
	switch {
	case errors.As(err, &notFound):
		httpErr(w, 404, err)
		return
	case err != nil:
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	if subagent != "" {
		s.writeSubagent(w, r, conv, subagent)
		return
	}
	if looksAtPane(r, paneID, last) {
		s.Radar.Seen(mc.ID(), paneID)
	}
	if conv.Path == "" {
		writeJSON(w, map[string]any{"sessions": []any{}, "turns": []any{}, "subagents": []any{}})
		return
	}
	etag := conv.ETag()
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	var tail chat.Tail
	if last > 0 {
		tail, err = conv.LastTurns(last)
	} else if tail.Turns, err = conv.Turns(); err == nil {
		tail.Plan = transcript.Plan(tail.Turns)
	}
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	sessions := conv.Sessions
	if sessions == nil || auth.IsViewer(r.Context()) { // a viewer reads the conversation the pane is in
		sessions = []transcript.SessionInfo{}
	}
	answer := map[string]any{"session_id": conv.ID, "sessions": sessions, "turns": tail.Turns}
	if len(tail.Plan) > 0 {
		answer["plan"] = tail.Plan
	}
	if last == 0 { // the lens's notice of the calls Claude Code refused without asking; a tile shows none
		if denied := transcript.DeniedAfter(tail.Turns, "").Calls; len(denied) > 0 {
			answer["denied"] = denied
		}
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	answer["subagents"] = subagentsOf(conv, tail.Turns)
	writeJSON(w, answer)
}

// writeSubagent answers one subagent's conversation: no sessions, nothing marked seen, an ETag of its own file.
func (s *Server) writeSubagent(w http.ResponseWriter, r *http.Request, conv chat.Conversation, id string) {
	turns, version, err := conv.Subagent(id)
	var notFound *chat.NotFoundError
	switch {
	case errors.As(err, &notFound):
		httpErr(w, 404, err)
		return
	case err != nil:
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	etag := fmt.Sprintf(`"%s-%s-%s"`, conv.ID, id, version)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"session_id": conv.ID, "subagent": id, "sessions": []any{}, "turns": turns, "subagents": subagentsOf(conv, turns)})
}

// subagentsOf is the conversation's subagents the turns answered have a use for, never null on the wire.
func subagentsOf(conv chat.Conversation, turns []transcript.Turn) []transcript.Subagent {
	if list := conv.SubagentsOf(turns); list != nil {
		return list
	}
	return []transcript.Subagent{}
}
