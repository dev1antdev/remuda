package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/dev1antdev/remuda/internal/icons"
)

// Folder icons: the emoji or picture the person gave a folder of the sidebar, set and cleared here and carried on
// the roster as each workspace's icon; a picture is served by its hash, cached for good. The favicon route fetches
// a site's icon once at the person's word in the icon dialog and answers it for them to keep or not.

func (s *Server) iconRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/icons/{hash}", s.handleIconImage)
	api.HandleFunc("PUT /api/folder-icons", s.handleSetIcon)
	api.HandleFunc("DELETE /api/folder-icons", s.handleClearIcon)
	api.HandleFunc("POST /api/folder-icons/favicon", s.handleFavicon)
}

var iconHashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

type iconReq struct {
	Machine string `json:"machine"`
	Folder  string `json:"folder"`
	Emoji   string `json:"emoji"`
	Image   string `json:"image"` // a data:image/… URL
}

func (s *Server) handleIconImage(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if !iconHashRe.MatchString(hash) || s.Icons == nil {
		httpErr(w, 404, errors.New("no such icon"))
		return
	}
	contentType, data, ok := s.Icons.Image(hash)
	if !ok {
		httpErr(w, 404, errors.New("no such icon"))
		return
	}
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'") // an SVG opened on its own runs nothing
	_, _ = w.Write(data)
}

// iconTarget reads the machine and folder an icon names: a machine the fleet knows and an absolute folder.
func (s *Server) iconTarget(w http.ResponseWriter, req iconReq) (string, string, bool) {
	machine := strings.TrimSpace(req.Machine)
	folder := strings.TrimSpace(req.Folder)
	if machine == "" || folder == "" {
		httpErr(w, 400, errors.New("a machine and a folder are required"))
		return "", "", false
	}
	if !strings.HasPrefix(folder, "/") {
		httpErr(w, 400, fmt.Errorf("the folder must be an absolute path: %q", folder))
		return "", "", false
	}
	if s.Machines != nil {
		if _, err := s.Machines.Get(machine); err != nil {
			httpErr(w, 404, fmt.Errorf("unknown machine %q", machine))
			return "", "", false
		}
	}
	return machine, folder, true
}

func (s *Server) handleSetIcon(w http.ResponseWriter, r *http.Request) {
	var req iconReq
	if !decode(w, r, &req) {
		return
	}
	machine, folder, ok := s.iconTarget(w, req)
	if !ok {
		return
	}
	if (req.Emoji == "") == (req.Image == "") {
		httpErr(w, 400, errors.New("an icon is an emoji or a picture, one of the two"))
		return
	}
	var icon icons.Icon
	if req.Emoji != "" {
		if err := s.Icons.SetEmoji(machine, folder, req.Emoji); err != nil {
			httpErr(w, 400, err)
			return
		}
		icon.Emoji = req.Emoji
	} else {
		hash, err := s.Icons.SetImage(machine, folder, req.Image)
		if err != nil {
			httpErr(w, 400, err)
			return
		}
		icon.Image = hash
	}
	s.rosterMoved()
	writeJSON(w, map[string]any{"icon": icon})
}

func (s *Server) handleClearIcon(w http.ResponseWriter, r *http.Request) {
	var req iconReq
	if !decode(w, r, &req) {
		return
	}
	machine, folder, ok := s.iconTarget(w, req)
	if !ok {
		return
	}
	if err := s.Icons.Clear(machine, folder); err != nil {
		httpErr(w, 500, err)
		return
	}
	s.rosterMoved()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &req) {
		return
	}
	data, from, err := icons.Favicon(r.Context(), icons.Client(), req.URL)
	if errors.Is(err, icons.ErrNotWeb) {
		httpErr(w, 400, err)
		return
	}
	if err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"data": data, "from": from})
}
