package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
)

// A terminal through the machine's own remuda: herdr lets one client attach to a
// terminal at a time, so a remote that runs a remuda has every viewer relayed to it.

const peerSep = "~" // a relayed HTTP stream's id: "<machine>~<the peer's sid>"

// peerClient is how to reach the peer: its forwarded address and token.
type peerClient struct {
	base  string
	token string
}

func (s *Server) peerFor(mc *machines.Machine) (*peerClient, error) {
	p := mc.Peer()
	if p == nil {
		return nil, nil
	}
	addr, err := mc.ForwardAddr(p.Port)
	if err != nil {
		return nil, fmt.Errorf("reach remuda on %s: %w", mc.Label(), err)
	}
	return &peerClient{base: "http://" + addr, token: p.Token}, nil
}

// peerAfterRefusal probes the machine again after a refused attach: a remuda may have started there since the last look.
func (s *Server) peerAfterRefusal(mc *machines.Machine, err error) *peerClient {
	if mc.Local || !herdr.Refused(err) || s.Health == nil {
		return nil
	}
	if r := s.Health.Report(mc.Cfg.ID); r == nil || time.Since(r.At) > 30*time.Second {
		s.Health.Refresh(mc)
	}
	pc, perr := s.peerFor(mc)
	if perr != nil {
		log.Printf("peer %s: %v", mc.Cfg.ID, perr)
		return nil
	}
	return pc
}

// attachRefused is the message for a viewer when nothing can be done.
// It travels as a WebSocket close reason, which holds 123 bytes at most.
func attachRefused(mc *machines.Machine, err error) string {
	msg := "attach failed: " + err.Error()
	if herdr.Refused(err) {
		msg = "another client holds this terminal on " + mc.Label() + " — start remuda there to share it"
	}
	if len(msg) > 120 {
		msg = msg[:117] + "…"
	}
	return msg
}

// relayPaneWS pipes the viewer's socket to the peer's /ws/pane for the same pane, both ways.
func (s *Server) relayPaneWS(w http.ResponseWriter, r *http.Request, pc *peerClient, paneID string, cols, rows int) {
	ctx := r.Context()
	wsURL := strings.Replace(pc.base, "http://", "ws://", 1) + "/ws/pane?" + url.Values{"machine": {"local"}, "id": {paneID}, "cols": {fmt.Sprint(cols)}, "rows": {fmt.Sprint(rows)}}.Encode()
	dctx, dcancel := context.WithTimeout(ctx, 15*time.Second)
	up, res, err := websocket.Dial(dctx, wsURL, &websocket.DialOptions{
		HTTPHeader:      http.Header{"Authorization": {"Bearer " + pc.token}},
		CompressionMode: websocket.CompressionNoContextTakeover,
	})
	dcancel()
	if err != nil {
		msg := err.Error()
		if res != nil {
			msg = fmt.Sprintf("%s (HTTP %d)", msg, res.StatusCode)
		}
		log.Printf("peer relay %s/%s: %s", pc.base, paneID, msg)
		httpErr(w, 502, fmt.Errorf("remuda on the machine: %s", msg))
		return
	}
	defer up.CloseNow()
	up.SetReadLimit(1 << 20)

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionNoContextTakeover})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(1 << 20)

	rctx, rcancel := context.WithCancel(ctx)
	defer rcancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			typ, data, err := up.Read(rctx)
			if err != nil {
				var ce websocket.CloseError
				if errors.As(err, &ce) {
					c.Close(ce.Code, ce.Reason)
				} else {
					c.Close(websocket.StatusGoingAway, "remuda on the machine went away")
				}
				return
			}
			if err := c.Write(rctx, typ, data); err != nil {
				return
			}
		}
	}()
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			break
		}
		if err := up.Write(ctx, typ, data); err != nil {
			break
		}
	}
	rcancel()
	<-done
}

// peerHTTP forwards one request of the HTTP transport to the peer and passes its answer back as it came.
func (s *Server) peerHTTP(w http.ResponseWriter, r *http.Request, pc *peerClient, path string, q url.Values) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	var body io.Reader
	if r.Body != nil {
		body = io.LimitReader(r.Body, 1<<20)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, pc.base+path+"?"+q.Encode(), body)
	if err != nil {
		httpErr(w, 500, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+pc.token)
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		httpErr(w, 502, fmt.Errorf("remuda on the machine: %w", err))
		return
	}
	defer res.Body.Close()
	for _, h := range []string{"Content-Type", "X-Remuda-End", "Cache-Control"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}

// peerOpen opens the HTTP transport's stream on the peer and hands the viewer an id that names the machine.
func (s *Server) peerOpen(w http.ResponseWriter, r *http.Request, pc *peerClient, machineID, paneID string, cols, rows int) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	q := url.Values{"machine": {"local"}, "id": {paneID}, "cols": {fmt.Sprint(cols)}, "rows": {fmt.Sprint(rows)}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, pc.base+"/api/term/open?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+pc.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		httpErr(w, 502, fmt.Errorf("remuda on the machine: %w", err))
		return
	}
	defer res.Body.Close()
	var out struct {
		SID   string `json:"sid"`
		Error string `json:"error"`
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if res.StatusCode != http.StatusOK || json.Unmarshal(body, &out) != nil || out.SID == "" {
		httpErr(w, res.StatusCode, fmt.Errorf("remuda on the machine: %s", strings.TrimSpace(string(body))))
		return
	}
	writeJSON(w, map[string]any{"sid": machineID + peerSep + out.SID})
}

// peerStream forwards a transport request whose stream lives on a peer; false when the id is a local one.
func (s *Server) peerStream(w http.ResponseWriter, r *http.Request, path string) bool {
	machineID, remote, ok := splitPeerSID(r.URL.Query().Get("sid"))
	if !ok {
		return false
	}
	mc, err := s.Machines.Get(machineID)
	if err != nil {
		httpErr(w, http.StatusGone, fmt.Errorf("stream expired"))
		return true
	}
	pc, err := s.peerFor(mc)
	if err != nil || pc == nil {
		httpErr(w, http.StatusGone, fmt.Errorf("stream expired"))
		return true
	}
	q := r.URL.Query()
	q.Set("sid", remote)
	s.peerHTTP(w, r, pc, path, q)
	return true
}

// splitPeerSID takes a relayed stream id apart; ok is false for a local stream.
func splitPeerSID(sid string) (machineID, remote string, ok bool) {
	i := strings.Index(sid, peerSep)
	if i < 0 {
		return "", "", false
	}
	return sid[:i], sid[i+1:], true
}
