package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/notes"
)

// Notes: one free-text note per checkout, written in the card's Note dialog. The roster carries each workspace's note
// as session.note, so the card, its hover card and the palette read it without a request of their own.

func (s *Server) noteRoutes(api *http.ServeMux) {
	api.HandleFunc("PUT /api/notes", s.handleNotePut)
	api.HandleFunc("DELETE /api/notes", s.handleNoteDelete)
}

type noteReq struct {
	Machine string `json:"machine"`
	Dir     string `json:"dir"`
	Text    string `json:"text"`
}

// noteMachine is the machine a note names, a 404 for one the fleet does not know.
func (s *Server) noteMachine(w http.ResponseWriter, r *http.Request, req noteReq) (string, bool) {
	if s.Machines == nil {
		return req.Machine, true
	}
	mc, err := s.machineFor(r, req.Machine)
	if err != nil {
		httpErr(w, 404, fmt.Errorf("unknown machine %q", req.Machine))
		return "", false
	}
	return mc.ID(), true
}

// handleNotePut is PUT /api/notes {machine, dir, text}: the folder's note kept, or removed when the text is empty.
func (s *Server) handleNotePut(w http.ResponseWriter, r *http.Request) {
	var req noteReq
	if !decode(w, r, &req) {
		return
	}
	machine, ok := s.noteMachine(w, r, req)
	if !ok {
		return
	}
	kept, err := s.Notes.Set(machine, req.Dir, req.Text)
	if err != nil {
		noteErr(w, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, map[string]any{"text": kept})
}

// handleNoteDelete is DELETE /api/notes {machine, dir}: the folder's note removed.
func (s *Server) handleNoteDelete(w http.ResponseWriter, r *http.Request) {
	var req noteReq
	if !decode(w, r, &req) {
		return
	}
	machine, ok := s.noteMachine(w, r, req)
	if !ok {
		return
	}
	if err := s.Notes.Remove(machine, req.Dir); err != nil {
		noteErr(w, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, map[string]any{"ok": true})
}

// noteErr answers what the page got wrong as a 400 and a failed write as a 500.
func noteErr(w http.ResponseWriter, err error) {
	for _, refused := range []error{notes.ErrNoPlace, notes.ErrRelative, notes.ErrTooLong, notes.ErrNotText} {
		if errors.Is(err, refused) {
			httpErr(w, 400, err)
			return
		}
	}
	httpErr(w, 500, err)
}
