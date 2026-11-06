package push

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

func blockedEvent() events.PaneStatus {
	return events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "p1"}, Workspace: "api", Agent: "claude", From: "working", To: "blocked"}
}

func TestSocketDeviceIsMintedUpdatedAndListed(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := s.SubscribeSocket("", nil, "remuda.app on the Mac")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(endpoint, SocketPrefix) || len(endpoint) != len(SocketPrefix)+32 {
		t.Fatalf("endpoint %q: want %s<32 hex>", endpoint, SocketPrefix)
	}
	subs := s.Subscriptions()
	if len(subs) != 1 || subs[0].Endpoint != endpoint || !subs[0].Prefs.Input || subs[0].Prefs.Done != "long" || subs[0].Label != "remuda.app on the Mac" {
		t.Fatalf("listed as %+v", subs)
	}
	if !s.HasSocket(endpoint) || s.HasSocket("https://push.example/dev") || s.HasSocket(SocketPrefix+"nope") {
		t.Fatal("HasSocket answers the wrong endpoints")
	}

	again, err := s.SubscribeSocket(endpoint, &Prefs{Done: "always"}, "")
	if err != nil || again != endpoint {
		t.Fatalf("update: %q %v", again, err)
	}
	subs = s.Subscriptions()
	if subs[0].Prefs.Done != "always" || subs[0].Prefs.Input || subs[0].Label != "remuda.app on the Mac" {
		t.Fatalf("after the update: %+v", subs[0])
	}
	if _, err := s.SubscribeSocket(SocketPrefix+"unknown", nil, "x"); err == nil {
		t.Fatal("an unknown socket device was accepted")
	}
	if _, err := s.SubscribeSocket("https://push.example/dev", nil, "x"); err == nil {
		t.Fatal("a push endpoint was taken for a socket device")
	}

	reopened, err := New(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if list := reopened.Subscriptions(); len(list) != 1 || list[0].Endpoint != endpoint {
		t.Fatalf("not persisted: %+v", list)
	}
	s.Unsubscribe(endpoint)
	if len(s.Subscriptions()) != 0 || s.HasSocket(endpoint) {
		t.Fatal("not removed")
	}
}

func TestSocketHubHandsAlertsToTheLiveConnection(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _ := s.SubscribeSocket("", nil, "app")
	s.Sockets.Deliver(endpoint, Payload{Kind: "input"}) // nobody connected: dropped, no panic

	ch, detach := s.Sockets.Attach(endpoint)
	if !s.Sockets.Connected(endpoint) {
		t.Fatal("not connected after Attach")
	}
	if err := s.SendTest(endpoint); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-ch:
		if p.Kind != "test" || p.Title != "remuda" {
			t.Fatalf("got %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("the test alert never arrived")
	}

	for i := 0; i < socketBuffer+5; i++ {
		s.Sockets.Deliver(endpoint, Payload{Kind: "done", Body: "n"})
	}
	if n := len(ch); n != socketBuffer {
		t.Fatalf("buffered %d, want %d (the rest dropped)", n, socketBuffer)
	}

	second, detachSecond := s.Sockets.Attach(endpoint)
	if _, open := <-ch; open {
		// the buffered alerts drain first; the channel must be closed behind them
		for range ch {
		}
	}
	s.Sockets.Deliver(endpoint, Payload{Kind: "input", Body: "to the second"})
	if p := <-second; p.Body != "to the second" {
		t.Fatalf("the second connection got %+v", p)
	}
	detach() // the first connection's detach must not take the second one down
	if !s.Sockets.Connected(endpoint) {
		t.Fatal("the first detach removed the second connection")
	}
	detachSecond()
	if s.Sockets.Connected(endpoint) {
		t.Fatal("still connected after detach")
	}
	if _, open := <-second; open {
		t.Fatal("the channel stays open after detach")
	}
}

func TestSocketHubCloseEndsEveryConnection(t *testing.T) {
	h := newSockets()
	a, _ := h.Attach(SocketPrefix + "a")
	b, _ := h.Attach(SocketPrefix + "b")
	h.Close()
	if !h.Closed() {
		t.Fatal("not closed")
	}
	for _, ch := range []<-chan Payload{a, b} {
		if _, open := <-ch; open {
			t.Fatal("a channel stayed open after Close")
		}
	}
	late, _ := h.Attach(SocketPrefix + "c")
	if _, open := <-late; open {
		t.Fatal("an Attach after Close got a live channel")
	}
	h.Deliver(SocketPrefix+"a", Payload{}) // dropped, no panic
}

func TestSocketDeviceHearsScheduledAlertsAndKeepsQuietHours(t *testing.T) {
	shrinkDelays(t)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _ := s.SubscribeSocket("", &Prefs{Input: true}, "app")
	ch, detach := s.Sockets.Attach(endpoint)
	defer detach()
	s.OnStatus(blockedEvent())
	select {
	case p := <-ch:
		if p.Kind != "input" {
			t.Fatalf("got %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("the question never reached the socket device")
	}

	quiet := &Quiet{From: "00:00", To: "23:59", TZ: "UTC"}
	if _, err := s.SubscribeSocket(endpoint, &Prefs{Input: true, Quiet: quiet}, ""); err != nil {
		t.Fatal(err)
	}
	s.OnStatus(blockedEvent())
	select {
	case p := <-ch:
		t.Fatalf("quiet hours ignored: %+v", p)
	case <-time.After(3 * inputDelay):
	}
}
