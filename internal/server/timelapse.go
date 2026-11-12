package server

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/timelapse"
)

// Timelapses: what the agents' screens did while they worked, recorded on the cockpit. A viewer link may list and play
// them, as it may read a screen (internal/auth viewerRoutes); the switch, the counts and every deletion are the
// person's alone, and an agent's key reaches none of it. The search over them is a viewer's too, as the recordings are.

func (s *Server) timelapseRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/timelapse", s.handleTimelapse)
	api.HandleFunc("PUT /api/timelapse", s.handleTimelapsePut)
	api.HandleFunc("DELETE /api/timelapse", s.handleTimelapseClear)
	api.HandleFunc("GET /api/timelapse/recordings", s.handleRecordings)
	api.HandleFunc("GET /api/timelapse/recordings/{id}", s.handleRecording)
	api.HandleFunc("DELETE /api/timelapse/recordings/{id}", s.handleRecordingDelete)
	api.HandleFunc("GET /api/timelapse/search", s.handleScreenSearch)
}

// handleScreenSearch is GET /api/timelapse/search?q&machine&pane&dir&days: where the text stood on the recorded screens
// — every machine's, or the machine's, its pane's and folder's as the recordings route lists them — the newest first,
// within the search's own bounds (timelapse.Search).
func (s *Server) handleScreenSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	text := []rune(strings.TrimSpace(q.Get("q")))
	if len(text) < timelapse.MinQuery {
		httpErr(w, 400, errors.New("two characters or more"))
		return
	}
	query := timelapse.SearchQuery{Text: string(text[:min(len(text), timelapse.MaxQuery)]), Pane: q.Get("pane"), Dir: q.Get("dir")}
	if id := q.Get("machine"); id != "" && id != "all" {
		mc, err := s.machineFor(r, "")
		if err != nil {
			httpErr(w, 404, err)
			return
		}
		query.Machine = mc.ID()
	} else if query.Pane != "" {
		httpErr(w, 400, errors.New("a pane needs its machine"))
		return
	}
	if days, _ := strconv.Atoi(q.Get("days")); days > 0 {
		query.Since = time.Now().AddDate(0, 0, -days)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, s.Timelapse.Search(r.Context(), query))
}

// handleTimelapse is GET /api/timelapse: whether turns are recorded, and how many recordings take how much.
func (s *Server) handleTimelapse(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Timelapse.Stats())
}

// handleTimelapsePut is PUT /api/timelapse {on}: recording on or off; off drops the turns being recorded.
func (s *Server) handleTimelapsePut(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On *bool `json:"on"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.On == nil {
		httpErr(w, 400, errors.New("on required"))
		return
	}
	if err := s.Timelapse.SetOn(*body.On); err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, s.Timelapse.Stats())
}

// handleTimelapseClear is DELETE /api/timelapse: every recording gone, answering how many.
func (s *Server) handleTimelapseClear(w http.ResponseWriter, r *http.Request) {
	n, err := s.Timelapse.DeleteAll()
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, map[string]any{"deleted": n, "stats": s.Timelapse.Stats()})
}

// handleRecordings is GET /api/timelapse/recordings?machine&pane&dir: the machine's recordings of the pane and of the
// folder — every earlier pane that worked there — the newest first, and whether recording is on.
func (s *Server) handleRecordings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("pane") == "" && q.Get("dir") == "" {
		httpErr(w, 400, errors.New("pane or dir required"))
		return
	}
	mc, err := s.machineFor(r, "")
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, map[string]any{"recordings": s.Timelapse.List(mc.ID(), q.Get("pane"), q.Get("dir")), "on": s.Timelapse.On()})
}

// handleRecording is GET /api/timelapse/recordings/{id}: the recording whole, as the file holds it — gzipped, which the
// browser unpacks itself, so a Raspberry Pi spends nothing on it; unpacked here only for a client that takes no gzip.
func (s *Server) handleRecording(w http.ResponseWriter, r *http.Request) {
	_, path, err := s.Timelapse.File(r.PathValue("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		httpErr(w, 404, fmt.Errorf("%w: %v", timelapse.ErrNotFound, err))
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h.Set("Content-Encoding", "gzip")
		io.Copy(w, f)
		return
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	io.Copy(w, zr)
}

// handleRecordingDelete is DELETE /api/timelapse/recordings/{id}.
func (s *Server) handleRecordingDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Timelapse.Delete(r.PathValue("id")); err != nil {
		httpErr(w, 404, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
