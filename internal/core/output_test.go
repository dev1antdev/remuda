package core

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestRowsTheCoreDroppedAreLoggedOnceAndTheStreamGoesOn(t *testing.T) {
	var said bytes.Buffer
	before := log.Writer()
	log.SetOutput(&said)
	t.Cleanup(func() { log.SetOutput(before) })
	var subscribes int
	client := New(fakeCore(`"output_watches"`, func(core peer, frame sent) {
		switch frame.method {
		case "events.subscribe":
			subscribes++
			if subscribes == 1 {
				core.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
				return
			}
			core.reply(frame.id, `{"seq":1,"terminals":[],"channel":4}`)
			go func() {
				for _, row := range []string{`"line":"a FAIL","lost":7`, `"line":"b FAIL","lost":2`, `"line":"c FAIL"`} {
					core.write(4, frameData, []byte(`{"jsonrpc":"2.0","method":"event","params":{"seq":2,"type":"output","data":{"watch":"0",`+row+`}}}`+"\n"))
				}
			}()
		case "doc.get":
			core.reply(frame.id, `{"value":null,"version":0}`)
		}
	}))
	defer client.Close()
	adapter := NewAdapter(client, AdapterHooks{})
	defer adapter.Close()
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	stream, err := adapter.Subscribe([]herdr.Subscription{{Type: herdr.EventOutputMatched, PaneID: "t1", Match: &herdr.OutputMatch{Type: "substring", Value: "FAIL"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var rows []string
	for len(rows) < 3 {
		ev, err := stream.Next()
		if err != nil {
			t.Fatalf("the stream ended after %v: %v", rows, err)
		}
		if ev.Kind == herdr.EventOutputMatched {
			rows = append(rows, ev.MatchedLine)
		}
	}
	if strings.Join(rows, ",") != "a FAIL,b FAIL,c FAIL" {
		t.Fatalf("rows %v", rows)
	}
	if n := strings.Count(said.String(), "dropped"); n != 1 {
		t.Fatalf("the drops were logged %d times: %s", n, said.String())
	}
}
