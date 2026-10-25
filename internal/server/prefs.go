package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/dev1antdev/remuda/internal/prefs"
)

// Shared settings: the keys a device asked to sync, each with the moment it was set; GET answers 304 to the ETag it gave, PUT merges a part.

func (s *Server) prefsRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/prefs", s.handlePrefsGet)
	api.HandleFunc("PUT /api/prefs", s.handlePrefsPut)
}

func (s *Server) handlePrefsGet(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(s.Prefs.All())
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:12]) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (s *Server) handlePrefsPut(w http.ResponseWriter, r *http.Request) {
	var in map[string]prefs.Entry
	if !decode(w, r, &in) {
		return
	}
	out, err := s.Prefs.Put(in)
	if err != nil {
		httpErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, out)
}
