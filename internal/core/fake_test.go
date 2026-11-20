package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// peer is the core's end of a pipe: the shapes a real core rarely shows on demand (events before their reply, a line
// cut across frames, missed, a reply that loses a race, a core that stops reading).
type peer struct {
	conn net.Conn
}

func (p peer) write(channel uint32, kind byte, payload []byte) {
	frame := make([]byte, headerSize+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], channel)
	frame[4] = kind
	binary.LittleEndian.PutUint32(frame[5:9], uint32(len(payload)))
	copy(frame[headerSize:], payload)
	p.conn.Write(frame)
}

func (p peer) reply(id uint64, result string) {
	p.write(0, frameData, fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%d,"result":%s}`, id, result))
}

// sent is a frame the client sent, its request's id and method when it is one.
type sent struct {
	channel uint32
	kind    byte
	payload []byte
	id      uint64
	method  string
}

// fakeCore answers hello with features and hands every frame after it to handle, on the reading goroutine; a nil
// handle reads nothing more, a core that stopped reading.
func fakeCore(features string, handle func(peer, sent)) Dialer {
	return func(ctx context.Context) (net.Conn, error) {
		ours, theirs := net.Pipe()
		go func() {
			defer theirs.Close()
			reader := bufio.NewReader(theirs)
			if _, err := reader.ReadBytes('\n'); err != nil {
				return
			}
			fmt.Fprintf(theirs, `{"jsonrpc":"2.0","id":0,"result":{"generation":1,"framed":true,"features":[%s]}}`+"\n", features)
			if handle == nil {
				select {} // a core that stopped reading, for the rest of the test binary
			}
			var header [headerSize]byte
			for {
				if _, err := io.ReadFull(reader, header[:]); err != nil {
					return
				}
				frame := sent{channel: binary.LittleEndian.Uint32(header[0:4]), kind: header[4]}
				frame.payload = make([]byte, binary.LittleEndian.Uint32(header[5:9]))
				if _, err := io.ReadFull(reader, frame.payload); err != nil {
					return
				}
				if frame.channel == 0 {
					var request struct {
						ID     uint64 `json:"id"`
						Method string `json:"method"`
					}
					json.Unmarshal(frame.payload, &request)
					frame.id, frame.method = request.ID, request.Method
				}
				handle(peer{theirs}, frame)
			}
		}()
		return ours, nil
	}
}

func within(t *testing.T, d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func event(seq uint64, kind string) []byte {
	return fmt.Appendf(nil, `{"jsonrpc":"2.0","method":"event","params":{"seq":%d,"type":%q,"terminal":"t1","data":{}}}`+"\n", seq, kind)
}

func TestEventsKeepTheirOrderAcrossTheReplyAndMissedSnapshots(t *testing.T) {
	ctx := within(t, 10*time.Second)
	client := New(fakeCore(`"events"`, func(core peer, frame sent) {
		if frame.method != "events.subscribe" {
			return
		}
		go func() {
			core.write(5, frameData, event(11, "created")) // the channel is open before the reply goes out
			core.reply(frame.id, `{"seq":10,"terminals":[],"channel":5}`)
			cut := event(12, "title")
			core.write(5, frameData, cut[:20])
			core.write(5, frameData, cut[20:])
			core.write(5, frameData, []byte(`{"jsonrpc":"2.0","method":"missed","params":{"seq":20,"terminals":[{"address":"t1","state":"live"}]}}`+"\n"))
			core.write(5, frameData, event(19, "status"))
			core.write(5, frameData, append(event(21, "status"), []byte(`{"jsonrpc":"2.0","method":"heartbeat","params":{"seq":21}}`+"\n")...))
		}()
	}))
	defer client.Close()
	sub, err := client.Subscribe(ctx, SubscribeParams{})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Seq != 10 {
		t.Fatalf("snapshot seq %d", sub.Seq)
	}
	for _, want := range []string{"event 11 created", "event 12 title", "missed 20 t1", "event 21 status", "heartbeat 21 "} {
		notice, err := sub.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := fmt.Sprintf("%s %d %s", notice.Kind, notice.Seq, notice.Event.Type)
		if notice.Kind == NoticeMissed && len(notice.Terminals) == 1 {
			got = fmt.Sprintf("%s %d %s", notice.Kind, notice.Seq, notice.Terminals[0].Address)
		}
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestACallWithoutTheFeatureIsRefusedHere(t *testing.T) {
	ctx := within(t, 10*time.Second)
	client := New(fakeCore(`"events"`, func(peer, sent) {}))
	defer client.Close()
	_, err := client.Subscribe(ctx, SubscribeParams{Output: []OutputWatch{{ID: "w", Terminal: "t1", Match: Match{Type: "substring", Value: "x"}}}})
	if !IsKind(err, "needs_update") {
		t.Fatalf("output watches: %v", err)
	}
	if _, err := client.Attach(ctx, AttachParams{Terminal: "t1", KeepSize: true}); !IsKind(err, "needs_update") {
		t.Fatalf("keep_size: %v", err)
	}
}

func TestACallWhoseContextEndedSendsNothing(t *testing.T) {
	methods := make(chan string, 8)
	client := New(fakeCore("", func(core peer, frame sent) {
		methods <- frame.method
		go core.reply(frame.id, `{"terminals":[]}`)
	}))
	defer client.Close()
	if _, err := client.List(within(t, 5*time.Second), false, nil); err != nil {
		t.Fatal(err)
	}
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Kill(ended, "t1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("kill: %v", err)
	}
	if _, err := client.List(within(t, 5*time.Second), false, nil); err != nil {
		t.Fatal(err)
	}
	close(methods)
	for method := range methods {
		if method != "terminal.list" {
			t.Errorf("the core heard %s", method)
		}
	}
}

func TestACoreThatStopsReadingEndsTheConnectionNotTheCallsBehindIt(t *testing.T) {
	writeWithin = 400 * time.Millisecond
	t.Cleanup(func() { writeWithin = 30 * time.Second })
	client := New(fakeCore("", nil))
	defer client.Close()
	if _, err := client.Hello(within(t, 5*time.Second)); err != nil {
		t.Fatal(err)
	}
	stalled := make(chan error, 1)
	go func() { stalled <- client.Call(context.Background(), "terminal.list", nil, nil) }()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	err := client.Call(within(t, 100*time.Millisecond), "terminal.list", nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 300*time.Millisecond {
		t.Fatalf("a call behind a stalled write: %v after %s", err, time.Since(start))
	}
	select {
	case err := <-stalled:
		if !IsTransport(err) {
			t.Fatalf("the stalled write: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stalled write never ended")
	}
}

func TestAnAttachWhoseCallerGaveUpIsClosed(t *testing.T) {
	closed := make(chan uint32, 1)
	client := New(fakeCore(`"attach"`, func(core peer, frame sent) {
		switch {
		case frame.method == "terminal.attach":
			go func() {
				time.Sleep(150 * time.Millisecond)
				core.reply(frame.id, `{"channel":7,"terminal":{"address":"t1"}}`)
			}()
		case frame.kind == frameClose:
			closed <- frame.channel
		}
	}))
	defer client.Close()
	if _, err := client.Attach(within(t, 30*time.Millisecond), AttachParams{Terminal: "t1"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("attach: %v", err)
	}
	select {
	case channel := <-closed:
		if channel != 7 {
			t.Fatalf("closed channel %d", channel)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the channel the late reply opened was never closed")
	}
}

func TestAnAnswerThatWonTheRaceAgainstItsContextIsKept(t *testing.T) {
	client := New(fakeCore(`"attach"`, func(core peer, frame sent) {
		if frame.method == "terminal.attach" {
			go core.reply(frame.id, `{"channel":7,"terminal":{"address":"t1"}}`)
		}
	}))
	defer client.Close()
	cn, err := client.connection(within(t, 5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for range 50 {
		ctx, cancel := context.WithCancel(context.Background())
		opened := false
		bind := func(json.RawMessage) (func(), error) {
			opened = true
			cancel() // the caller's context ends while the reader holds the answer
			return nil, nil
		}
		if err := cn.call(ctx, "terminal.attach", AttachParams{Terminal: "t1"}, nil, bind); err != nil || !opened {
			t.Fatalf("an answer taken before the cancel: %v (opened %v)", err, opened)
		}
	}
}
