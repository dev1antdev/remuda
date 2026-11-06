package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/dev1antdev/remuda/internal/live"
)

// quickPings makes the sockets ping often and wait little for the answer, for as long as the test runs.
func quickPings(t *testing.T) {
	t.Helper()
	ping, wait := livePing, livePongWait
	livePing, livePongWait = 20*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { livePing, livePongWait = ping, wait })
}

// A page that stops answering pings — a phone asleep, a tab frozen — is let go of: its stream ends, its subscription with it.
func TestEventsSocketLetsGoOfAPageThatDoesNotAnswerPings(t *testing.T) {
	quickPings(t)
	s := &Server{Live: live.New(live.Inputs{})}
	done := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/events", func(w http.ResponseWriter, r *http.Request) {
		s.handleEventsWS(w, r)
		close(done)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow() // never read: a client answers pings only while it reads
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream of a page that answers no ping still stands")
	}
}
