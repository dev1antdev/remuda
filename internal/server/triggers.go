package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/triggers"
)

// Triggers: the person's rules on what a pane prints, replaced whole, and one tried on a pane's last rows
// (internal/triggers). Neither a viewer link nor an agent's key reaches these routes.

// TryRows is how many of a pane's last rows a rule is tried on.
const TryRows = 500

func (s *Server) triggerRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/triggers", s.handleTriggers)
	api.HandleFunc("PUT /api/triggers", s.handleTriggersPut)
	api.HandleFunc("POST /api/triggers/try", s.handleTriggerTry)
}

// triggersAnswer is the rules, why herdr refused them on a machine, and the built-in watches every pane has.
type triggersAnswer struct {
	Rules   []triggers.Rule   `json:"rules"`
	Refused map[string]string `json:"refused"`
	Builtin map[string]string `json:"builtin"`
	Max     int               `json:"max"`
}

func (s *Server) triggersAnswer() triggersAnswer {
	return triggersAnswer{Rules: s.Triggers.Rules.List(), Refused: s.Triggers.Refusals(), Max: triggers.MaxRules,
		Builtin: map[string]string{triggers.WatchServe: triggers.ServePattern, triggers.WatchTests: triggers.TestsPattern}}
}

func (s *Server) handleTriggers(w http.ResponseWriter, r *http.Request) {
	if s.Triggers == nil {
		httpErr(w, http.StatusNotFound, errors.New("triggers are off on this remuda"))
		return
	}
	writeJSON(w, s.triggersAnswer())
}

// handleTriggersPut is PUT /api/triggers [rules]: the list replaced whole, each rule checked, a rule's machine one
// the fleet knows.
func (s *Server) handleTriggersPut(w http.ResponseWriter, r *http.Request) {
	if s.Triggers == nil {
		httpErr(w, http.StatusNotFound, errors.New("triggers are off on this remuda"))
		return
	}
	var list []triggers.Rule
	if !decode(w, r, &list) {
		return
	}
	for _, rule := range list {
		if rule.Machine == "" {
			continue
		}
		if _, err := s.Machines.Get(rule.Machine); err != nil {
			httpErr(w, http.StatusBadRequest, fmt.Errorf("trigger %q: %w", rule.Name, err))
			return
		}
	}
	if _, err := s.Triggers.Replace(list); err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, s.triggersAnswer())
}

type tryReq struct {
	Machine string        `json:"machine"`
	Pane    string        `json:"pane"`
	Rule    triggers.Rule `json:"rule"`
}

// handleTriggerTry is POST /api/triggers/try {machine, pane, rule}: the rows of the pane's last TryRows the rule
// matches, as herdr would read them.
func (s *Server) handleTriggerTry(w http.ResponseWriter, r *http.Request) {
	var req tryReq
	if !decode(w, r, &req) {
		return
	}
	if req.Pane == "" {
		httpErr(w, http.StatusBadRequest, errors.New("pane required"))
		return
	}
	_, client, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	text, _, err := client.PaneRecent(req.Pane, TryRows)
	if err != nil {
		rpcErr(w, err)
		return
	}
	tried, err := triggers.Try(req.Rule, text)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, tried)
}
