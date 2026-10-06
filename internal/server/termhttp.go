package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/dev1antdev/herdr-ui/internal/herdr"
)

// The terminal over plain HTTP: for viewers whose path to the server passes
// a proxy that forwards requests but not WebSockets (JetBrains' embedded
// browser in Remote Development is one). A stream is a server-side
// subscription to the pane's attach; the client long-polls it for output and
// POSTs input. Same bytes, same attach sharing, a little more latency.

const (
	streamIdle     = 90 * time.Second // no read for this long → the stream is dropped
	streamMaxWait  = 25 * time.Second
	streamMaxChunk = 512 * 1024
)

type termStream struct {
	id       string
	termID   string
	attach   *herdr.AttachManager
	ch       chan []byte
	cancel   func()
	replay   []byte
	mu       sync.Mutex // one reader at a time
	lastRead time.Time
	closed   bool
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
		s.mu.Lock()
		if !s.closed {
			s.closed = true
			s.cancel()
		}
		s.mu.Unlock()
	}
}

// sweep drops streams nobody has read for a while — a tab closed without
// saying goodbye must not pin the attach open.
func (ts *termStreams) sweep() {
	for {
		time.Sleep(15 * time.Second)
		var stale []string
		ts.mu.Lock()
		for id, s := range ts.streams {
			s.mu.Lock()
			if time.Since(s.lastRead) > streamIdle {
				stale = append(stale, id)
			}
			s.mu.Unlock()
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

// POST /api/term/open?machine&id&cols&rows → {"sid": …}
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
	replay, ch, cancel, err := attach.Subscribe(pane.TerminalID, cols, rows)
	if err != nil {
		httpErr(w, 502, fmt.Errorf("attach failed: %w", err))
		return
	}
	st := &termStream{
		id: newStreamID(), termID: pane.TerminalID, attach: attach,
		ch: ch, cancel: cancel, replay: replay, lastRead: time.Now(),
	}
	s.streams.add(st)
	writeJSON(w, map[string]any{"sid": st.id})
}

// GET /api/term/read?sid&wait=<seconds> → raw bytes; waits for the first
// chunk, then drains what else is queued. X-Herdr-UI-End: 1 when the attach
// is gone, 410 when the stream is unknown (the client reopens).
func (s *Server) handleTermRead(w http.ResponseWriter, r *http.Request) {
	st := s.streams.get(r.URL.Query().Get("sid"))
	if st == nil {
		httpErr(w, http.StatusGone, fmt.Errorf("stream expired"))
		return
	}
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait <= 0 || time.Duration(wait)*time.Second > streamMaxWait {
		wait = int(streamMaxWait / time.Second)
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	st.lastRead = time.Now()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")

	var out []byte
	ended := false
	if len(st.replay) > 0 {
		out, st.replay = st.replay, nil
	} else {
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
		w.Header().Set("X-Herdr-UI-End", "1")
		st.closed = true
		st.cancel()
	}
	w.Write(out)
}

// POST /api/term/input?sid — body is what the keyboard typed.
func (s *Server) handleTermInput(w http.ResponseWriter, r *http.Request) {
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

// POST /api/term/resize?sid&cols&rows
func (s *Server) handleTermResize(w http.ResponseWriter, r *http.Request) {
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

// DELETE /api/term/close?sid
func (s *Server) handleTermClose(w http.ResponseWriter, r *http.Request) {
	s.streams.remove(r.URL.Query().Get("sid"))
	w.WriteHeader(http.StatusNoContent)
}
