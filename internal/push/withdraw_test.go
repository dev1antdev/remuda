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

// eventually waits up to a second for what goes out in no set order: the devices are sent to one after another in the
// subscriptions' map order, so the Mac app may hear an alert before the phone's push is logged or the alert stands.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// standingTags are the alerts standing, read under the lock the timers write them under.
func standingTags(s *Service) []string {
	s.withdrawals.mu.Lock()
	defer s.withdrawals.mu.Unlock()
	var tags []string
	for _, alert := range s.withdrawals.standing {
		tags = append(tags, alert.tag)
	}
	return tags
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
	eventually(t, "the question stands for both devices", func() bool { return len(sent.get()) == 1 && len(standingTags(s)) == 1 })
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
	eventually(t, "the second question stands for both devices", func() bool { return len(sent.get()) == 2 && len(standingTags(s)) == 1 })
	if got := sent.get(); len(got[1].Withdraw) != 1 || got[1].Withdraw[0] != first {
		t.Fatalf("the second question's push closes %v, want the first alone", got[1].Withdraw)
	}
	if got := s.Withdrawn(); len(got) != 1 || got[0] != first {
		t.Fatalf("the roster lists %v while the second question stands", got)
	}
	if f := standingTags(s); len(f) != 1 || f[0] != second.Tag {
		t.Fatalf("standing %+v", f)
	}
}

func TestAFinishComesDownOnceLookedAtAndIsNeverPushedWhenLookedAtFirst(t *testing.T) {
	s, sent, _, frames := fleetOfDevices(t)
	s.OnStatus(status("working", "done"))
	done := nextFrame(t, frames).Alert
	eventually(t, "the finish stands for both devices", func() bool { return len(sent.get()) == 1 && len(standingTags(s)) == 1 })
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
		w.stand(Payload{Machine: pane.Machine, Pane: pane.Pane, Kind: "input", Tag: episodeTag(pane, "input", now.Add(time.Duration(i)))}, []string{phone}, w.now(), now)
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
	for i := 0; i < 3; i++ {
		w.moved(events.PaneRef{Machine: "local", Pane: "w1:p" + strconv.Itoa(i)}, "working", now)
	}
	w.looked(apiPane, now)
	if len(w.marks) != 3 {
		t.Fatalf("marks %v", w.marks)
	}
	w.withdrawn(now.Add(keepFor))
	if len(w.marks) != 0 {
		t.Fatalf("a day later the panes' marks are still kept: %v", w.marks)
	}
}

// A task's pushes share one tag: its agent asking first (a question's kind, taken back once the pane stops asking) and
// then its finish or its pull request (kinds no status takes back). The finish shows under the tag the question was
// taken back by, so neither a page nor the phone's next push may close it.
func TestAnotherKindRaisedUnderATagTakenBackIsNotClosed(t *testing.T) {
	s, sent, _, frames := fleetOfDevices(t)
	s.Notify(Payload{Machine: "local", Pane: "w1:p1", Kind: "input", Title: "Task #3: the agent asks something first", Tag: "task|3"})
	nextFrame(t, frames)
	s.OnStatus(status("blocked", "working"))
	if f := nextFrame(t, frames); len(f.Withdraw) != 1 || f.Withdraw[0] != "task|3" {
		t.Fatalf("the question was not taken back: %+v", f)
	}
	s.Notify(Payload{Machine: "local", Pane: "w1:p1", Kind: "task", Title: "Task #3: the agent answered → Testing", Tag: "task|3"})
	if f := nextFrame(t, frames); f.Alert == nil || f.Alert.Kind != "task" {
		t.Fatalf("the finish: %+v", f)
	}
	if got := s.Withdrawn(); len(got) != 0 {
		t.Fatalf("every page would close the task's finish: withdrawn %v", got)
	}
	s.OnStatus(status("working", "done"))
	time.Sleep(80 * time.Millisecond)
	got := sent.get()
	if last := got[len(got)-1]; last.Kind != "done" || len(last.Withdraw) != 0 {
		t.Fatalf("the phone's next push closes %v, the task's finish among them", last.Withdraw)
	}
}

// A question answered, or a finish looked at, while its push is still going out device by device — a slow push service,
// a second phone — comes down all the same: the move lands before the alert is noted as standing.
func TestAMoveWhileThePushGoesOutStillTakesItBack(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raise     events.PaneStatus
		meanwhile func(s *Service)
	}{
		{"answered", status("working", "blocked"), func(s *Service) { s.OnStatus(status("blocked", "working")) }},
		{"looked at", status("working", "done"), func(s *Service) { s.Seen("local", "w1:p1") }},
		{"closed", status("working", "done"), func(s *Service) { s.OnStatus(events.PaneStatus{Pane: apiPane, To: events.Gone}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, mac, _ := fleetOfDevices(t)
			s.Unsubscribe(mac) // the phone alone, whose push the move overtakes
			var once sync.Once
			pushed := make(chan string, 1)
			s.send = func(sub *Subscription, p Payload) error {
				once.Do(func() {
					tc.meanwhile(s)
					pushed <- p.Tag
				})
				return nil
			}
			s.OnStatus(tc.raise)
			var tag string
			select {
			case tag = <-pushed:
			case <-time.After(time.Second):
				t.Fatal("nothing was pushed")
			}
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) && len(s.Withdrawn()) == 0 {
				time.Sleep(5 * time.Millisecond)
			}
			if got := s.Withdrawn(); len(got) != 1 || got[0] != tag {
				t.Fatalf("withdrawn %v, want the alert overtaken on its way out (%s)", got, tag)
			}
			if owed := s.OwedTo(phone); len(owed) != 1 || owed[0] != tag {
				t.Fatalf("the phone is owed %v", owed)
			}
			if len(s.withdrawals.standing) != 0 {
				t.Fatalf("still standing: %+v", s.withdrawals.standing)
			}
		})
	}
}
