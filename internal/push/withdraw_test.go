package push

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
)

const phone = "https://push.example/phone"

// fleetOfDevices is a service with a phone on Web Push (its pushes logged, none sent) and the Mac app as a socket
// device that takes withdrawals, connected.
func fleetOfDevices(t *testing.T) (*Service, *sentLog, string, <-chan Frame) {
	t.Helper()
	shrinkDelays(t)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := &sentLog{}
	s.send = func(sub *Subscription, p Payload) error {
		if IsSocket(sub.Endpoint) {
			s.Sockets.Deliver(sub.Endpoint, p)
			return nil
		}
		log.mu.Lock()
		log.list = append(log.list, p)
		log.mu.Unlock()
		return nil
	}
	sub := Subscription{Endpoint: phone, Prefs: Prefs{Input: true, Done: "always"}}
	sub.Keys.P256dh, sub.Keys.Auth = "p", "a"
	s.Subscribe(sub, "")
	mac, _ := s.SubscribeSocket("", &Prefs{Input: true, Done: "always"}, "remuda on the Mac")
	frames, detach := s.Sockets.Attach(mac, true)
	t.Cleanup(detach)
	return s, log, mac, frames
}

var apiPane = events.PaneRef{Machine: "local", Pane: "w1:p1"}

func status(from, to string) events.PaneStatus {
	return events.PaneStatus{Pane: apiPane, Workspace: "api", Agent: "claude", From: from, To: to, Worked: time.Second}
}

func nextFrame(t *testing.T, frames <-chan Frame) Frame {
	t.Helper()
	select {
	case f := <-frames:
		return f
	case <-time.After(time.Second):
		t.Fatal("no frame reached the Mac app")
		return Frame{}
	}
}

func noFrame(t *testing.T, frames <-chan Frame) {
	t.Helper()
	select {
	case f := <-frames:
		t.Fatalf("the Mac app heard %+v", f)
	case <-time.After(3 * inputDelay):
	}
}

func TestAQuestionAnsweredAtTheDeskComesDownEverywhere(t *testing.T) {
	s, sent, _, frames := fleetOfDevices(t)
	moved := 0
	var mu sync.Mutex
	s.OnWithdraw = func() { mu.Lock(); moved++; mu.Unlock() }
	before := time.Now()
	s.OnStatus(status("working", "blocked"))
	asked := nextFrame(t, frames)
	if asked.Alert == nil || asked.Alert.Kind != "input" {
		t.Fatalf("the question: %+v", asked)
	}
	tag := asked.Alert.Tag
	parts := strings.Split(tag, "|")
	began, _ := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if len(parts) != 4 || parts[0] != "local" || parts[1] != "w1:p1" || parts[2] != "input" || began < before.UnixMilli() || began > time.Now().UnixMilli() {
		t.Fatalf("the tag %q names no episode", tag)
	}
	if got := sent.get(); len(got) != 1 || got[0].Tag != tag || len(got[0].Withdraw) != 0 {
		t.Fatalf("the phone got %+v", got)
	}

	s.OnStatus(status("blocked", "working"))
	if f := nextFrame(t, frames); f.Alert != nil || len(f.Withdraw) != 1 || f.Withdraw[0] != tag {
		t.Fatalf("the Mac app heard %+v, want the question taken back", f)
	}
	if got := s.Withdrawn(); len(got) != 1 || got[0] != tag {
		t.Fatalf("the roster lists %v", got)
	}
	mu.Lock()
	if moved != 1 {
		t.Fatalf("the roster was told %d times", moved)
	}
	mu.Unlock()
	if owed := s.OwedTo(phone); len(owed) != 1 || owed[0] != tag {
		t.Fatalf("the phone is owed %v", owed)
	}

	s.OnStatus(status("working", "done"))
	time.Sleep(80 * time.Millisecond)
	got := sent.get()
	if len(got) != 2 || got[1].Kind != "done" || len(got[1].Withdraw) != 1 || got[1].Withdraw[0] != tag {
		t.Fatalf("the next push to the phone: %+v, want the finish closing the question first", got)
	}
}

// A pane that asks again raises a newer notification under a tag of its own: taking back the first never takes it down.
func TestAWithdrawalNeverTakesDownANewerEpisode(t *testing.T) {
	s, sent, _, frames := fleetOfDevices(t)
	s.OnStatus(status("working", "blocked"))
	first := nextFrame(t, frames).Alert.Tag
	s.OnStatus(status("blocked", "working"))
	nextFrame(t, frames)
	s.OnStatus(status("working", "blocked"))
	second := nextFrame(t, frames).Alert
	if second.Tag == first {
		t.Fatalf("two episodes share the tag %q", first)
	}
	if got := sent.get(); len(got[1].Withdraw) != 1 || got[1].Withdraw[0] != first {
		t.Fatalf("the second question's push closes %v, want the first alone", got[1].Withdraw)
	}
	if got := s.Withdrawn(); len(got) != 1 || got[0] != first {
		t.Fatalf("the roster lists %v while the second question stands", got)
	}
	if f := s.withdrawals.standing; len(f) != 1 || f[0].tag != second.Tag {
		t.Fatalf("standing %+v", f)
	}
}

func TestAFinishComesDownOnceLookedAtAndIsNeverPushedWhenLookedAtFirst(t *testing.T) {
	s, sent, _, frames := fleetOfDevices(t)
	s.OnStatus(status("working", "done"))
	done := nextFrame(t, frames).Alert
	s.Seen("local", "w1:p1")
	if f := nextFrame(t, frames); len(f.Withdraw) != 1 || f.Withdraw[0] != done.Tag {
		t.Fatalf("after the look the Mac app heard %+v", f)
	}
	if len(sent.get()) != 1 {
		t.Fatalf("the phone got %+v", sent.get())
	}

	var looked atomic.Bool
	s.Looked = func(machine, pane string) bool { return looked.Load() && machine == "local" && pane == "w1:p1" }
	s.OnStatus(status("done", "working"))
	s.OnStatus(status("working", "done"))
	looked.Store(true) // the person had the chat open as it finished
	noFrame(t, frames)
	if len(sent.get()) != 1 {
		t.Fatalf("a finish looked at was pushed: %+v", sent.get())
	}
}

func TestAFinishLeftWhenTheAgentWorksAgainComesDown(t *testing.T) {
	s, _, _, frames := fleetOfDevices(t)
	s.OnStatus(status("working", "done"))
	done := nextFrame(t, frames).Alert
	s.OnStatus(status("done", "working"))
	if f := nextFrame(t, frames); len(f.Withdraw) != 1 || f.Withdraw[0] != done.Tag {
		t.Fatalf("heard %+v", f)
	}
	s.Seen("local", "w1:p1") // nothing stands any more: no second word
	select {
	case f := <-frames:
		t.Fatalf("a second withdrawal: %+v", f)
	default:
	}
}

// A watchdog note stands while the state it is about does; a closed pane takes everything it raised with it.
func TestAWatchdogNoteComesDownWhenItsStateMovesOn(t *testing.T) {
	s, _, _, frames := fleetOfDevices(t)
	s.Notify(Payload{Machine: "local", Pane: "w1:p1", Kind: "stuck", Title: "api looks stuck", Tag: "local|w1:p1|stuck|1800000000"})
	nextFrame(t, frames)
	s.Notify(Payload{Machine: "local", Pane: "w1:p2", Kind: "waiting", Title: "web is still waiting", Tag: "local|w1:p2|waiting|1800000000"})
	nextFrame(t, frames)
	s.OnStatus(status("working", "blocked"))
	if f := nextFrame(t, frames); len(f.Withdraw) != 1 || f.Withdraw[0] != "local|w1:p1|stuck|1800000000" {
		t.Fatalf("heard %+v", f)
	}
	nextFrame(t, frames) // the question itself
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "local", Pane: "w1:p2"}, To: events.Gone})
	if f := nextFrame(t, frames); len(f.Withdraw) != 1 || f.Withdraw[0] != "local|w1:p2|waiting|1800000000" {
		t.Fatalf("heard %+v", f)
	}
}

// A note under a tag that is no episode's — a task's agent asking first — raised again after it was taken back stands
// again: no page and no push closes it.
func TestATagRaisedAgainStandsAgain(t *testing.T) {
	s, _, _, frames := fleetOfDevices(t)
	note := Payload{Machine: "local", Pane: "w1:p1", Kind: "input", Title: "Task #3: the agent asks something first", Tag: "task|3"}
	s.Notify(note)
	nextFrame(t, frames)
	s.OnStatus(status("blocked", "working"))
	nextFrame(t, frames)
	if got := s.Withdrawn(); len(got) != 1 {
		t.Fatalf("withdrawn %v", got)
	}
	s.Notify(note)
	nextFrame(t, frames)
	if got, owed := s.Withdrawn(), s.OwedTo(phone); len(got) != 0 || len(owed) != 0 {
		t.Fatalf("raised again, yet withdrawn %v and owed %v", got, owed)
	}
}

// A device the alert never reached — inside its quiet hours — is owed nothing; a connection that takes no withdrawals
// (an app from before them) hears none and keeps them for its next connection that does.
func TestOnlyTheDevicesAnAlertReachedAreOwedItsWithdrawal(t *testing.T) {
	s, _, mac, frames := fleetOfDevices(t)
	quiet := &Quiet{From: "00:00", To: "23:59", TZ: "UTC"}
	sub := Subscription{Endpoint: phone, Prefs: Prefs{Input: true, Done: "always", Quiet: quiet}}
	sub.Keys.P256dh, sub.Keys.Auth = "p", "a"
	s.Subscribe(sub, "")
	s.OnStatus(status("working", "blocked"))
	nextFrame(t, frames)
	old, detachOld := s.Sockets.Attach(mac, false)
	defer detachOld()
	s.OnStatus(status("blocked", "working"))
	if owed := s.OwedTo(phone); len(owed) != 0 {
		t.Fatalf("the phone in its quiet hours is owed %v", owed)
	}
	select {
	case f := <-old:
		t.Fatalf("a connection that takes no withdrawals heard %+v", f)
	case <-time.After(20 * time.Millisecond):
	}
	if owed := s.OwedTo(mac); len(owed) != 1 {
		t.Fatalf("the Mac app is owed %v, want the question for its next connection", owed)
	}
	s.Unsubscribe(mac)
	if owed := s.OwedTo(mac); len(owed) != 0 {
		t.Fatalf("a device gone is still owed %v", owed)
	}
}

func TestWithdrawalsAreBounded(t *testing.T) {
	w := newWithdrawals()
	now := time.Now()
	for i := 0; i < maxStanding+maxWithdrawn+10; i++ {
		pane := events.PaneRef{Machine: "local", Pane: "p" + string(rune('a'+i%26)) + string(rune('a'+i/26))}
		w.stand(Payload{Machine: pane.Machine, Pane: pane.Pane, Kind: "input", Tag: episodeTag(pane, "input", now.Add(time.Duration(i)))}, []string{phone}, now)
		w.take(pane, func(string) bool { return true }, now)
	}
	if got := w.withdrawn(now); len(got) != maxWithdrawn {
		t.Fatalf("the roster would list %d", len(got))
	}
	if got := w.owedTo(phone, "", now); len(got) != maxOwed {
		t.Fatalf("a push would carry %d", len(got))
	}
	if got := w.withdrawn(now.Add(keepFor)); len(got) != 0 {
		t.Fatalf("a day later the roster still lists %d", len(got))
	}
	if got := w.owedTo(phone, "", now.Add(keepFor)); len(got) != 0 {
		t.Fatalf("a day later a push still carries %d", len(got))
	}
}
