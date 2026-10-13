package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/coder/websocket"
)

// The terminal lens: bytes over a WebSocket, or over plain HTTP for a viewer behind a proxy (termhttp.go),
// both relayed to the machine's own remuda when one holds the attach (peer.go).

func (s *Server) terminalRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /ws/pane", s.handlePaneWS)
	api.HandleFunc("POST /api/term/open", s.handleTermOpen)
	api.HandleFunc("GET /api/term/read", s.handleTermRead)
	api.HandleFunc("POST /api/term/input", s.handleTermInput)
	api.HandleFunc("POST /api/term/resize", s.handleTermResize)
	api.HandleFunc("DELETE /api/term/close", s.handleTermClose)
}

type wsClientMsg struct {
	Type string `json:"type"` // input | resize
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

func (s *Server) handlePaneWS(w http.ResponseWriter, r *http.Request) {
	mc, _, attach, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	paneID := r.URL.Query().Get("id")
	pane, err := s.findPane(mc, paneID)
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	// the client's proxy probe is a plain GET of this URL and must cost no attach, here or on a peer
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}
	s.Radar.Seen(mc.ID(), pane.PaneID)
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))

	if pc, err := s.peerFor(mc); err != nil {
		httpErr(w, 502, err)
		return
	} else if pc != nil {
		s.relayPaneWS(w, r, pc, pane.PaneID, cols, rows)
		return
	}
	ch, cancel, err := attach.Subscribe(pane.TerminalID, cols, rows)
	if err != nil {
		if pc := s.peerAfterRefusal(mc, err); pc != nil {
			s.relayPaneWS(w, r, pc, pane.PaneID, cols, rows)
			return
		}
	}

	c, err2 := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionNoContextTakeover})
	if err2 != nil {
		if err == nil {
			cancel()
		}
		return
	}
	defer c.CloseNow()
	ctx := r.Context()
	c.SetReadLimit(1 << 20)
	if err != nil {
		c.Close(websocket.StatusInternalError, attachRefused(mc, err))
		return
	}
	defer cancel()

	// the writer is stopped by hand when the read loop ends: on a hijacked connection
	// r.Context() lives until the handler returns, so the attach would linger until the pane's next byte
	wctx, wcancel := context.WithCancel(ctx)
	defer wcancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					c.Close(websocket.StatusNormalClosure, "terminal detached")
					return
				}
				for more := true; more; {
					select {
					case extra, ok := <-ch:
						if !ok {
							more = false
						} else {
							// the chunk is shared with every viewer: the full-slice expression makes append copy first
							data = append(data[:len(data):len(data)], extra...)
						}
					default:
						more = false
					}
				}
				if err := c.Write(wctx, websocket.MessageBinary, data); err != nil {
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
		var msg wsClientMsg
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "input":
			if msg.Data != "" {
				if err := attach.Write(pane.TerminalID, []byte(msg.Data)); err != nil {
					log.Printf("input %s/%s: %v", mc.ID(), paneID, err)
				}
			}
		case "resize":
			if err := attach.Resize(pane.TerminalID, msg.Cols, msg.Rows); err != nil {
				log.Printf("resize %s/%s: %v", mc.ID(), paneID, err)
			}
		}
	}
	wcancel()
	<-done
}
