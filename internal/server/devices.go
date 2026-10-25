package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
)

func (s *Server) deviceRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/devices", s.handleDevices)
	api.HandleFunc("PATCH /api/devices/{id}", s.handleDeviceRename)
	api.HandleFunc("DELETE /api/devices/{id}", s.handleDeviceRevoke)
	api.HandleFunc("DELETE /api/devices", s.handleDevicesRevokeOthers)
}

// deviceView is a signed-in device as the page sees it: never the hash, the browser and system in words, whether it is the caller's own.
type deviceView struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"last_seen"`
	IP       string    `json:"ip,omitempty"`
	This     bool      `json:"this"`
}

func viewOf(device auth.Device, caller string) deviceView {
	return deviceView{ID: device.ID, Name: device.Name, Kind: device.Kind(), Created: device.Created, LastSeen: device.LastSeen, IP: device.IP, This: device.ID == caller && caller != ""}
}

// devices is the store behind the guard, or a 503 when this remuda keeps none.
func (s *Server) devices(w http.ResponseWriter) (*auth.Devices, bool) {
	if s.Guard == nil || s.Guard.Devices == nil {
		httpErr(w, http.StatusServiceUnavailable, errors.New("this remuda keeps no devices"))
		return nil, false
	}
	return s.Guard.Devices, true
}

// handleDevices is GET /api/devices: every signed-in device, the most recently seen first, the caller's own marked.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	store, ok := s.devices(w)
	if !ok {
		return
	}
	caller := auth.DeviceID(r.Context())
	list := store.List()
	out := make([]deviceView, 0, len(list))
	for _, device := range list {
		out = append(out, viewOf(device, caller))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, out)
}

// handleDeviceRename is PATCH /api/devices/{id} {name}: a name of the person's own, empty for the one the browser gives.
func (s *Server) handleDeviceRename(w http.ResponseWriter, r *http.Request) {
	store, ok := s.devices(w)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	device, err := store.Rename(r.PathValue("id"), body.Name)
	if errors.Is(err, auth.ErrNoDevice) {
		httpErr(w, 404, err)
		return
	}
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	writeJSON(w, viewOf(device, auth.DeviceID(r.Context())))
}

// handleDeviceRevoke is DELETE /api/devices/{id}: the device is signed out; the caller's own too, and its cookie goes with the answer.
func (s *Server) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	store, ok := s.devices(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !store.Revoke(id) {
		httpErr(w, 404, auth.ErrNoDevice)
		return
	}
	self := id == auth.DeviceID(r.Context())
	if self {
		s.Guard.SignOut(w, r)
	}
	writeJSON(w, map[string]any{"revoked": id, "self": self})
}

// handleDevicesRevokeOthers is DELETE /api/devices?others=1: every device but the caller's is signed out.
func (s *Server) handleDevicesRevokeOthers(w http.ResponseWriter, r *http.Request) {
	store, ok := s.devices(w)
	if !ok {
		return
	}
	if r.URL.Query().Get("others") == "" {
		httpErr(w, 400, fmt.Errorf("say which: ?others=1 signs out every device but this one"))
		return
	}
	gone := store.RevokeOthers(auth.DeviceID(r.Context()))
	writeJSON(w, map[string]any{"revoked": gone})
}
