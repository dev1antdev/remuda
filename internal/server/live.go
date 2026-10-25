package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// The page's event stream: one WebSocket per page carrying the hub's events out and the page's watched panes in.

func (s *Server) liveRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /ws/events", s.handleEventsWS)
}

// livePing is how often a quiet stream is pinged, so a dead page is let go of.
const livePing = 30 * time.Second

type liveClientMsg struct {
	Sub []string `json:"sub"` // the panes the page shows, "machine|pane"
}

func (s *Server) handleEventsWS(w http.ResponseWriter, r *http.Request) {
	if s.Live == nil {
		http.Error(w, "no event stream", http.StatusNotFound)
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
	sub := s.Live.Subscribe()
	defer sub.Close()
	ctx := r.Context()
	c.SetReadLimit(64 << 10)

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
			case ev, ok := <-sub.Events:
				if !ok {
					c.Close(websocket.StatusGoingAway, "remuda is stopping")
					return
				}
				body, _ := json.Marshal(ev)
				if err := c.Write(wctx, websocket.MessageText, body); err != nil {
					return
				}
			case <-ping.C:
				pctx, cancel := context.WithTimeout(wctx, 10*time.Second)
				err := c.Ping(pctx)
				cancel()
				if err != nil {
					return
				}
			case <-wctx.Done():
				return
			}
		}
	}()

	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		if typ != websocket.MessageText {
			continue
		}
		var msg liveClientMsg
		if json.Unmarshal(data, &msg) != nil || msg.Sub == nil {
			continue
		}
		sub.Watch(msg.Sub)
	}
	wcancel()
	<-done
}
