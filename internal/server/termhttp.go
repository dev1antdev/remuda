package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// The terminal over plain HTTP, for viewers behind a proxy that forwards requests
// but not WebSockets: a stream is a server-side subscription the client long-polls.

const (
	streamIdle     = 90 * time.Second
	streamMaxWait  = 25 * time.Second
	streamMaxChunk = 512 * 1024
)

type termStream struct {
	id       string
	termID   string
	attach   *herdr.AttachManager
	ch       chan []byte
	cancel   func()
	readMu   sync.Mutex // one long-poll at a time; never held by anyone else
	lastRead atomic.Int64
	mu       sync.Mutex
	closed   bool
}

// end stops the subscription once.
func (s *termStream) end() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.cancel()
	}
}

type termStreams struct {
	mu      sync.Mutex
	streams map[string]*termStream
	janitor sync.Once
}

func (ts *termStreams) add(s *termStream) {
	ts.mu.Lock()
	if ts.streams == nil {
		ts.streams = map[string]*termStream{}
	}
	ts.streams[s.id] = s
	ts.mu.Unlock()
	ts.janitor.Do(func() { go ts.sweep() })
}

func (ts *termStreams) get(id string) *termStream {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.streams[id]
}

func (ts *termStreams) remove(id string) {
	ts.mu.Lock()
	s := ts.streams[id]
	delete(ts.streams, id)
	ts.mu.Unlock()
	if s != nil {
		s.end()
	}
}

// sweep drops streams nobody has read for a while, so a tab closed without a goodbye does not pin the attach open.
func (ts *termStreams) sweep() {
	for {
		time.Sleep(15 * time.Second)
		var stale []string
		ts.mu.Lock()
		for id, s := range ts.streams {
			if time.Since(time.Unix(0, s.lastRead.Load())) > streamIdle {
				stale = append(stale, id)
			}
		}
		ts.mu.Unlock()
		for _, id := range stale {
			ts.remove(id)
		}
	}
}

func newStreamID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// handleTermOpen is POST /api/term/open?machine&id&cols&rows → {"sid": …}.
func (s *Server) handleTermOpen(w http.ResponseWriter, r *http.Request) {
	mc, _, attach, ok := s.ready(w, r, "")
	if !ok {
		return
	}
	pane, err := s.findPane(mc, r.URL.Query().Get("id"))
	if err != nil {
		httpErr(w, 404, err)
		return
	}
	s.Radar.Seen(mc.Cfg.ID, pane.PaneID)
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	open := func(pc *peerClient) {
		s.peerOpen(w, r, pc, mc.Cfg.ID, pane.PaneID, cols, rows)
	}
	if pc, err := s.peerFor(mc); err != nil {
		httpErr(w, 502, err)
		return
	} else if pc != nil {
		open(pc)
		return
	}
	ch, cancel, err := attach.Subscribe(pane.TerminalID, cols, rows)
	if err != nil {
		if pc := s.peerAfterRefusal(mc, err); pc != nil {
			open(pc)
			return
		}
		httpErr(w, 409, fmt.Errorf("%s", attachRefused(mc, err)))
		return
	}
	st := &termStream{id: newStreamID(), termID: pane.TerminalID, attach: attach, ch: ch, cancel: cancel}
	st.lastRead.Store(time.Now().UnixNano())
	s.streams.add(st)
	writeJSON(w, map[string]any{"sid": st.id})
}

// handleTermRead is GET /api/term/read?sid&wait=<seconds> → raw bytes: the first chunk, then what else is queued.
// X-Remuda-End: 1 says the attach is gone; 410 says the stream is unknown and the client reopens.
func (s *Server) handleTermRead(w http.ResponseWriter, r *http.Request) {
	if s.peerStream(w, r, "/api/term/read") {
		return
	}
	st := s.streams.get(r.URL.Query().Get("sid"))
	if st == nil {
		httpErr(w, http.StatusGone, fmt.Errorf("stream expired"))
		return
	}
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait <= 0 || time.Duration(wait)*time.Second > streamMaxWait {
		wait = int(streamMaxWait / time.Second)
	}

	st.readMu.Lock()
	defer st.readMu.Unlock()
	st.lastRead.Store(time.Now().UnixNano())
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")

	var out []byte
	ended := false
	select {
	case data, ok := <-st.ch:
		if !ok {
			ended = true
		} else {
			out = append(out, data...)
		}
	case <-time.After(time.Duration(wait) * time.Second):
	case <-r.Context().Done():
		return
	}
	for !ended && len(out) < streamMaxChunk {
		select {
		case data, ok := <-st.ch:
			if !ok {
				ended = true
			} else {
				out = append(out, data...)
			}
			continue
		default:
		}
		break
	}
	if ended {
		w.Header().Set("X-Remuda-End", "1")
		s.streams.remove(st.id)
	}
	w.Write(out)
}

// handleTermInput is POST /api/term/input?sid: the body is what the keyboard typed.
func (s *Server) handleTermInput(w http.ResponseWriter, r *http.Request) {
	if s.peerStream(w, r, "/api/term/input") {
		return
	}
	st := s.streams.get(r.URL.Query().Get("sid"))
	if st == nil {
		httpErr(w, http.StatusGone, fmt.Errorf("stream expired"))
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	if len(data) > 0 {
		if err := st.attach.Write(st.termID, data); err != nil {
			httpErr(w, 502, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTermResize is POST /api/term/resize?sid&cols&rows.
func (s *Server) handleTermResize(w http.ResponseWriter, r *http.Request) {
	if s.peerStream(w, r, "/api/term/resize") {
		return
	}
	st := s.streams.get(r.URL.Query().Get("sid"))
	if st == nil {
		httpErr(w, http.StatusGone, fmt.Errorf("stream expired"))
		return
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	st.attach.Resize(st.termID, cols, rows)
	w.WriteHeader(http.StatusNoContent)
}

// handleTermClose is DELETE /api/term/close?sid.
func (s *Server) handleTermClose(w http.ResponseWriter, r *http.Request) {
	if s.peerStream(w, r, "/api/term/close") {
		return
	}
	s.streams.remove(r.URL.Query().Get("sid"))
	w.WriteHeader(http.StatusNoContent)
}
