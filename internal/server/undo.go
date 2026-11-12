package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/undo"
)

// An agent's turn taken back: the folder's turns, what undoing one would do now, and the undo at the person's word.

func (s *Server) undoRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/undo", s.handleUndoTurns)
	api.HandleFunc("GET /api/undo/{id}/plan", s.handleUndoPlan)
	api.HandleFunc("POST /api/undo/{id}", s.handleUndo)
}

// handleUndoTurns answers GET /api/undo?machine=&dir= with the folder's turns, newest first, their files counted, and whether this remuda
// takes the checkpoints at all.
func (s *Server) handleUndoTurns(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	mc, ok := s.reviewTarget(w, r, "", dir)
	if !ok {
		return
	}
	writeJSON(w, map[string]any{"turns": undo.Listing(s.Undo.Store.List(mc.ID(), dir)), "recording": s.Undo.Tracker.Recording()})
}

// handleUndoPlan answers GET /api/undo/{id}/plan?machine= with what undoing the turn would do as the folder stands.
func (s *Server) handleUndoPlan(w http.ResponseWriter, r *http.Request) {
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	plan, err := s.Undo.Plan(mc.ID(), r.PathValue("id"))
	if err != nil {
		undoErr(w, err)
		return
	}
	writeJSON(w, plan)
}

// handleUndo answers POST /api/undo/{id} {machine, revert}: the turn undone, its commits reverted when revert says so.
func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine string `json:"machine"`
		Revert  bool   `json:"revert"`
	}
	if !decode(w, r, &req) {
		return
	}
	mc, err := s.machineFor(r, req.Machine)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	id := r.PathValue("id")
	result, err := s.Undo.Undo(mc.ID(), id, req.Revert)
	if turn, ok := s.Undo.Store.Get(id); ok && !isRefusal(err) {
		go func() { // the sidebar's counts follow what the folder holds now
			if s.Review != nil {
				s.Review.Badges(mc, []string{turn.Dir})
			}
			s.rosterMoved()
		}()
	}
	if err != nil {
		undoErr(w, err)
		return
	}
	writeJSON(w, result)
}

func isRefusal(err error) bool {
	var refusal *undo.Refusal
	return errors.As(err, &refusal)
}

// undoErr is a turn not kept (404), one that cannot be undone now (409, its reason and the files in the way), or
// what the machine or git answered (502).
func undoErr(w http.ResponseWriter, err error) {
	var refusal *undo.Refusal
	switch {
	case errors.Is(err, undo.ErrNotFound):
		httpErr(w, 404, err)
	case errors.As(err, &refusal):
		httpErr(w, 409, errors.New(strings.TrimSpace(refusal.Error())))
	default:
		httpErr(w, 502, err)
	}
}
