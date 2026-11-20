package core

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestStreamsAskingForTheSameWatchesShareOneFeed(t *testing.T) {
	var mu sync.Mutex
	var core peer
	opened, closed := 0, map[uint32]bool{}
	client := New(fakeCore(`"output_watches"`, func(c peer, frame sent) {
		mu.Lock()
		defer mu.Unlock()
		core = c
		switch {
		case frame.kind == frameClose:
			closed[frame.channel] = true
		case frame.method == "events.subscribe" && bytes.Contains(frame.payload, []byte(outputOnly)):
			opened++
			c.reply(frame.id, fmt.Sprintf(`{"seq":1,"terminals":[],"channel":%d}`, 3+opened))
		case frame.method == "events.subscribe":
			c.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
		case frame.method == "doc.get":
			c.reply(frame.id, `{"value":null,"version":0}`)
		}
	}))
	t.Cleanup(func() { client.Close() })
	adapter := NewAdapter(client, AdapterHooks{})
	t.Cleanup(func() { adapter.Close() })
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	watching := func(value string) herdr.EventStream {
		t.Helper()
		stream, err := adapter.Subscribe([]herdr.Subscription{{Type: herdr.EventOutputMatched, PaneID: "t1", Match: &herdr.OutputMatch{Type: "substring", Value: value}}})
		if err != nil {
			t.Fatal(err)
		}
		return stream
	}
	tell := func(channel uint32, line string) {
		mu.Lock()
		defer mu.Unlock()
		core.write(channel, frameData, []byte(`{"jsonrpc":"2.0","method":"event","params":{"seq":2,"type":"output","data":{"watch":"0","line":"`+line+`","fresh":false}}}`+"\n"))
	}
	heard := func(stream herdr.EventStream, want string) {
		t.Helper()
		for {
			ev, err := stream.Next()
			if err != nil {
				t.Fatalf("waiting for %q: %v", want, err)
			}
			if ev.Kind == herdr.EventOutputMatched {
				if ev.MatchedLine != want || ev.PaneID != "t1" {
					t.Fatalf("heard %+v, want %q", ev, want)
				}
				return
			}
		}
	}
	state := func() (int, map[uint32]bool) {
		mu.Lock()
		defer mu.Unlock()
		shut := map[uint32]bool{}
		for k, v := range closed {
			shut[k] = v
		}
		return opened, shut
	}

	before, after := watching("FAIL"), watching("FAIL") // the watcher's swap: the new stream beside the old
	if n, _ := state(); n != 1 {
		t.Fatalf("two streams with the same watches opened %d core subscriptions", n)
	}
	tell(4, "a FAIL")
	heard(before, "a FAIL")
	heard(after, "a FAIL")
	before.Close()
	tell(4, "b FAIL")
	heard(after, "b FAIL")
	other := watching("PASS")
	if n, shut := state(); n != 2 || shut[4] {
		t.Fatalf("other watches open their own feed and the shared one stands: %d opened, closed %v", n, shut)
	}
	after.Close()
	other.Close()
	for start := time.Now(); ; time.Sleep(10 * time.Millisecond) {
		if _, shut := state(); shut[4] && shut[5] {
			break
		}
		if time.Since(start) > 2*time.Second {
			_, shut := state()
			t.Fatalf("a feed no stream is left on is closed: %v", shut)
		}
	}
	again := watching("FAIL")
	defer again.Close()
	if n, _ := state(); n != 3 {
		t.Fatalf("a closed feed is not joined again: %d opened", n)
	}
}
