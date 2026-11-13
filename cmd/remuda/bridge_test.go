package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTheBridgeCarriesARequestAndItsAnswer(t *testing.T) {
	ours, herdrs := net.Pipe()
	go func() {
		line, _ := bufio.NewReader(herdrs).ReadString('\n')
		herdrs.Write([]byte(`{"id":"x","result":{"type":"pong","echo":` + strings.TrimSpace(line) + "}}\n"))
		herdrs.Close() // herdr answers once and closes
	}()
	in, keepOpen := io.Pipe()
	defer keepOpen.Close()
	go keepOpen.Write([]byte(`{"id":"x","method":"ping"}` + "\n"))
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- bridge(ours, in, &out) }()
	select {
	case err := <-done:
		if err != nil || !strings.Contains(out.String(), `"pong"`) || !strings.Contains(out.String(), `"method":"ping"`) {
			t.Fatalf("the bridge gave %q, %v", out.String(), err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the bridge did not end when herdr closed")
	}
}

func TestTheBridgeEndsWhenNobodyReadsAnyMore(t *testing.T) {
	ours, herdrs := net.Pipe()
	defer herdrs.Close()
	go io.Copy(io.Discard, herdrs) // a stream that never answers, as an event subscription waits
	done := make(chan error, 1)
	go func() {
		done <- bridge(ours, strings.NewReader(`{"id":"s","method":"events.subscribe"}`+"\n"), io.Discard)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the bridge outlived its reader")
	}
}
