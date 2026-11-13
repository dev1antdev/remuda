package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/push"
)

// The Mac app away when an alert was taken back hears it as it connects again, asking for withdrawals; a connection
// that does not ask (an app from before them) never gets a frame it would show as a blank alert.
func TestAlertsSocketHandsOwedWithdrawalsToAConnectionThatTakesThem(t *testing.T) {
	s, mux := alertsServer(t)
	_, out := taskCall(mux, "POST", "/api/push/socket", "", `{"label":"app"}`)
	endpoint := out["endpoint"].(string)
	const tag = "local|w1:p1|stuck|1800000000"
	s.Push.Notify(push.Payload{Machine: "local", Pane: "w1:p1", Kind: "stuck", Title: "api looks stuck", Tag: tag})
	s.Push.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "w1:p1"}, To: events.Gone})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	base := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/alerts?endpoint=" + endpoint

	before, _, err := websocket.Dial(ctx, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	quiet, stop := context.WithTimeout(ctx, 200*time.Millisecond)
	if _, data, err := before.Read(quiet); err == nil {
		t.Fatalf("a connection that takes no withdrawals heard %q", data)
	}
	stop()
	before.CloseNow()
	// a handler let go of has ended its writer: the next test's pings are its own
	gone := func() {
		for deadline := time.Now().Add(2 * time.Second); s.Push.Sockets.Connected(endpoint); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal("the connection was never let go of")
			}
		}
	}
	gone()

	c, _, err := websocket.Dial(ctx, base+"&withdraw=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer gone()
	defer c.CloseNow()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Type string   `json:"type"`
		Tags []string `json:"tags"`
	}
	if json.Unmarshal(data, &frame) != nil || frame.Type != "withdraw" || len(frame.Tags) != 1 || frame.Tags[0] != tag {
		t.Fatalf("first frame %q, want the stuck note taken back", data)
	}
	if owed := s.Push.OwedTo(endpoint); len(owed) != 0 {
		t.Fatalf("still owed %v after the connection took them", owed)
	}
}

func alertsServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	svc, err := push.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Push: svc}
	mux := http.NewServeMux()
	s.alertRoutes(mux)
	return s, mux
}

func TestSocketDeviceRouteMintsAndUpdates(t *testing.T) {
	s, mux := alertsServer(t)
	code, out := taskCall(mux, "POST", "/api/push/socket", "", `{"label":"remuda.app"}`)
	endpoint, _ := out["endpoint"].(string)
	if code != 200 || !strings.HasPrefix(endpoint, push.SocketPrefix) {
		t.Fatalf("mint: %d %v", code, out)
	}
	subs := s.Push.Subscriptions()
	if len(subs) != 1 || subs[0].Label != "remuda.app" || !subs[0].Prefs.Input || subs[0].Prefs.Done != "long" {
		t.Fatalf("minted as %+v", subs)
	}
	code, out = taskCall(mux, "POST", "/api/push/socket", "", `{"endpoint":"`+endpoint+`","prefs":{"input":false,"done":"always"}}`)
	if code != 200 || out["endpoint"] != endpoint {
		t.Fatalf("update: %d %v", code, out)
	}
	if subs = s.Push.Subscriptions(); subs[0].Prefs.Input || subs[0].Prefs.Done != "always" || subs[0].Label != "remuda.app" {
		t.Fatalf("after the update: %+v", subs[0])
	}
	if code, _ = taskCall(mux, "POST", "/api/push/socket", "", `{"endpoint":"socket:nope"}`); code != 404 {
		t.Fatalf("unknown device: %d, want 404", code)
	}
}

func TestAlertsSocketRefusesBeforeTheUpgrade(t *testing.T) {
	_, mux := alertsServer(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/ws/alerts?endpoint=socket:nope", nil))
	if rec.Code != 404 {
		t.Fatalf("unknown device: %d, want 404", rec.Code)
	}
	_, out := taskCall(mux, "POST", "/api/push/socket", "", `{}`)
	endpoint := out["endpoint"].(string)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/ws/alerts?endpoint="+endpoint, nil))
	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("plain GET: %d, want 426", rec.Code)
	}
}

// The Mac app asleep or gone answers no ping: its device is let go of, so the alerts go no more into a socket nobody reads.
func TestAlertsSocketLetsGoOfADeviceThatDoesNotAnswerPings(t *testing.T) {
	quickPings(t)
	s, mux := alertsServer(t)
	_, out := taskCall(mux, "POST", "/api/push/socket", "", `{"label":"app"}`)
	endpoint := out["endpoint"].(string)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/alerts?endpoint="+endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow() // never read: a client answers pings only while it reads
	for i := 0; i < 50 && !s.Push.Sockets.Connected(endpoint); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if !s.Push.Sockets.Connected(endpoint) {
		t.Fatal("the device never connected")
	}
	for deadline := time.Now().Add(2 * time.Second); s.Push.Sockets.Connected(endpoint); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("a device that answers no ping still counts as connected")
		}
	}
}

func TestAlertsSocketCarriesTheDevicesAlerts(t *testing.T) {
	s, mux := alertsServer(t)
	_, out := taskCall(mux, "POST", "/api/push/socket", "", `{"label":"app"}`)
	endpoint := out["endpoint"].(string)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/alerts?endpoint=" + endpoint
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	for i := 0; i < 50 && !s.Push.Sockets.Connected(endpoint); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if err := s.Push.SendTest(endpoint); err != nil {
		t.Fatal(err)
	}
	typ, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var p push.Payload
	if typ != websocket.MessageText || json.Unmarshal(data, &p) != nil || p.Kind != "test" || p.Title != "remuda" {
		t.Fatalf("frame %v %q", typ, data)
	}

	s.Push.Sockets.Close()
	if _, _, err := c.Read(ctx); err == nil || !strings.Contains(err.Error(), "remuda is stopping") {
		t.Fatalf("after Close the read ended with %v, want the stopping reason", err)
	}
}
