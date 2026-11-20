package core

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

func TestACoreThatFallsSilentIsDialledAgainAndItsMirrorDropped(t *testing.T) {
	quiet := quietFor
	quietFor = 200 * time.Millisecond
	t.Cleanup(func() { quietFor = quiet })
	var dials atomic.Int32
	answer := fakeCore(`"output_watches"`, func(core peer, frame sent) {
		switch frame.method {
		case "events.subscribe":
			core.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live","cols":80,"rows":24}],"channel":3}`)
		case "doc.get":
			core.reply(frame.id, `{"value":null,"version":0}`)
		}
	})
	client := New(func(ctx context.Context) (net.Conn, error) {
		dials.Add(1)
		return answer(ctx)
	})
	defer client.Close()
	var lost atomic.Int32
	adapter := NewAdapter(client, AdapterHooks{TransportError: func(error) { lost.Add(1) }})
	defer adapter.Close()
	if snap, err := adapter.SessionSnapshot(); err != nil || len(snap.Panes) != 1 {
		t.Fatalf("first snapshot %+v %v", snap, err)
	}
	for start := time.Now(); dials.Load() < 2 || lost.Load() == 0; time.Sleep(20 * time.Millisecond) {
		if time.Since(start) > 5*time.Second {
			t.Fatalf("after the core fell silent: %d dials, %d losses", dials.Load(), lost.Load())
		}
	}
}

func TestATerminalOfAMachineTheCoreAddedStaysOutOfTheMirror(t *testing.T) {
	m := mirror{terminals: map[string]*Terminal{}}
	if _, ok := m.apply(coreEvent("created", "rohan/t7", Terminal{Address: "rohan/t7", State: "live"})); ok || len(m.terminals) != 0 {
		t.Fatalf("a remote terminal came in: %v", m.terminals)
	}
	if _, ok := m.apply(coreEvent("created", "t1", Terminal{Address: "t2", State: "live"})); ok {
		t.Fatal("a record under another address came in")
	}
}

func TestAColouredReadReadsAsText(t *testing.T) {
	if got := withoutSGR("\x1b[0m\x1b[31mred\x1b[0m!\n\x1b[0m\x1b[1;38;2;1;2;3mbold"); got != "red!\nbold" {
		t.Fatalf("%q", got)
	}
}

func TestOutputRowsTheCoreDroppedEndTheStreamAsHerdrsLostEvents(t *testing.T) {
	var subscribes atomic.Int32
	client := New(fakeCore(`"output_watches"`, func(core peer, frame sent) {
		switch frame.method {
		case "events.subscribe":
			if subscribes.Add(1) == 1 {
				core.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
				return
			}
			core.reply(frame.id, `{"seq":1,"terminals":[],"channel":4}`)
			go core.write(4, frameData, []byte(`{"jsonrpc":"2.0","method":"missed","params":{"seq":9,"terminals":[]}}`+"\n"))
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
	for {
		_, err := stream.Next()
		if err == nil {
			continue
		}
		if !herdr.IsCode(err, herdr.ErrEventsLost) {
			t.Fatalf("the stream ended with %v", err)
		}
		return
	}
}

func TestACoreOlderThanRemudaNeedsIsRefused(t *testing.T) {
	client := New(fakeCore(`"events","keep_size"`, func(peer, sent) {}))
	defer client.Close()
	adapter := NewAdapter(client, AdapterHooks{})
	defer adapter.Close()
	start := time.Now()
	if _, err := adapter.SessionSnapshot(); !IsKind(err, "needs_update") || time.Since(start) > 5*time.Second {
		t.Fatalf("a core without output watches: %v after %s", err, time.Since(start))
	}
	if _, _, err := adapter.PaneRecent("t1", 10); err == nil {
		t.Fatal("a read went to a core the adapter refused")
	}
}
