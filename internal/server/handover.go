package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/feed"
	"github.com/dev1antdev/remuda/internal/handover"
	"github.com/dev1antdev/remuda/internal/recap"
	"github.com/dev1antdev/remuda/internal/sessions"
)

// A conversation handed to another agent: its gist as the first prompt of a fresh agent in a new tab of the same workspace.

func (s *Server) handoverRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/handover", s.handleHandover)
}

type handoverReq struct {
	Machine string `json:"machine"`
	Pane    string `json:"pane"`
	Session string `json:"session"` // the conversation shown, "" for the pane's live one
	Agent   string `json:"agent"`
	Model   string `json:"model"`
	Args    string `json:"args"`
	Turns   *int   `json:"turns"` // the last exchanges carried in full; 3 when unsaid
}

// handleHandover is POST /api/handover: the pane's conversation summed up into a first prompt, a new tab in its
// workspace with the agent asked, the prompt handed over once the agent is up; answers {id, pane, size, from}.
func (s *Server) handleHandover(w http.ResponseWriter, r *http.Request) {
	var req handoverReq
	if !decode(w, r, &req) {
		return
	}
	if req.Pane == "" || req.Agent == "" || req.Agent == "shell" {
		httpErr(w, 400, errors.New("a pane and an agent to take over are required"))
		return
	}
	if !sessions.KnownAgent(req.Agent) {
		httpErr(w, 400, fmt.Errorf("unknown agent %q", req.Agent))
		return
	}
	if req.Session != "" && !sessionIDRe.MatchString(req.Session) {
		httpErr(w, 400, errors.New("bad session id"))
		return
	}
	lastN := 3
	if req.Turns != nil {
		lastN = *req.Turns
	}
	create := createReq{Agent: req.Agent, Model: req.Model, Args: req.Args}
	start, err := create.start()
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	snap, err := s.Watch.Snapshot(mc.ID())
	if snap == nil {
		httpErr(w, 502, err)
		return
	}
	pane := snap.Pane(req.Pane)
	if pane == nil {
		httpErr(w, 404, fmt.Errorf("pane %s not found", req.Pane))
		return
	}
	if pane.Agent == "" {
		httpErr(w, 400, errors.New("a shell pane has no conversation to hand over"))
		return
	}
	conv, err := s.Chat.Open(mc, req.Pane, req.Session, "", false)
	var notFound *chat.NotFoundError
	switch {
	case errors.As(err, &notFound):
		httpErr(w, 404, err)
		return
	case err != nil:
		httpErr(w, http.StatusBadGateway, err)
		return
	case conv.Path == "":
		httpErr(w, 400, errors.New("nothing to hand over yet: the agent has not said anything"))
		return
	}
	turns, err := conv.Turns()
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	from := pane.AgentName()
	text := handover.Text(from, recap.Of(turns, time.Time{}), turns, lastN)
	to := agentLabel(req.Agent)
	label := to + " handover"
	res, err := client.TabCreate(pane.WorkspaceID, label)
	if err != nil {
		rpcErr(w, err)
		return
	}
	newPane := res.RootPane.PaneID
	resp := map[string]any{"id": res.Tab.TabID, "pane": newPane, "size": len(text), "from": from}
	if err := sessions.StartAgentWith(client, newPane, req.Agent, start); err != nil {
		resp["agent_error"] = err.Error() + " (the handover was not sent)"
	} else {
		go s.firstPrompt(client, events.PaneRef{Machine: mc.ID(), Pane: newPane}, text, label)
	}
	if s.Feed != nil {
		workspace := snap.WorkspaceLabel(pane.WorkspaceID)
		s.Feed.Add(feed.Entry{Kind: "handover", Machine: mc.ID(), Pane: newPane, Title: "Handed over to " + to + ": " + workspace, Body: handoverWords(from, lastN, len(text))})
	}
	s.Watch.Refresh(mc)
	writeJSON(w, resp)
}

// handoverWords says what went over, for the activity: "from Claude Code · the recap and the last 3 exchanges · 14 KB".
func handoverWords(from string, lastN, size int) string {
	what := "the recap"
	switch lastN {
	case 0:
	case 1:
		what += " and the last exchange"
	default:
		what += fmt.Sprintf(" and the last %d exchanges", lastN)
	}
	return fmt.Sprintf("from %s · %s · %d KB", from, what, (size+512)/1024)
}

// agentLabel is the agent's name as the page shows it, the kind itself when the table lacks it.
func agentLabel(kind string) string {
	for _, agent := range sessions.Agents {
		if agent.ID == kind {
			return agent.Label
		}
	}
	return kind
}
