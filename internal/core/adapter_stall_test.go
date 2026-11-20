package core

import (
	"bytes"
	"context"
	"log"
	"net"
	"strings"
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
	stream := outputStream(t, func(core peer) {
		core.write(4, frameData, []byte(`{"jsonrpc":"2.0","method":"missed","params":{"seq":9,"terminals":[]}}`+"\n"))
	})
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

func TestOutputRowsKeepTheirStandingAndTheirDropsAreLoggedOnce(t *testing.T) {
	var said bytes.Buffer
	before := log.Writer()
	log.SetOutput(&said)
	t.Cleanup(func() { log.SetOutput(before) })
	stream := outputStream(t, func(core peer) {
		for _, row := range []string{`"line":"a FAIL","lost":7,"fresh":true`, `"line":"b FAIL","lost":2`, `"line":"c FAIL"`} {
			core.write(4, frameData, []byte(`{"jsonrpc":"2.0","method":"event","params":{"seq":2,"type":"output","data":{"watch":"0",`+row+`}}}`+"\n"))
		}
	})
	var rows []string
	for len(rows) < 3 {
		ev, err := stream.Next()
		if err != nil {
			t.Fatalf("the stream ended after %v: %v", rows, err)
		}
		if ev.Kind == herdr.EventOutputMatched {
			rows = append(rows, ev.MatchedLine+map[bool]string{true: " standing"}[ev.Standing])
		}
	}
	if strings.Join(rows, ",") != "a FAIL standing,b FAIL,c FAIL" {
		t.Fatalf("rows %v", rows)
	}
	if n := strings.Count(said.String(), "dropped"); n != 1 {
		t.Fatalf("the drops were logged %d times: %s", n, said.String())
	}
}

// outputStream is an adapter's stream watching t1 for "FAIL" on a fake core; tell writes what the core sends on the
// watches' channel, 4, once the stream's subscription is answered.
func outputStream(t *testing.T, tell func(peer)) herdr.EventStream {
	var subscribes atomic.Int32
	client := New(fakeCore(`"output_watches"`, func(core peer, frame sent) {
		switch frame.method {
		case "events.subscribe":
			if subscribes.Add(1) == 1 {
				core.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
				return
			}
			core.reply(frame.id, `{"seq":1,"terminals":[],"channel":4}`)
			go tell(core)
		case "doc.get":
			core.reply(frame.id, `{"value":null,"version":0}`)
		}
	}))
	t.Cleanup(func() { client.Close() })
	adapter := NewAdapter(client, AdapterHooks{})
	t.Cleanup(func() { adapter.Close() })
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	stream, err := adapter.Subscribe([]herdr.Subscription{{Type: herdr.EventOutputMatched, PaneID: "t1", Match: &herdr.OutputMatch{Type: "substring", Value: "FAIL"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stream.Close() })
	return stream
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

func TestAReadNeverReachesAnOlderCoreDialledSince(t *testing.T) {
	var dials atomic.Int32
	var reads atomic.Int32
	current := fakeCore(`"output_watches"`, func(core peer, frame sent) {
		switch frame.method {
		case "events.subscribe":
			core.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
		case "doc.get":
			core.reply(frame.id, `{"value":null,"version":0}`)
		}
	})
	older := fakeCore(`"events"`, func(core peer, frame sent) {
		if frame.method == "terminal.read" {
			reads.Add(1)
			core.reply(frame.id, `{"text":"","complete":true,"how":"history"}`)
		}
	})
	client := New(func(ctx context.Context) (net.Conn, error) {
		if dials.Add(1) == 1 {
			return current(ctx)
		}
		return older(ctx)
	})
	defer client.Close()
	adapter := NewAdapter(client, AdapterHooks{})
	defer adapter.Close()
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	client.live.fail(&TransportError{net.ErrClosed}) // the core is rolled back under the mirror
	client.mu.Unlock()
	if _, _, err := adapter.PaneRecent("t1", 100); err == nil {
		t.Fatal("a read of a rolled-back core answered")
	}
	if reads.Load() != 0 {
		t.Fatal("a read reached the older core")
	}
}

func TestAPromptNeverReachesACoreDialledSinceTheOneThatChecksTheAgent(t *testing.T) {
	var dials, sends atomic.Int32
	agent := `{"address":"t1","state":"live","pid":10,"foreground":{"pid":11,"name":"claude","argv":["claude"]},"status":{"state":"idle","app":"claude-code"}}`
	current := fakeCore(`"output_watches","send_foreground"`, func(core peer, frame sent) {
		switch frame.method {
		case "events.subscribe":
			core.reply(frame.id, `{"seq":1,"terminals":[`+agent+`],"channel":3}`)
		case "doc.get":
			core.reply(frame.id, `{"value":null,"version":0}`)
		case "terminal.get":
			core.reply(frame.id, agent)
			core.conn.Close() // the core is rolled back between the look and the send
		case "terminal.send":
			sends.Add(1)
		}
	})
	older := fakeCore(`"output_watches"`, func(core peer, frame sent) {
		if frame.method == "terminal.send" {
			sends.Add(1)
			core.reply(frame.id, `{"status":"idle","turn":0}`)
		}
	})
	client := New(func(ctx context.Context) (net.Conn, error) {
		if dials.Add(1) == 1 {
			return current(ctx)
		}
		return older(ctx)
	})
	defer client.Close()
	adapter := NewAdapter(client, AdapterHooks{})
	defer adapter.Close()
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	if err := adapter.AgentPrompt("t1", "rm -rf build"); err == nil {
		t.Fatal("the prompt went through after the connection that checked was lost")
	}
	if sends.Load() != 0 {
		t.Fatal("a send reached a core dialled after the one that checks the agent")
	}
}

func TestRecentRowsEndAtTheLastTextNotAColouredBlankRow(t *testing.T) {
	vt := "\x1b[0m49\n\x1b[0m50\n\x1b[0m\x1b[31mred\x1b[0m\n\x1b[0m\x1b[44m    \n\x1b[0m\x1b[44m    \n\x1b[0m"
	if got := lastTextRows(vt, 3); got != "49\n50\nred" {
		t.Fatalf("%q", got)
	}
}
