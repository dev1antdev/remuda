package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/limits"
)

// offerServer is the limits routes behind the guard with one offer open, the person's device, a viewer link's screen and
// an agent's key
func offerServer(t *testing.T) (h http.Handler, svc *limits.Service, person, screen *http.Cookie, key string) {
	t.Helper()
	dir := t.TempDir()
	devices, err := auth.OpenDevices(dir)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := auth.OpenKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc = limits.New(dir)
	svc.Offer(limits.Offer{Kind: limits.OfferTask, Project: 1, Task: 12, Ref: "#12", Title: "Fix the login page", Machine: "local", From: "claude", To: "codex", Why: "Claude's 5 h window is at 96 %", Episode: "claude|5h|1", ResetsAt: time.Now().Add(time.Hour)})
	s := &Server{Guard: auth.NewGuard(deviceTestToken, devices, keys), Keys: keys, Limits: svc}
	mux := http.NewServeMux()
	s.limitRoutes(mux)
	mine, _, _ := devices.Mint("10.0.0.1", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	_, link, err := keys.MintViewer("the office TV")
	if err != nil {
		t.Fatal(err)
	}
	tv, _, err := devices.MintViewer("10.0.0.9", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36", link.ID)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err = keys.Mint("GOG")
	if err != nil {
		t.Fatal(err)
	}
	return s.Guard.Middleware(mux), svc, &http.Cookie{Name: auth.CookieName, Value: mine}, &http.Cookie{Name: auth.CookieName, Value: tv}, key
}

func offerCall(h http.Handler, method, path string, cookie *http.Cookie, bearer string) (int, string) {
	req := httptest.NewRequest(method, path, strings.NewReader(""))
	req.Host = "cockpit.local:7317"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestOnlyThePersonReadsAndRunsAnOffer(t *testing.T) {
	h, svc, person, screen, key := offerServer(t)
	id := svc.Offers()[0].ID
	ran := 0
	svc.Run = func(o limits.Offer) (string, error) {
		ran++
		return "started #12 on Studio with Codex", nil
	}
	for _, path := range []string{"/api/limits/offers/" + id + "/run"} {
		if code, body := offerCall(h, "POST", path, screen, ""); code != 403 || body != "this link is for viewing only" {
			t.Fatalf("a viewer's screen ran an offer: %d %s", code, body)
		}
		if code, _ := offerCall(h, "POST", path, nil, key); code != 403 {
			t.Fatalf("an agent's key ran an offer: %d", code)
		}
	}
	if code, _ := offerCall(h, "GET", "/api/limits/offers", screen, ""); code != 403 {
		t.Fatalf("a viewer read the offers, task titles and all: %d", code)
	}
	if code, _ := offerCall(h, "DELETE", "/api/limits/offers/"+id, nil, key); code != 403 || ran != 0 || len(svc.Offers()) != 1 {
		t.Fatalf("an agent's key declined an offer: %d", code)
	}

	code, body := offerCall(h, "GET", "/api/limits/offers", person, "")
	var list struct{ Offers []limits.Offer }
	if err := json.Unmarshal([]byte(body), &list); code != 200 || err != nil || len(list.Offers) != 1 || list.Offers[0].To != "codex" {
		t.Fatalf("the list: %d %v %s", code, err, body)
	}
	if code, body := offerCall(h, "POST", "/api/limits/offers/"+id+"/run", person, ""); code != 200 || !strings.Contains(body, "started #12") || ran != 1 {
		t.Fatalf("run: %d %s", code, body)
	}
	if code, _ := offerCall(h, "POST", "/api/limits/offers/"+id+"/run", person, ""); code != 404 || ran != 1 {
		t.Fatalf("a taken offer ran again: %d", code)
	}
}

func TestARefusedOfferIsAConflictAndADeclineIsKept(t *testing.T) {
	h, svc, person, _, _ := offerServer(t)
	id := svc.Offers()[0].ID
	svc.Run = func(limits.Offer) (string, error) { return "", errors.New("#12 no longer waits in the Queued column") }
	if code, body := offerCall(h, "POST", "/api/limits/offers/"+id+"/run", person, ""); code != 409 || !strings.Contains(body, "no longer waits") {
		t.Fatalf("refused: %d %s", code, body)
	}
	if code, _ := offerCall(h, "DELETE", "/api/limits/offers/"+id, person, ""); code != 200 || len(svc.Offers()) != 0 {
		t.Fatalf("decline: %d", code)
	}
	if code, _ := offerCall(h, "DELETE", "/api/limits/offers/"+id, person, ""); code != 404 {
		t.Fatalf("declined twice: %d", code)
	}
}
