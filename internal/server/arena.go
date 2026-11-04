package server

import (
	"errors"
	"net/http"

	"github.com/dev1antdev/remuda/internal/arena"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// Arenas: one prompt to several agents side by side, each in a worktree of its own (internal/arena).

// arenaStatsFor is how many of the newest arenas get their lanes' numbers read on a listing.
const arenaStatsFor = 5

type arenaView struct {
	arena.Arena
	Stats map[int]arena.Stat `json:"stats,omitempty"`
}

func (s *Server) arenaRoutes(api *http.ServeMux) {
	// every arena, the newest first, the newest few with each lane's +ins −del · files read on its machine
	api.HandleFunc("GET /api/arenas", func(w http.ResponseWriter, r *http.Request) {
		list := s.Arena.Store.List()
		out := make([]arenaView, len(list))
		for i, a := range list {
			out[i] = arenaView{Arena: a}
			if i < arenaStatsFor {
				out[i].Stats = s.Arena.Stats(a)
			}
		}
		writeJSON(w, out)
	})
	// a new arena {prompt | task_id, groom, lanes: [{machine, workspace, agent, model}]}: answered at once, its lanes starting
	api.HandleFunc("POST /api/arenas", func(w http.ResponseWriter, r *http.Request) {
		var req arena.Request
		if !decode(w, r, &req) {
			return
		}
		a, err := s.Arena.Start(req)
		s.arenaReply(w, a, err)
	})
	// keep one lane {lane}: every other lane's work saved as a patch, its workspace closed, its worktree and branch removed
	api.HandleFunc("POST /api/arenas/{id}/keep", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var req struct {
			Lane int `json:"lane"`
		}
		if !decode(w, r, &req) {
			return
		}
		a, err := s.Arena.Keep(int(id), req.Lane)
		s.rosterMoved()
		s.arenaReply(w, a, err)
	})
	// forget an arena: its lanes stay as they are
	api.HandleFunc("DELETE /api/arenas/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		if err := s.Arena.Store.Forget(int(id)); err != nil {
			httpErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}

func (s *Server) arenaReply(w http.ResponseWriter, a arena.Arena, err error) {
	switch {
	case err == nil:
		writeJSON(w, a)
	case errors.Is(err, arena.ErrInvalid):
		httpErr(w, http.StatusBadRequest, err)
	case errors.Is(err, arena.ErrNotFound), errors.Is(err, tasks.ErrNotFound):
		httpErr(w, http.StatusNotFound, err)
	default:
		httpErr(w, http.StatusInternalServerError, err)
	}
}
