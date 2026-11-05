package preview

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// loopbackService opens every preview on a free loopback port and lets everyone in
func loopbackService() *Service {
	return &Service{
		Listen: func(port int, host string) (net.Listener, string, error) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return nil, "", err
			}
			return ln, fmt.Sprintf("http://%s:%d", host, ln.Addr().(*net.TCPAddr).Port), nil
		},
		Allow:     func(*http.Request) bool { return true },
		IdleAfter: time.Minute,
	}
}

func TestOpenServesThePortAndClosesItAgain(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "the app") }))
	defer app.Close()
	dial := func() (string, error) { return strings.TrimPrefix(app.URL, "http://"), nil }
	s := loopbackService()
	defer s.Shutdown()
	p, err := s.Open("gog", 3000, dial, "127.0.0.1", "http://127.0.0.1:7317")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Open("gog", 3000, dial, "127.0.0.1", "http://127.0.0.1:7317"); again.URL != p.URL {
		t.Fatalf("a second Open made another preview: %s, then %s", p.URL, again.URL)
	}
	res, err := http.Get(p.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "the app" {
		t.Fatalf("the preview answered %q", body)
	}
	if list := s.List(); len(list) != 1 || list[0].Machine != "gog" || list[0].Port != 3000 {
		t.Fatalf("open previews %+v", list)
	}
	if !s.Close("gog", 3000) || s.Close("gog", 3000) {
		t.Fatal("Close did not say what it closed")
	}
	if _, err := http.Get(p.URL + "/"); err == nil {
		t.Fatal("a closed preview still answers")
	}
}

func TestAPreviewNobodyAsksForCloses(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer app.Close()
	dial := func() (string, error) { return strings.TrimPrefix(app.URL, "http://"), nil }
	s := loopbackService()
	defer s.Shutdown()
	if _, err := s.Open("gog", 3000, dial, "127.0.0.1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open("roofr", 3000, dial, "127.0.0.1", ""); err != nil {
		t.Fatal(err)
	}
	s.open[key("roofr", 3000)].holds.Add(1)
	s.closeIdleAt(time.Now().Add(30 * time.Second))
	if len(s.List()) != 2 {
		t.Fatal("a preview closed before its idle time")
	}
	s.closeIdleAt(time.Now().Add(2 * time.Minute))
	if list := s.List(); len(list) != 1 || list[0].Machine != "roofr" {
		t.Fatalf("after the idle time %+v stayed; only the one holding a connection should", list)
	}
}

func TestOpenRefusesAMachineItCannotReachAndAPortItCannotListenOn(t *testing.T) {
	s := loopbackService()
	defer s.Shutdown()
	if _, err := s.Open("gog", 3000, func() (string, error) { return "", errors.New("gog is away") }, "h", ""); err == nil || !strings.Contains(err.Error(), "away") {
		t.Fatalf("an unreachable machine opened: %v", err)
	}
	s.Listen = func(int, string) (net.Listener, string, error) { return nil, "", ErrNotReady }
	if _, err := s.Open("gog", 3000, func() (string, error) { return "x:1", nil }, "h", ""); !errors.Is(err, ErrNotReady) {
		t.Fatalf("a cockpit not on the tailnet yet opened: %v", err)
	}
}

func TestTheCockpitPortsTriedSkipTheOnesABrowserRefuses(t *testing.T) {
	var tried []int
	s := &Service{Listen: func(port int, host string) (net.Listener, string, error) {
		tried = append(tried, port)
		return nil, "", errors.New("taken")
	}}
	if _, _, _, err := s.listen(6000, "h"); err == nil {
		t.Fatal("no port was free, yet one opened")
	}
	if len(tried) == 0 || tried[0] == 6000 {
		t.Fatalf("tried %v: 6000 is a port browsers refuse", tried)
	}
	if tried[0] != 41000 {
		t.Fatalf("the range of its own starts at %d", tried[0])
	}
}
