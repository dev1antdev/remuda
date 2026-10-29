package server

import (
	"net/http"

	"github.com/dev1antdev/remuda/internal/push"
)

// Webhooks: the places alerts go beside the devices — the list with its secrets masked, the list replaced whole, a test.

func (s *Server) webhookRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/webhooks", s.handleWebhooks)
	api.HandleFunc("PUT /api/webhooks", s.handleWebhooksPut)
	api.HandleFunc("POST /api/webhooks/{id}/test", s.handleWebhookTest)
}

func (s *Server) writeWebhooks(w http.ResponseWriter) {
	base, hooks := s.Push.Webhooks.Views()
	writeJSON(w, map[string]any{"base": base, "hooks": hooks, "kinds": push.HookKinds})
}

func (s *Server) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	s.writeWebhooks(w)
}

func (s *Server) handleWebhooksPut(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Base  string         `json:"base"`
		Hooks []push.Webhook `json:"hooks"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.Push.Webhooks.Replace(req.Base, req.Hooks); err != nil {
		httpErr(w, 400, err)
		return
	}
	s.writeWebhooks(w)
}

func (s *Server) handleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if err := s.Push.Webhooks.Test(r.PathValue("id")); err != nil {
		httpErr(w, 502, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
