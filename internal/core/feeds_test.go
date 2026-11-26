package core

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/coreapi"
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
	adapter := NewAdapter(client, nil, AdapterHooks{})
	t.Cleanup(func() { adapter.Close() })
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	watching := func(value string) coreapi.EventStream {
		t.Helper()
		stream, err := adapter.Subscribe([]coreapi.Subscription{{Type: coreapi.EventOutputMatched, PaneID: "t1", Match: &coreapi.OutputMatch{Type: "substring", Value: value}}})
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
	heard := func(stream coreapi.EventStream, want string) {
		t.Helper()
		for {
			ev, err := stream.Next()
			if err != nil {
				t.Fatalf("waiting for %q: %v", want, err)
			}
			if ev.Kind == coreapi.EventOutputMatched {
				if ev.MatchedLine != want || ev.PaneID != "t1" {
					t.Fatalf("heard %+v, want %q at 42", ev, want)
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

// Feeds tell their standing rows at once, before the stream they open for is handed back: a stream holds them all.
func TestAStreamHoldsEveryStandingRowItsFeedsTellAsTheyOpen(t *testing.T) {
	const panes, perWatch = 60, 40
	var mu sync.Mutex
	var core peer
	opened := 0
	client := New(fakeCore(`"output_watches"`, func(c peer, frame sent) {
		switch {
		case frame.method == "events.subscribe" && bytes.Contains(frame.payload, []byte(outputOnly)):
			mu.Lock()
			opened++
			n := opened
			core = c
			mu.Unlock()
			channel := uint32(3 + n)
			if n == 2*panes {
				time.Sleep(250 * time.Millisecond) // the last reply late, the rows before it all told
			}
			c.reply(frame.id, fmt.Sprintf(`{"seq":1,"terminals":[],"channel":%d}`, channel))
			for row := 0; row < perWatch; row++ {
				c.write(channel, frameData, fmt.Appendf(nil, `{"jsonrpc":"2.0","method":"event","params":{"seq":2,"type":"output","data":{"watch":"0","line":"FAIL %d","fresh":true}}}`+"\n", row))
			}
		case frame.method == "events.subscribe":
			c.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
		case frame.method == "doc.get":
			c.reply(frame.id, `{"value":null,"version":0}`)
		}
	}))
	t.Cleanup(func() { client.Close() })
	adapter := NewAdapter(client, nil, AdapterHooks{})
	t.Cleanup(func() { adapter.Close() })
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	var subs []coreapi.Subscription
	for pane := 1; pane <= panes; pane++ {
		for _, value := range []string{"FAIL", "AIL"} {
			subs = append(subs, coreapi.Subscription{Type: coreapi.EventOutputMatched, PaneID: fmt.Sprintf("t%d", pane), Match: &coreapi.OutputMatch{Type: "substring", Value: value}, Lines: perWatch})
		}
	}
	stream, err := adapter.Subscribe(subs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stream.Close() })
	mu.Lock()
	core.write(4, frameData, []byte(`{"jsonrpc":"2.0","method":"event","params":{"seq":3,"type":"output","data":{"watch":"0","line":"FAIL live","fresh":false}}}`+"\n"))
	mu.Unlock()
	standing, live := 0, 0
	for standing < panes*2*perWatch || live == 0 {
		ev, err := stream.Next()
		if err != nil {
			t.Fatalf("after %d standing rows and %d live the stream ended: %v", standing, live, err)
		}
		if ev.Kind != coreapi.EventOutputMatched {
			continue
		}
		if ev.Standing != nil && !*ev.Standing {
			live++
		} else {
			standing++
		}
	}
	if standing != panes*2*perWatch || live != 1 {
		t.Fatalf("%d standing rows and %d live, want %d and 1", standing, live, panes*2*perWatch)
	}
}

// A rule edited is a new watch: the stream after it subscribes that one alone, the watches it kept go on telling
// through the feeds that stood, so a row they match while the streams swap reaches the new stream as it appears.
func TestAnEditedRuleLeavesTheOtherWatchesFeedsStanding(t *testing.T) {
	var mu sync.Mutex
	var core peer
	var asked []string
	client := New(fakeCore(`"output_watches"`, func(c peer, frame sent) {
		mu.Lock()
		defer mu.Unlock()
		core = c
		switch {
		case frame.method == "events.subscribe" && bytes.Contains(frame.payload, []byte(outputOnly)):
			for _, value := range []string{"FAIL", "PASS", "DEPLOYED"} {
				if bytes.Contains(frame.payload, []byte(`"value":"`+value+`"`)) {
					asked = append(asked, value)
				}
			}
			c.reply(frame.id, fmt.Sprintf(`{"seq":1,"terminals":[],"channel":%d}`, 3+len(asked)))
		case frame.method == "events.subscribe":
			c.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
		case frame.method == "doc.get":
			c.reply(frame.id, `{"value":null,"version":0}`)
		}
	}))
	t.Cleanup(func() { client.Close() })
	adapter := NewAdapter(client, nil, AdapterHooks{})
	t.Cleanup(func() { adapter.Close() })
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	watching := func(values ...string) coreapi.EventStream {
		t.Helper()
		var subs []coreapi.Subscription
		for _, value := range values {
			subs = append(subs, coreapi.Subscription{Type: coreapi.EventOutputMatched, PaneID: "t1", Match: &coreapi.OutputMatch{Type: "substring", Value: value}})
		}
		stream, err := adapter.Subscribe(subs)
		if err != nil {
			t.Fatal(err)
		}
		return stream
	}
	before := watching("FAIL", "PASS")
	after := watching("FAIL", "DEPLOYED") // PASS's rule edited to look for DEPLOYED
	before.Close()
	defer after.Close()
	mu.Lock()
	got := fmt.Sprint(asked)
	channel := uint32(4)
	if asked[0] != "FAIL" {
		channel = 5
	}
	core.write(channel, frameData, []byte(`{"jsonrpc":"2.0","method":"event","params":{"seq":2,"type":"output","data":{"watch":"0","line":"--- FAIL: TestLogin","fresh":false}}}`+"\n"))
	mu.Unlock()
	if got != "[FAIL PASS DEPLOYED]" && got != "[PASS FAIL DEPLOYED]" {
		t.Fatalf("the core was asked for %s: the kept watch is subscribed once", got)
	}
	for {
		ev, err := after.Next()
		if err != nil {
			t.Fatalf("the kept watch's row never came: %v", err)
		}
		if ev.Kind == coreapi.EventOutputMatched {
			if ev.MatchedLine != "--- FAIL: TestLogin" || ev.Standing == nil || *ev.Standing {
				t.Fatalf("heard %+v", ev)
			}
			return
		}
	}
}

// A core that takes requests and answers none: a stream's watches give up once nothing answered for joinWithin,
// however many rounds they would take, and the adapter's Close ends the wait at once.
func TestWatchesTheCoreNeverAnswersEndAfterOneSilenceOrTheAdaptersClose(t *testing.T) {
	saved := joinWithin
	t.Cleanup(func() { joinWithin = saved })
	silent := func() *Adapter {
		client := New(fakeCore(`"output_watches"`, func(c peer, frame sent) {
			switch {
			case frame.method == "events.subscribe" && bytes.Contains(frame.payload, []byte(outputOnly)):
				// never answered
			case frame.method == "events.subscribe":
				c.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
			case frame.method == "doc.get":
				c.reply(frame.id, `{"value":null,"version":0}`)
			}
		}))
		t.Cleanup(func() { client.Close() })
		adapter := NewAdapter(client, nil, AdapterHooks{})
		if _, err := adapter.SessionSnapshot(); err != nil {
			t.Fatal(err)
		}
		return adapter
	}
	var subs []coreapi.Subscription
	for i := 0; i < 20; i++ { // more than opensAtOnce: three rounds of eight
		subs = append(subs, coreapi.Subscription{Type: coreapi.EventOutputMatched, PaneID: "t1", Match: &coreapi.OutputMatch{Type: "substring", Value: fmt.Sprint("FAIL ", i)}})
	}

	joinWithin = 300 * time.Millisecond
	adapter := silent()
	start := time.Now()
	if _, err := adapter.Subscribe(subs); err == nil {
		t.Fatal("watches the core never answered stand")
	}
	if took := time.Since(start); took > 2*joinWithin {
		t.Fatalf("the watches took %s to give up, past one silence of %s", took, joinWithin)
	}
	adapter.Close()

	joinWithin = time.Minute
	adapter = silent()
	ended := make(chan error, 1)
	go func() {
		_, err := adapter.Subscribe(subs)
		ended <- err
	}()
	time.Sleep(100 * time.Millisecond)
	closed := time.Now()
	adapter.Close()
	select {
	case err := <-ended:
		if err == nil {
			t.Fatal("watches subscribed through a closed adapter")
		}
		if took := time.Since(closed); took > time.Second {
			t.Fatalf("Close ended the wait %s later", took)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not end the wait")
	}
}

// A core far away answers every subscribe, one at a time and slowly: the whole join takes much longer than
// joinWithin, but no answer is ever that late, so the watches all stand.
func TestASlowCoreThatKeepsAnsweringOpensEveryWatch(t *testing.T) {
	saved := joinWithin
	joinWithin = 300 * time.Millisecond
	t.Cleanup(func() { joinWithin = saved })
	opened := 0
	client := New(fakeCore(`"output_watches"`, func(c peer, frame sent) {
		switch {
		case frame.method == "events.subscribe" && bytes.Contains(frame.payload, []byte(outputOnly)):
			time.Sleep(100 * time.Millisecond) // one round trip at a time
			opened++
			c.reply(frame.id, fmt.Sprintf(`{"seq":1,"terminals":[],"channel":%d}`, 3+opened))
		case frame.method == "events.subscribe":
			c.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
		case frame.method == "doc.get":
			c.reply(frame.id, `{"value":null,"version":0}`)
		}
	}))
	t.Cleanup(func() { client.Close() })
	adapter := NewAdapter(client, nil, AdapterHooks{})
	t.Cleanup(func() { adapter.Close() })
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	var subs []coreapi.Subscription
	for i := 0; i < 20; i++ {
		subs = append(subs, coreapi.Subscription{Type: coreapi.EventOutputMatched, PaneID: "t1", Match: &coreapi.OutputMatch{Type: "substring", Value: fmt.Sprint("FAIL ", i)}})
	}
	start := time.Now()
	stream, err := adapter.Subscribe(subs)
	if err != nil {
		t.Fatalf("after %s: %v", time.Since(start), err)
	}
	stream.Close()
	if took := time.Since(start); took < 4*joinWithin {
		t.Fatalf("the join took %s: the test meant it to outlast joinWithin %s several times", took, joinWithin)
	}
}

// A regex the core refuses loses the join at once: the watcher asks again without the person's watches rather than
// waiting for the rest, which the core is slow to answer.
func TestARefusedRegexEndsTheJoinWithoutWaitingForTheRest(t *testing.T) {
	var closes sync.WaitGroup
	client := New(fakeCore(`"output_watches"`, func(c peer, frame sent) {
		switch {
		case frame.method == "events.subscribe" && bytes.Contains(frame.payload, []byte(`"value":"(`)):
			c.write(0, frameData, fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%d,"error":{"code":-32000,"message":"watch \"0\": unclosed group","data":{"kind":"invalid_regex"}}}`, frame.id))
		case frame.method == "events.subscribe" && bytes.Contains(frame.payload, []byte(outputOnly)):
			closes.Add(1)
			go func() {
				defer closes.Done()
				time.Sleep(time.Second) // a far core, slow to answer the rest
				c.reply(frame.id, `{"seq":1,"terminals":[],"channel":40}`)
			}()
		case frame.method == "events.subscribe":
			c.reply(frame.id, `{"seq":1,"terminals":[{"address":"t1","state":"live"}],"channel":3}`)
		case frame.method == "doc.get":
			c.reply(frame.id, `{"value":null,"version":0}`)
		}
	}))
	t.Cleanup(func() { closes.Wait(); client.Close() })
	adapter := NewAdapter(client, nil, AdapterHooks{})
	t.Cleanup(func() { adapter.Close() })
	if _, err := adapter.SessionSnapshot(); err != nil {
		t.Fatal(err)
	}
	subs := []coreapi.Subscription{{Type: coreapi.EventOutputMatched, PaneID: "t1", Match: &coreapi.OutputMatch{Type: "regex", Value: "("}}}
	for i := 1; i < opensAtOnce; i++ { // one round: the refusal and seven slow ones subscribed side by side
		subs = append(subs, coreapi.Subscription{Type: coreapi.EventOutputMatched, PaneID: "t1", Match: &coreapi.OutputMatch{Type: "substring", Value: fmt.Sprint("FAIL ", i)}})
	}
	start := time.Now()
	_, err := adapter.Subscribe(subs)
	if !coreapi.IsCode(err, "invalid_regex") {
		t.Fatalf("the join ended with %v, want invalid_regex", err)
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Fatalf("the refusal was told %s later, after the slow answers", took)
	}
}
