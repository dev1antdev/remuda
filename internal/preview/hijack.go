package preview

// A preview's WebSocket upgrade (a dev server's live reload) leaves net/http's hands: srv.Close closes the listener and
// the connections it still answers, never a hijacked one. The set below keeps them, so a preview closed or remuda
// stopped leaves no live-reload socket talking to the app.

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sync"
)

// hijacked is the connections a preview's upgrades took, until each is closed.
type hijacked struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

func (h *hijacked) add(c net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns == nil {
		h.conns = map[net.Conn]struct{}{}
	}
	h.conns[c] = struct{}{}
}

func (h *hijacked) remove(c net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, c)
}

// closeAll closes every connection still open, and the set with it.
func (h *hijacked) closeAll() {
	h.mu.Lock()
	conns := h.conns
	h.conns = nil
	h.mu.Unlock()
	for c := range conns {
		_ = c.Close()
	}
}

// trackedConn is a hijacked connection that leaves the set when it is closed, however it ends.
type trackedConn struct {
	net.Conn
	set  *hijacked
	once sync.Once
}

func (c *trackedConn) Close() error {
	c.once.Do(func() { c.set.remove(c) })
	return c.Conn.Close()
}

// tracking is a writer whose upgrades are noted in the set; everything else is the writer's own.
type tracking struct {
	http.ResponseWriter
	set *hijacked
}

func (t tracking) Unwrap() http.ResponseWriter { return t.ResponseWriter }

func (t tracking) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := t.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("the connection cannot be hijacked")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return conn, rw, err
	}
	tracked := &trackedConn{Conn: conn, set: t.set}
	t.set.add(tracked)
	return tracked, rw, nil
}

// keepHijacked is the handler a preview serves: its upgrades are kept in set.
func keepHijacked(next http.Handler, set *hijacked) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(tracking{ResponseWriter: w, set: set}, r)
	})
}
