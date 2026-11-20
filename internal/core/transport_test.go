package core_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
)

// severable dials the core and can cut the connection it gave last, as a network or an ssh master would.
type severable struct {
	dial  core.Dialer
	mu    sync.Mutex
	last  net.Conn
	dials int
}

func (s *severable) Dial(ctx context.Context) (net.Conn, error) {
	nc, err := s.dial(ctx)
	s.mu.Lock()
	s.last, s.dials = nc, s.dials+1
	s.mu.Unlock()
	return nc, err
}

func (s *severable) sever() {
	s.mu.Lock()
	s.last.Close()
	s.mu.Unlock()
}

func TestALostConnectionEndsWhatWaitedAndTheNextCallDialsAgain(t *testing.T) {
	ctx := within(t, 30*time.Second)
	running := coretest.Server(t)
	line := &severable{dial: running.Dial}
	client := core.New(line.Dial)
	defer client.Close()

	sleeper, err := client.Run(ctx, core.RunParams{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := client.Subscribe(ctx, core.SubscribeParams{})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Attach(ctx, core.AttachParams{Terminal: sleeper.Address, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	waiting := make(chan error, 1)
	go func() {
		_, err := client.Wait(ctx, sleeper.Address, "never printed", 10)
		waiting <- err
	}()
	time.Sleep(200 * time.Millisecond)
	line.sever()

	if err := <-waiting; !core.IsTransport(err) {
		t.Errorf("the wait in flight: %v", err)
	}
	for {
		_, err := sub.Next(ctx) // what came before the loss first
		if err != nil {
			if !core.IsTransport(err) {
				t.Errorf("the subscription: %v", err)
			}
			break
		}
	}
	if _, err := stream.Read(make([]byte, 64)); !core.IsTransport(err) {
		t.Errorf("the stream: %v", err)
	}
	if _, err := client.Get(ctx, sleeper.Address); err != nil {
		t.Fatalf("the next call: %v", err)
	}
	if line.dials != 2 {
		t.Errorf("dialled %d times", line.dials)
	}
	if _, err := client.Subscribe(ctx, core.SubscribeParams{}); err != nil {
		t.Errorf("subscribing again: %v", err)
	}
}

func TestAClosedClientDialsNoMore(t *testing.T) {
	ctx := within(t, 10*time.Second)
	client := coretest.Server(t).Client(t)
	if _, err := client.List(ctx, false, nil); err != nil {
		t.Fatal(err)
	}
	client.Close()
	if _, err := client.List(ctx, false, nil); !core.IsTransport(err) {
		t.Fatalf("after Close: %v", err)
	}
}
