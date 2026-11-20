package core_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/core"
	"github.com/dev1antdev/remuda/internal/core/coretest"
)

func within(t *testing.T, d time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestHelloNamesTheGenerationAndFeatures(t *testing.T) {
	client := coretest.Server(t).Client(t)
	hello, err := client.Hello(within(t, 10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if hello.Generation != core.Generation || !hello.Framed {
		t.Fatalf("hello = %+v", hello)
	}
	for _, feature := range []string{"attach", "events", "keep_size", "output_watches"} {
		if !hello.Has(feature) {
			t.Errorf("no %s in %v", feature, hello.Features)
		}
	}
}

func TestCallsGoSideBySide(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	sleeper, err := client.Run(ctx, core.RunParams{Argv: []string{"sleep", "60"}})
	if err != nil {
		t.Fatal(err)
	}
	slow := make(chan error, 1)
	go func() {
		waited, err := client.Wait(ctx, sleeper.Address, "never printed", 2)
		if err == nil && waited.Reason != "timeout" {
			t.Errorf("wait ended %+v", waited)
		}
		slow <- err
	}()
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	var quick sync.WaitGroup
	for range 20 {
		quick.Go(func() {
			if _, err := client.List(ctx, false, nil); err != nil {
				t.Error(err)
			}
		})
	}
	quick.Wait()
	if took := time.Since(start); took > time.Second {
		t.Errorf("20 lists took %s behind a slow wait", took)
	}
	select {
	case <-slow:
		t.Fatal("the wait ended before the lists did")
	default:
	}
	if err := <-slow; err != nil {
		t.Fatal(err)
	}
}

func TestARefusalHasItsKind(t *testing.T) {
	client := coretest.Server(t).Client(t)
	_, err := client.Get(within(t, 10*time.Second), "t999")
	if !core.IsKind(err, "not_found") || core.IsTransport(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestDocumentsKeepTheirVersion(t *testing.T) {
	ctx := within(t, 10*time.Second)
	client := coretest.Server(t).Client(t)
	version, err := client.DocSet(ctx, "remuda", map[string]any{"spaces": []string{"w1"}}, 0)
	if err != nil || version == 0 {
		t.Fatalf("set: %d %v", version, err)
	}
	doc, err := client.DocGet(ctx, "remuda")
	if err != nil || doc.Version != version || !strings.Contains(string(doc.Value), `"w1"`) {
		t.Fatalf("get: %+v %v", doc, err)
	}
	if _, err := client.DocSet(ctx, "remuda", nil, 0); !core.IsKind(err, "conflict") {
		t.Fatalf("a stale write: %v", err)
	}
}

// readUntil reads a stream until what it handed out holds want, answering everything read.
func readUntil(t *testing.T, stream *core.Stream, want string) []byte {
	t.Helper()
	var got []byte
	chunk := make([]byte, 8192)
	deadline := time.AfterFunc(20*time.Second, func() { stream.Close() })
	defer deadline.Stop()
	for !bytes.Contains(got, []byte(want)) {
		n, err := stream.Read(chunk)
		got = append(got, chunk[:n]...)
		if err != nil {
			t.Fatalf("waiting for %q: %v after %d bytes, ending %q", want, err, len(got), tail(got))
		}
	}
	return got
}

func tail(b []byte) []byte {
	return b[max(0, len(b)-200):]
}

func TestAnAttachEchoesAndEndsWithTheProgram(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	reader, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", `read line; echo "got:$line"; exit 3`}})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Attach(ctx, core.AttachParams{Terminal: reader.Address, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if stream.Terminal.Address != reader.Address {
		t.Errorf("attached to %q", stream.Terminal.Address)
	}
	if _, err := stream.Write([]byte("hello\r")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, stream, "got:hello")
	for {
		if _, err := stream.Read(make([]byte, 4096)); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
	}
	ended := stream.Ended()
	if ended == nil || ended.Reason != "exited" || ended.ExitCode == nil || *ended.ExitCode != 3 {
		t.Fatalf("ended %+v", ended)
	}
}

func TestAStreamNobodyReadsHoldsNothingElseUp(t *testing.T) {
	ctx := within(t, 60*time.Second)
	client := coretest.Server(t).Client(t)
	noisy, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c",
		`read go; i=0; while [ $i -lt 40000 ]; do echo "line-$i"; i=$((i+1)); done; echo "PRINTED-ALL"; sleep 60`}})
	if err != nil {
		t.Fatal(err)
	}
	stalled, err := client.Attach(ctx, core.AttachParams{Terminal: noisy.Address, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stalled.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second) // past the first window: the core now waits for credit

	echo, err := client.Run(ctx, core.RunParams{Argv: []string{"cat"}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := client.Attach(ctx, core.AttachParams{Terminal: echo.Address, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	other.Write([]byte("ping\r"))
	readUntil(t, other, "ping")
	if _, err := client.List(ctx, false, nil); err != nil {
		t.Fatal(err)
	}

	got := readUntil(t, stalled, "PRINTED-ALL")
	if len(got) < 300_000 {
		t.Fatalf("read %d bytes: the stream stopped at its first window", len(got))
	}
}

func TestEventsFollowTheTerminals(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	sub, err := client.Subscribe(ctx, core.SubscribeParams{})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	started, err := client.Run(ctx, core.RunParams{Argv: []string{"sleep", "60"}, Labels: map[string]string{"remuda.space": "w1"}})
	if err != nil {
		t.Fatal(err)
	}
	created := next(t, sub, func(n core.Notice) bool { return n.Event.Type == "created" })
	if created.Event.Terminal != started.Address || created.Seq <= sub.Seq {
		t.Fatalf("created %+v after seq %d", created, sub.Seq)
	}
	if _, err := client.Label(ctx, started.Address, map[string]string{"remuda.tab": "t1"}); err != nil {
		t.Fatal(err)
	}
	labelled := next(t, sub, func(n core.Notice) bool { return n.Event.Type == "labels" })
	if !strings.Contains(string(labelled.Event.Data), `"remuda.tab":"t1"`) {
		t.Fatalf("labels %s", labelled.Event.Data)
	}
	if _, err := client.Kill(ctx, started.Address); err != nil {
		t.Fatal(err)
	}
	next(t, sub, func(n core.Notice) bool { return n.Event.Type == "finished" && n.Event.Terminal == started.Address })
	next(t, sub, func(n core.Notice) bool { return n.Kind == core.NoticeHeartbeat })
}

func TestAnOutputWatchTellsTheRow(t *testing.T) {
	ctx := within(t, 30*time.Second)
	client := coretest.Server(t).Client(t)
	builder, err := client.Run(ctx, core.RunParams{Argv: []string{"sh", "-c", `sleep 1; echo "BUILD FAILED in 3s"; sleep 60`}})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := client.Subscribe(ctx, core.SubscribeParams{Terminal: builder.Address, Output: []core.OutputWatch{
		{ID: "build", Terminal: builder.Address, Match: core.Match{Type: "regex", Value: `BUILD FAILED in \d+s`}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	told := next(t, sub, func(n core.Notice) bool { return n.Event.Type == "output" })
	if !strings.Contains(string(told.Event.Data), `"watch":"build"`) || !strings.Contains(string(told.Event.Data), "BUILD FAILED in 3s") {
		t.Fatalf("output %s", told.Event.Data)
	}
}

func next(t *testing.T, sub *core.Subscription, want func(core.Notice) bool) core.Notice {
	t.Helper()
	ctx := within(t, 10*time.Second)
	for {
		notice, err := sub.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want(notice) {
			return notice
		}
	}
}
