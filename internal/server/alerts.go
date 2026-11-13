package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/dev1antdev/remuda/internal/push"
)

// A socket device's alerts: the Mac app registers as a device that hears its alerts over a WebSocket instead of
// Web Push, so the push service's rules, prefs, quiet hours and action buttons apply to it as to a phone. A
// connection asking with withdraw=1 also hears which of its alerts no longer stand, the ones owed since it was away
// first, so it takes them down.

func (s *Server) alertRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /api/push/socket", s.handleSocketDevice)
	api.HandleFunc("GET /ws/alerts", s.handleAlertsWS)
}

// handleSocketDevice mints a socket device, or updates the prefs and label of the one named.
func (s *Server) handleSocketDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string      `json:"endpoint"`
		Prefs    *push.Prefs `json:"prefs"`
		Label    string      `json:"label"`
	}
	if !decode(w, r, &req) {
		return
	}
	endpoint, err := s.Push.SubscribeSocket(req.Endpoint, req.Prefs, req.Label)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	writeJSON(w, map[string]any{"endpoint": endpoint})
}

// handleAlertsWS streams a socket device's alerts — and, when it takes them, its withdrawals — as JSON text frames for
// as long as the connection stands.
func (s *Server) handleAlertsWS(w http.ResponseWriter, r *http.Request) {
	endpoint := r.URL.Query().Get("endpoint")
	if !s.Push.HasSocket(endpoint) {
		httpErr(w, 404, fmt.Errorf("unknown socket device"))
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionNoContextTakeover})
	if err != nil {
		return
	}
	defer c.CloseNow()
	withdraws := r.URL.Query().Get("withdraw") == "1"
	alerts, detach := s.Push.Sockets.Attach(endpoint, withdraws)
	defer detach()
	ctx := r.Context()
	c.SetReadLimit(16 << 10)
	if owed := s.Push.OwedTo(endpoint); withdraws && len(owed) > 0 { // taken back while the device was away
		body, _ := json.Marshal(push.Frame{Withdraw: owed})
		if c.Write(ctx, websocket.MessageText, body) == nil {
			s.Push.Settle(endpoint, owed)
		}
	}

	// the writer is stopped by hand when the read loop ends: on a hijacked connection r.Context() lives until the handler returns
	wctx, wcancel := context.WithCancel(ctx)
	defer wcancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ping := time.NewTicker(livePing)
		defer ping.Stop()
		for {
			select {
			case frame, ok := <-alerts:
				if !ok {
					if s.Push.Sockets.Closed() {
						c.Close(websocket.StatusGoingAway, "remuda is stopping")
					} else {
						c.Close(websocket.StatusPolicyViolation, "another connection took this device")
					}
					return
				}
				body, _ := json.Marshal(frame)
				if err := c.Write(wctx, websocket.MessageText, body); err != nil {
					return
				}
			case <-ping.C:
				pctx, cancel := context.WithTimeout(wctx, livePongWait)
				err := c.Ping(pctx)
				cancel()
				if err != nil {
					c.CloseNow() // a ping unanswered leaves the read waiting on a connection nobody answers: it ends here
					return
				}
			case <-wctx.Done():
				return
			}
		}
	}()

	for {
		if _, _, err := c.Read(ctx); err != nil { // the device sends nothing; a read ends with the connection
			break
		}
	}
	wcancel()
	<-done
}
