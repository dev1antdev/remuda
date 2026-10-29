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

	"github.com/dev1antdev/remuda/internal/push"
)

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
