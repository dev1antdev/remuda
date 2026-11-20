package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/herdr"
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

// viewerRecheck is how often a quiet socket asks again whether its viewer link still stands.
const viewerRecheck = 5 * time.Second

// maxFrame bounds the bytes one frame gathers, so a terminal that never stops printing still ends a frame, and with it
// a viewer link's check, at least this often.
const maxFrame = 256 << 10

// viewerStands says the viewer link of a request is not revoked since it came in.
func (s *Server) viewerStands(r *http.Request) bool {
	return s.Guard != nil && s.Guard.StillViewer(r)
}

// gridFrame is a grid as the socket tells it, a text frame between the binary ones: {"type":"grid","cols":…,"rows":…}.
func gridFrame(o herdr.Out) []byte {
	frame, _ := json.Marshal(map[string]any{"type": "grid", "cols": o.Cols, "rows": o.Rows})
	return frame
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
	// a viewer link only watches, which remuda-core alone lets a viewer do (viewerOwnGuard keeps it to its roster's panes)
	watching := auth.IsViewer(r.Context())
	if watching && !mc.OnCore() {
		httpErr(w, http.StatusForbidden, errors.New("this link is for viewing only"))
		return
	}
	// the client's proxy probe is a plain GET of this URL and must cost no attach, here or on a peer
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "upgrade required", http.StatusUpgradeRequired)
		return
	}
	if !watching {
		s.Radar.Seen(mc.ID(), pane.PaneID)
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))

	// never a viewer's: remuda-core has no peer, and a refusal of its is none of herdr's
	if pc, err := s.peerFor(mc); err != nil {
		httpErr(w, 502, err)
		return
	} else if pc != nil {
		s.relayPaneWS(w, r, pc, mc.ID(), pane.PaneID, cols, rows)
		return
	}
	view := herdr.View{Cols: cols, Rows: rows, Keep: r.URL.Query().Get("keep") == "1", Observe: watching}
	viewer, err := attach.Open(pane.TerminalID, view)
	if err != nil {
		if pc := s.peerAfterRefusal(mc, err); pc != nil {
			s.relayPaneWS(w, r, pc, mc.ID(), pane.PaneID, cols, rows)
			return
		}
	}

	c, err2 := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionNoContextTakeover})
	if err2 != nil {
		if err == nil {
			viewer.Close()
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
	defer viewer.Close()

	// the writer is stopped by hand when the read loop ends: on a hijacked connection
	// r.Context() lives until the handler returns, so the attach would linger until the pane's next byte
	wctx, wcancel := context.WithCancel(ctx)
	defer wcancel()
	// a viewer link revoked while it watches sees nothing more: its key or device is asked again before every write,
	// and every few seconds while the terminal is quiet
	var recheck <-chan time.Time
	if watching {
		ticker := time.NewTicker(viewerRecheck)
		defer ticker.Stop()
		recheck = ticker.C
	}
	revoked := func() bool {
		if watching && !s.viewerStands(r) {
			c.Close(websocket.StatusPolicyViolation, "this link was revoked")
			return true
		}
		return false
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		out := viewer.Out()
		var held *herdr.Out // a grid that ended a run of bytes, sent next
		for {
			next, ok := herdr.Out{}, true
			if held != nil {
				next, held = *held, nil
			} else {
				select {
				case <-recheck:
					if revoked() {
						return
					}
					continue
				case next, ok = <-out:
				case <-wctx.Done():
					return
				}
			}
			if !ok {
				c.Close(websocket.StatusNormalClosure, "terminal detached")
				return
			}
			if next.Grid() {
				if revoked() {
					return
				}
				if err := c.Write(wctx, websocket.MessageText, gridFrame(next)); err != nil {
					return
				}
				continue
			}
			data := next.Data
			for more := len(data) < maxFrame; more; more = more && len(data) < maxFrame {
				select {
				case extra, ok := <-out:
					switch {
					case !ok:
						more = false
					case extra.Grid():
						held, more = &extra, false // the bytes before it go first
					default:
						// the chunk is shared with every viewer: the full-slice expression makes append copy first
						data = append(data[:len(data):len(data)], extra.Data...)
					}
				default:
					more = false
				}
			}
			if revoked() { // asked once the batch is whole: nothing that came after a revocation goes out
				return
			}
			if err := c.Write(wctx, websocket.MessageBinary, data); err != nil {
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
		if watching {
			continue
		}
		switch msg.Type {
		case "input":
			if msg.Data != "" {
				if err := viewer.Write([]byte(msg.Data)); err != nil {
					log.Printf("input %s/%s: %v", mc.ID(), paneID, err)
				}
			}
		case "resize":
			if err := viewer.Resize(msg.Cols, msg.Rows); err != nil {
				log.Printf("resize %s/%s: %v", mc.ID(), paneID, err)
			}
		}
	}
	wcancel()
	<-done
}
