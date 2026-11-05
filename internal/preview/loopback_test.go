package preview

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestALoopbackPreviewNeverTakesAPortTheHostAnswersOn(t *testing.T) {
	v6, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback here")
	}
	defer v6.Close()
	taken := v6.Addr().(*net.TCPAddr).Port
	listen := LoopbackListen("127.0.0.1:7317")
	if ln, _, err := listen(taken, "localhost"); !errors.Is(err, errInUse) {
		if ln != nil {
			ln.Close()
		}
		t.Fatalf("a port a server answers on over ::1 was taken on 127.0.0.1: %v", err)
	}
	free, _ := net.Listen("tcp", "127.0.0.1:0")
	port := free.Addr().(*net.TCPAddr).Port
	free.Close()
	ln, url, err := listen(port, "::1")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
	if url != fmt.Sprintf("http://[::1]:%d", port) {
		t.Fatalf("the preview's address %q", url)
	}
}

func TestAMachineLeavingTheFleetTakesItsPreviewsAlong(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "the app") }))
	defer app.Close()
	dial := func() (string, error) { return strings.TrimPrefix(app.URL, "http://"), nil }
	s := loopbackService()
	gog, err := s.Open("gog", 3000, dial, "127.0.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open("roofr", 3000, dial, "127.0.0.1", ""); err != nil {
		t.Fatal(err)
	}
	s.CloseMachine("gog")
	if list := s.List(); len(list) != 1 || list[0].Machine != "roofr" {
		t.Fatalf("after gog left %+v stayed open", list)
	}
	if _, err := http.Get(gog.URL + "/"); err == nil {
		t.Fatal("the preview of a machine that left still answers")
	}
	s.Shutdown()
	if _, err := s.Open("roofr", 4000, dial, "127.0.0.1", ""); !errors.Is(err, errShutdown) {
		t.Fatalf("a preview opened after remuda began to stop: %v", err)
	}
}
