package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dev1antdev/remuda/internal/backup"
)

// Backups: where and when the cockpit's state dir is copied to, the last runs, a run now, and the copies on the target.
// The person's only: neither an agent key's routes nor a viewer link's.

func (s *Server) backupRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/backups", s.handleBackups)
	api.HandleFunc("PUT /api/backups", s.handleBackupsPut)
	api.HandleFunc("POST /api/backups/run", s.handleBackupRun)
	api.HandleFunc("GET /api/backups/files", s.handleBackupFiles)
}

// handleBackups is GET /api/backups: the settings, the last run and the last good one, the next nightly run.
func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Backup.Status())
}

// handleBackupsPut is PUT /api/backups {machine, at, keep}: a machine of the fleet, or "" for no backups. A remuda
// that serves the page only has no machine of its own there: a copy on its own disk would die with it.
func (s *Server) handleBackupsPut(w http.ResponseWriter, r *http.Request) {
	var settings backup.Settings
	if !decode(w, r, &settings) {
		return
	}
	if settings.Machine != "" && !s.inFleet(settings.Machine) {
		httpErr(w, 400, fmt.Errorf("%q is no machine of the fleet to keep the copies on; pick one of its machines", settings.Machine))
		return
	}
	if err := s.Backup.Put(settings); err != nil {
		httpErr(w, 400, err)
		return
	}
	writeJSON(w, s.Backup.Status())
}

// inFleet says whether a machine is one the cockpit works with: the host only while it is no server
func (s *Server) inFleet(id string) bool {
	for _, mc := range s.Machines.List() {
		if mc.ID() == id {
			return true
		}
	}
	return false
}

// handleBackupRun is POST /api/backups/run: a backup now, answered when it is on the target. 409 while one runs, 400
// with no machine picked, 502 with what went wrong.
func (s *Server) handleBackupRun(w http.ResponseWriter, r *http.Request) {
	result, err := s.Backup.Run(true)
	switch {
	case errors.Is(err, backup.ErrBusy):
		httpErr(w, http.StatusConflict, err)
	case errors.Is(err, backup.ErrNoTarget):
		httpErr(w, 400, err)
	case err != nil:
		httpErr(w, http.StatusBadGateway, err)
	default:
		writeJSON(w, result)
	}
}

// handleBackupFiles is GET /api/backups/files: the copies in the target's folder, newest first.
func (s *Server) handleBackupFiles(w http.ResponseWriter, r *http.Request) {
	folder, files, err := s.Backup.List()
	if errors.Is(err, backup.ErrNoTarget) {
		httpErr(w, 400, err)
		return
	}
	if err != nil {
		httpErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"folder": folder, "files": files})
}
