package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/limits"
)

// Limits: the person's booking to continue an agent a subscription limit stopped, at the moment the window resets —
// made and called off here, said by the limits service when it comes due. The roster carries every machine's
// windows and each pane's hit and booking.

func (s *Server) limitRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/limits/continue", s.handleLimitContinue)
	api.HandleFunc("DELETE /api/limits/continue", s.handleLimitCancel)
	// the offers to run a held card or schedule on the other vendor: the person's alone — a viewer link reads nothing
	// here and an agent's key stays out of /api/limits
	api.HandleFunc("GET /api/limits/offers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"offers": s.Limits.Offers()})
	})
	api.HandleFunc("POST /api/limits/offers/{id}/run", s.handleOfferRun)
	api.HandleFunc("DELETE /api/limits/offers/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Limits.DeclineOffer(r.PathValue("id")); err != nil {
			httpErr(w, offerStatus(err), err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
}

func (s *Server) handleOfferRun(w http.ResponseWriter, r *http.Request) {
	words, err := s.Limits.RunOffer(r.PathValue("id"))
	if err != nil {
		httpErr(w, offerStatus(err), err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "words": words})
}

// offerStatus is the answer to an offer that could not run: gone, running already, or refused by what it would start.
func offerStatus(err error) int {
	if errors.Is(err, limits.ErrNoOffer) {
		return http.StatusNotFound
	}
	return http.StatusConflict
}

type continueReq struct {
	Machine string `json:"machine"`
	Pane    string `json:"pane"`
	At      string `json:"at"`   // RFC 3339, the moment to say it
	Text    string `json:"text"` // "continue" when empty
}

// continueTarget reads the machine and pane a booking names, refusing an empty one and a machine the fleet does not know.
func (s *Server) continueTarget(w http.ResponseWriter, req continueReq) (string, string, bool) {
	machine := strings.TrimSpace(req.Machine)
	pane := strings.TrimSpace(req.Pane)
	if machine == "" || pane == "" {
		httpErr(w, 400, fmt.Errorf("a machine and a pane are required"))
		return "", "", false
	}
	if s.Machines != nil {
		if _, err := s.Machines.Get(machine); err != nil {
			httpErr(w, 404, fmt.Errorf("unknown machine %q", machine))
			return "", "", false
		}
	}
	return machine, pane, true
}

func (s *Server) handleLimitContinue(w http.ResponseWriter, r *http.Request) {
	var req continueReq
	if !decode(w, r, &req) {
		return
	}
	machine, pane, ok := s.continueTarget(w, req)
	if !ok {
		return
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(req.At))
	if err != nil {
		httpErr(w, 400, fmt.Errorf("at must be an RFC 3339 time: %w", err))
		return
	}
	text := strings.TrimSpace(req.Text)
	if len(text) > 2000 {
		httpErr(w, 400, fmt.Errorf("the words to send are too long"))
		return
	}
	if err := s.Limits.Book(machine, pane, at, text); err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, map[string]any{"continue_at": at.UTC()})
}

func (s *Server) handleLimitCancel(w http.ResponseWriter, r *http.Request) {
	var req continueReq
	if !decode(w, r, &req) {
		return
	}
	machine, pane, ok := s.continueTarget(w, req)
	if !ok {
		return
	}
	if err := s.Limits.Cancel(machine, pane); err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
