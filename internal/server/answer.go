package server

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/dev1antdev/remuda/internal/chat"
)

// A question answered from a notification: the choice is checked against the dialog on the screen now, then picked.

func (s *Server) answerRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/question/answer", s.handleAnswer)
}

// handleAnswer is POST /api/question/answer {machine, pane, index, label, dialog}: walks the cursor to the choice at
// index and presses Enter, a 409 when the screen shows no dialog, another dialog than the one dialog names (the
// notification's DialogKey), or that choice no longer reads label.
func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Pane    string `json:"pane"`
		Index   *int   `json:"index"`
		Label   string `json:"label"`
		Dialog  string `json:"dialog"`
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
	err := answerOnScreen(s.Input.Lock(mc.ID(), req.Pane),
		func() (string, error) { return client.PaneRead(req.Pane) },
		*req.Index, req.Label, req.Dialog,
		func(keys []string) error { return client.PaneSendKeys(req.Pane, keys) })
	var stale staleDialog
	switch {
	case errors.As(err, &stale):
		httpErr(w, http.StatusConflict, stale.error)
		return
	case err != nil:
		rpcErr(w, err)
		return
	}
	s.noteSent(r.Context(), mc.ID(), req.Pane, req.Label, false)
	writeJSON(w, map[string]any{"ok": true, "chosen": req.Label})
}

// staleDialog says the screen shows no dialog, or not the one the choice was offered on: the caller's 409.
type staleDialog struct{ error }

// answerOnScreen reads the screen, checks the choice and presses the keys under the pane's typing lock, held across
// all three: two answers to one notification — two devices, a tap repeated — would otherwise both read the dialog,
// and the second's keys would land in whatever the agent shows next.
func answerOnScreen(lock sync.Locker, read func() (string, error), index int, label, dialog string, send func([]string) error) error {
	lock.Lock()
	defer lock.Unlock()
	screen, err := read()
	if err != nil {
		return err
	}
	keys, err := chat.Answer(screen, index, label, dialog)
	if err != nil {
		return staleDialog{err}
	}
	return send(keys)
}
