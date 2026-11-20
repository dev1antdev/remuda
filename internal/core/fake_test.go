package core_test

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
)

// fakeCore answers hello and then hands every request to answer, on a pipe: the shapes a real core rarely shows on
// demand (events before their subscription's reply, a line cut across frames, missed).
func fakeCore(t *testing.T, answer func(write func(channel uint32, kind byte, payload []byte), id uint64, method string)) core.Dialer {
	return func(ctx context.Context) (net.Conn, error) {
		ours, theirs := net.Pipe()
		go func() {
			defer theirs.Close()
			reader := bufio.NewReader(theirs)
			if _, err := reader.ReadBytes('\n'); err != nil {
				return
			}
			fmt.Fprintln(theirs, `{"jsonrpc":"2.0","id":0,"result":{"generation":1,"framed":true,"features":["events"]}}`)
			write := func(channel uint32, kind byte, payload []byte) {
				frame := make([]byte, 9+len(payload))
				binary.LittleEndian.PutUint32(frame[0:4], channel)
				frame[4] = kind
				binary.LittleEndian.PutUint32(frame[5:9], uint32(len(payload)))
				copy(frame[9:], payload)
				theirs.Write(frame)
			}
			var header [9]byte
			for {
				if _, err := io.ReadFull(reader, header[:]); err != nil {
					return
				}
				payload := make([]byte, binary.LittleEndian.Uint32(header[5:9]))
				if _, err := io.ReadFull(reader, payload); err != nil {
					return
				}
				var request struct {
					ID     uint64 `json:"id"`
					Method string `json:"method"`
				}
				if binary.LittleEndian.Uint32(header[0:4]) == 0 && json.Unmarshal(payload, &request) == nil {
					go answer(write, request.ID, request.Method)
				}
			}
		}()
		return ours, nil
	}
}

func event(seq uint64, kind string) []byte {
	return fmt.Appendf(nil, `{"jsonrpc":"2.0","method":"event","params":{"seq":%d,"type":%q,"terminal":"t1","data":{}}}`+"\n", seq, kind)
}

func TestEventsKeepTheirOrderAcrossTheReplyAndMissedSnapshots(t *testing.T) {
	ctx := within(t, 10*time.Second)
	client := core.New(fakeCore(t, func(write func(uint32, byte, []byte), id uint64, method string) {
		write(5, 0, event(11, "created")) // the channel is open before the reply goes out
		write(0, 0, fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%d,"result":{"seq":10,"terminals":[],"channel":5}}`, id))
		cut := event(12, "title")
		write(5, 0, cut[:20])
		write(5, 0, cut[20:])
		write(5, 0, []byte(`{"jsonrpc":"2.0","method":"missed","params":{"seq":20,"terminals":[{"address":"t1","state":"live"}]}}`+"\n"))
		write(5, 0, event(19, "status"))
		write(5, 0, append(event(21, "status"), []byte(`{"jsonrpc":"2.0","method":"heartbeat","params":{"seq":21}}`+"\n")...))
	}))
	defer client.Close()
	sub, err := client.Subscribe(ctx, core.SubscribeParams{})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Seq != 10 {
		t.Fatalf("snapshot seq %d", sub.Seq)
	}
	want := []string{"event 11 created", "event 12 title", "missed 20 t1", "event 21 status", "heartbeat 21"}
	for _, expected := range want {
		notice, err := sub.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		got := fmt.Sprintf("%s %d %s", notice.Kind, notice.Seq, notice.Event.Type)
		if notice.Kind == core.NoticeMissed && len(notice.Terminals) == 1 {
			got = fmt.Sprintf("%s %d %s", notice.Kind, notice.Seq, notice.Terminals[0].Address)
		}
		if notice.Kind == core.NoticeHeartbeat {
			got = fmt.Sprintf("%s %d", notice.Kind, notice.Seq)
		}
		if got != expected {
			t.Fatalf("got %q, want %q", got, expected)
		}
	}
}

func TestACallWithoutTheFeatureIsRefusedHere(t *testing.T) {
	ctx := within(t, 10*time.Second)
	client := core.New(fakeCore(t, func(func(uint32, byte, []byte), uint64, string) {}))
	defer client.Close()
	_, err := client.Subscribe(ctx, core.SubscribeParams{Output: []core.OutputWatch{{ID: "w", Terminal: "t1", Match: core.Match{Type: "substring", Value: "x"}}}})
	if !core.IsKind(err, "needs_update") {
		t.Fatalf("output watches: %v", err)
	}
	if _, err := client.Attach(ctx, core.AttachParams{Terminal: "t1", KeepSize: true}); !core.IsKind(err, "needs_update") {
		t.Fatalf("keep_size: %v", err)
	}
}
