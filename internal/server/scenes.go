package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dev1antdev/remuda/internal/scenes"
)

// Scenes: the morning set-ups and the evening tidies, the list replaced whole; a run started at the person's press and
// read back while it goes; a tidy's plan shown before it runs; a workspace's tab captured as a step's layout.

func (s *Server) sceneRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/scenes", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Scenes.Store.List()) })
	api.HandleFunc("PUT /api/scenes", s.handleScenesPut)
	api.HandleFunc("POST /api/scenes/capture", s.handleSceneCapture)
	api.HandleFunc("GET /api/scenes/{id}/plan", s.handleScenePlan)
	api.HandleFunc("POST /api/scenes/{id}/run", s.handleSceneRun)
	api.HandleFunc("GET /api/scene-runs/{id}", s.handleSceneRunGet)
}

func (s *Server) handleScenesPut(w http.ResponseWriter, r *http.Request) {
	var list []scenes.Scene
	if !decode(w, r, &list) {
		return
	}
	if err := s.fleetKnows(list); err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.Scenes.Store.Replace(list)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, out)
}

// fleetKnows refuses a scene naming a machine the fleet does not list — under -server, the host serving the page among them.
func (s *Server) fleetKnows(list []scenes.Scene) error {
	known := map[string]bool{}
	for _, mc := range s.Machines.List() {
		known[mc.ID()] = true
	}
	for _, scene := range list {
		names := []string{}
		for _, step := range scene.Steps {
			names = append(names, step.Machine)
		}
		if scene.Tidy != nil {
			names = append(names, scene.Tidy.Machines...)
		}
		for _, id := range names {
			if id != "" && !known[id] {
				return fmt.Errorf("scene %q names %q, not a machine of this cockpit", scene.Name, id)
			}
		}
	}
	return nil
}

// handleSceneCapture is POST /api/scenes/capture {machine, workspace}: the workspace's active tab as a step's layout,
// with the workspace's folder and the agent running in that tab.
func (s *Server) handleSceneCapture(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Machine   string `json:"machine"`
		Workspace string `json:"workspace"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Workspace == "" {
		httpErr(w, http.StatusBadRequest, errors.New("a workspace is required"))
		return
	}
	mc, _, _, ok := s.ready(w, r, req.Machine)
	if !ok {
		return
	}
	layout, dir, agent, err := s.Scenes.Capture(mc.ID(), req.Workspace)
	if err != nil {
		rpcErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"layout": layout, "dir": dir, "agent": agent})
}

// handleScenePlan is GET /api/scenes/{id}/plan: what a tidy scene would do now, before the person picks what runs.
func (s *Server) handleScenePlan(w http.ResponseWriter, r *http.Request) {
	scene, ok := s.Scenes.Store.Get(r.PathValue("id"))
	if !ok {
		httpErr(w, http.StatusNotFound, errors.New("no such scene"))
		return
	}
	if scene.Kind != scenes.KindTidy {
		httpErr(w, http.StatusBadRequest, errors.New("a scene that starts things has its steps for a plan"))
		return
	}
	writeJSON(w, s.Scenes.Plan(*scene.Tidy, time.Now()))
}

// handleSceneRun is POST /api/scenes/{id}/run {steps?, actions?}: the run begun in the background, answered at once.
func (s *Server) handleSceneRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Steps   []int    `json:"steps"`
		Actions []string `json:"actions"`
	}
	if !decode(w, r, &req) {
		return
	}
	run, err := s.Scenes.Start(r.PathValue("id"), req.Steps, req.Actions, s.senderName(r.Context()))
	switch {
	case errors.Is(err, scenes.ErrNoScene):
		httpErr(w, http.StatusNotFound, err)
	case errors.Is(err, scenes.ErrBusy):
		httpErr(w, http.StatusConflict, err)
	case err != nil:
		httpErr(w, http.StatusBadRequest, err)
	default:
		writeJSON(w, run)
	}
}

func (s *Server) handleSceneRunGet(w http.ResponseWriter, r *http.Request) {
	run, ok := s.Scenes.Get(r.PathValue("id"))
	if !ok {
		httpErr(w, http.StatusNotFound, errors.New("no such run: it ended long ago, or with the remuda that ran it"))
		return
	}
	writeJSON(w, run)
}
