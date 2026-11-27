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

	"github.com/dev1antdev/remuda/internal/coreapi"
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
	viewer   coreapi.Viewer
	readMu   sync.Mutex   // one long-poll at a time; never held by anyone else; guards grid and next
	grid     coreapi.Out  // the grid the bytes are drawn at now, once remuda-core told one
	next     *coreapi.Out // a grid that came after bytes a read already held: the next read opens with it
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
		s.viewer.Close()
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
	s.Radar.Seen(mc.ID(), pane.PaneID)
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	viewer, err := attach.Open(pane.TerminalID, coreapi.View{Cols: cols, Rows: rows, Keep: r.URL.Query().Get("keep") == "1"})
	if err != nil {
		httpErr(w, 409, fmt.Errorf("%s", attachRefused(err)))
		return
	}
	st := &termStream{id: newStreamID(), viewer: viewer}
	st.lastRead.Store(time.Now().UnixNano())
	s.streams.add(st)
	writeJSON(w, map[string]any{"sid": st.id})
}

// handleTermRead is GET /api/term/read?sid&wait=<seconds> → raw bytes: the first chunk, then what else is queued.
// X-Remuda-End: 1 says the attach is gone; X-Remuda-Grid: <cols>x<rows> is the grid these bytes are drawn at
// (remuda-core's, for a viewer that keeps the size), on every read once known, so a read lost on the way loses no
// grid; a grid that changes after some bytes ends the read; 410 says the stream is unknown and the client reopens.
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

	st.readMu.Lock()
	defer st.readMu.Unlock()
	st.lastRead.Store(time.Now().UnixNano())
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")

	var out []byte
	ended, full := false, false
	if st.next != nil {
		st.grid, st.next = *st.next, nil
	}
	take := func(next coreapi.Out, ok bool) {
		switch {
		case !ok:
			ended = true
		case next.Grid() && len(out) > 0: // the bytes held are drawn at the grid before it: it opens the next read
			st.next, full = &next, true
		case next.Grid():
			st.grid = next
		default:
			out = append(out, next.Data...)
		}
	}
	select {
	case next, ok := <-st.viewer.Out():
		take(next, ok)
	case <-time.After(time.Duration(wait) * time.Second):
	case <-r.Context().Done():
		return
	}
	for !ended && !full && len(out) < streamMaxChunk {
		select {
		case next, ok := <-st.viewer.Out():
			take(next, ok)
			continue
		default:
		}
		break
	}
	if st.grid.Grid() {
		w.Header().Set("X-Remuda-Grid", fmt.Sprintf("%dx%d", st.grid.Cols, st.grid.Rows))
	}
	if ended {
		w.Header().Set("X-Remuda-End", "1")
		s.streams.remove(st.id)
	}
	w.Write(out)
}

// termInputMax is the most one input request carries: a paste, not a file.
const termInputMax = 1 << 20

// handleTermInput is POST /api/term/input?sid: the body is what the keyboard typed.
func (s *Server) handleTermInput(w http.ResponseWriter, r *http.Request) {
	st := s.streams.get(r.URL.Query().Get("sid"))
	if st == nil {
		httpErr(w, http.StatusGone, fmt.Errorf("stream expired"))
		return
	}
	// one byte past the bound tells a body that was cut from one that fit: a paste's tail must not vanish into a 204
	data, err := io.ReadAll(io.LimitReader(r.Body, termInputMax+1))
	if err != nil {
		httpErr(w, 400, err)
		return
	}
	if len(data) > termInputMax {
		httpErr(w, http.StatusRequestEntityTooLarge, fmt.Errorf("that is more than %d KB of typing at once: paste it in smaller pieces", termInputMax>>10))
		return
	}
	if len(data) > 0 {
		if err := st.viewer.Write(data); err != nil {
			httpErr(w, 502, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTermResize is POST /api/term/resize?sid&cols&rows.
func (s *Server) handleTermResize(w http.ResponseWriter, r *http.Request) {
	st := s.streams.get(r.URL.Query().Get("sid"))
	if st == nil {
		httpErr(w, http.StatusGone, fmt.Errorf("stream expired"))
		return
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	st.viewer.Resize(cols, rows)
	w.WriteHeader(http.StatusNoContent)
}

// handleTermClose is DELETE /api/term/close?sid.
func (s *Server) handleTermClose(w http.ResponseWriter, r *http.Request) {
	s.streams.remove(r.URL.Query().Get("sid"))
	w.WriteHeader(http.StatusNoContent)
}
