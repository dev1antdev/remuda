package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestATerminalRelayedToThePeerCountsAsWatched: a viewer whose terminal this remuda pipes to the machine's own remuda
// is a viewer of that pane here, while the relay stands and no longer.
func TestATerminalRelayedToThePeerCountsAsWatched(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		c.Write(r.Context(), websocket.MessageBinary, []byte("screen"))
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				return
			}
		}
	}))
	defer peer.Close()
	s := &Server{}
	cockpit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.relayPaneWS(w, r, &peerClient{base: peer.URL, token: "t"}, "gog", "w1:p1", 80, 24)
	}))
	defer cockpit.Close()

	if s.Relaying("gog", "w1:p1") {
		t.Fatal("nothing relayed yet")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	viewer, _, err := websocket.Dial(ctx, strings.Replace(cockpit.URL, "http://", "ws://", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, data, err := viewer.Read(ctx); err != nil || string(data) != "screen" {
		t.Fatalf("the peer's screen: %q %v", data, err)
	}
	if !s.Relaying("gog", "w1:p1") || s.Relaying("gog", "w1:p2") || s.Relaying("studio", "w1:p1") {
		t.Fatal("the relayed pane, and that one alone, is watched")
	}
	viewer.Close(websocket.StatusNormalClosure, "")
	deadline := time.Now().Add(5 * time.Second)
	for s.Relaying("gog", "w1:p1") {
		if time.Now().After(deadline) {
			t.Fatal("a relay that ended still counts")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
