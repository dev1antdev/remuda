package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/dev1antdev/remuda/internal/push"
)

// Web push: the public key, a device's subscription and its preferences, a test.

func (s *Server) pushRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/push", s.handlePushInfo)
	api.HandleFunc("POST /api/push/subscribe", s.handlePushSubscribe)
	api.HandleFunc("DELETE /api/push/subscribe", s.handlePushUnsubscribe)
	api.HandleFunc("POST /api/push/test", s.handlePushTest)
	s.webhookRoutes(api)
	s.alertRoutes(api)
	s.muteRoutes(api)
}

func (s *Server) handlePushInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"public_key": s.Push.PublicKey()})
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subscription push.Subscription `json:"subscription"`
		Prefs        *push.Prefs       `json:"prefs"`
		Label        string            `json:"label"`
		OldEndpoint  string            `json:"old_endpoint"`
	}
	if !decode(w, r, &req) {
		return
	}
	sub := req.Subscription
	if !strings.HasPrefix(sub.Endpoint, "https://") || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		httpErr(w, 400, fmt.Errorf("a push subscription with an https endpoint and keys is required"))
		return
	}
	if req.Prefs != nil {
		sub.Prefs = *req.Prefs
	} else {
		sub.Prefs = push.Prefs{Input: true}
	}
	sub.Label = req.Label
	s.Push.Subscribe(sub, req.OldEndpoint)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Endpoint == "" {
		httpErr(w, 400, fmt.Errorf("endpoint required"))
		return
	}
	s.Push.Unsubscribe(req.Endpoint)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.Push.SendTest(req.Endpoint); err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
