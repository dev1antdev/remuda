package server

import (
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/events"
)

// A question answered from a notification: the choice is checked against the dialog on the screen now, then picked.

func (s *Server) answerRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/question/answer", s.handleAnswer)
}

// handleAnswer is POST /api/question/answer {machine, pane, index, label}: walks the cursor to the choice at index and
// presses Enter, a 409 when the screen shows no dialog or that choice no longer reads label.
func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Pane    string `json:"pane"`
		Index   *int   `json:"index"`
		Label   string `json:"label"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Pane == "" || req.Index == nil || *req.Index < 0 || req.Label == "" {
		httpErr(w, 400, fmt.Errorf("pane, index and label required"))
		return
	}
	mc, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	screen, err := client.PaneRead(req.Pane)
	if err != nil {
		rpcErr(w, err)
		return
	}
	keys, err := chat.Answer(screen, *req.Index, req.Label)
	if err != nil {
		httpErr(w, 409, err)
		return
	}
	if err := s.Input.SendKeys(client, events.PaneRef{Machine: mc.ID(), Pane: req.Pane}, keys); err != nil {
		rpcErr(w, err)
		return
	}
	s.noteSent(r.Context(), mc.ID(), req.Pane, req.Label, false)
	writeJSON(w, map[string]any{"ok": true, "chosen": req.Label})
}
