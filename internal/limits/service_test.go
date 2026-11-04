package limits

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

type pushes struct {
	mu   sync.Mutex
	sent []push.Payload
}

func (p *pushes) notify(x push.Payload) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, x)
}

func (p *pushes) titles() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, x := range p.sent {
		out = append(out, x.Title)
	}
	return out
}

// reading is a machine's answer carrying Claude's 5-hour window at the use given
func reading(claude5h float64) string {
	return script.RS + "\n" + `{"at":"2026-11-04T10:05:00Z","rate_limits":{"five_hour":{"used_percentage":` + strconv.FormatFloat(claude5h, 'f', -1, 64) + `,"resets_at":1793801000}}}` + "\n" + script.RS + "\n\n"
}

func TestWarnsOncePerWindowAndLevel(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1793790000, 0)
	var got pushes
	answer := reading(50)
	host := &shell.Fake{Name: "gog", Remote: true, Answer: func(string) (string, error) { return answer, nil }}
	s := New(dir)
	s.now = func() time.Time { return now }
	s.Notify = got.notify

	s.Read(host, "GOG")
	if len(got.titles()) != 0 {
		t.Fatalf("warned at 50 %%: %v", got.titles())
	}
	answer = reading(82)
	s.Read(host, "GOG")
	s.Read(host, "GOG")
	other := &shell.Fake{Name: "roofr", Remote: true, Answer: func(string) (string, error) { return answer, nil }}
	s.Read(other, "Roofr") // the same account read on another machine
	if titles := got.titles(); len(titles) != 1 || titles[0] != "Claude's 5-hour limit at 82 %" {
		t.Fatalf("at 82 %%: %v", titles)
	}
	answer = reading(100)
	s.Read(host, "GOG")
	if titles := got.titles(); len(titles) != 2 || titles[1] != "Claude's 5-hour limit reached" {
		t.Fatalf("at the limit: %v", titles)
	}

	again := New(dir) // a restart remembers what was said
	again.now = s.now
	again.Notify = got.notify
	again.Read(host, "GOG")
	if len(got.titles()) != 2 {
		t.Fatalf("a restart warned again: %v", got.titles())
	}
	if r := again.Of("gog"); len(r.Windows) != 1 || r.Windows[0].Used != 100 {
		t.Fatalf("reading kept: %+v", r)
	}
	if p := got.sent[0]; p.Kind != "limits" || p.Panel != "usage" || !strings.Contains(p.Body, "read on GOG") {
		t.Fatalf("payload: %+v", p)
	}
}

func TestAWarningIsForgottenOnceItsWindowResets(t *testing.T) {
	s := New(t.TempDir())
	now := time.Unix(1793790000, 0)
	s.now = func() time.Time { return now }
	s.kept.Alerted["claude|5h|1"] = 80 // reset long ago
	s.warn("GOG", Reading{})
	if len(s.kept.Alerted) != 0 {
		t.Fatalf("kept: %v", s.kept.Alerted)
	}
}

func TestLevel(t *testing.T) {
	for used, want := range map[float64]int{0: 0, 79.9: 0, 80: 80, 99.9: 80, 100: 100, 130: 100} {
		if got := Level(used); got != want {
			t.Fatalf("Level(%v) = %d, want %d", used, got, want)
		}
	}
}

func TestAHitIsNotedAndClearedByWork(t *testing.T) {
	s := New(t.TempDir())
	hitAt := time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)
	checked := make(chan string, 4)
	s.Check = func(machine, pane string) (Hit, bool) {
		checked <- machine + "|" + pane
		return Hit{At: hitAt, Text: "You've reached your Fable limit."}, true
	}
	changed := make(chan struct{}, 8)
	s.Changed = func() { changed <- struct{}{} }
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "gog", Pane: "w1:p1"}, Kind: "claude", From: herdr.StatusWorking, To: herdr.StatusIdle})
	<-checked
	<-changed
	if p := s.PaneOf("gog", "w1:p1"); p == nil || p.HitAt == nil || !p.HitAt.Equal(hitAt) || p.Text == "" {
		t.Fatalf("hit: %+v", p)
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "gog", Pane: "w1:p1"}, Kind: "claude", From: herdr.StatusIdle, To: herdr.StatusWorking})
	if p := s.PaneOf("gog", "w1:p1"); p != nil {
		t.Fatalf("still noted after work began: %+v", p)
	}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "gog", Pane: "w1:p2"}, Kind: "", From: herdr.StatusWorking, To: herdr.StatusIdle})
	select {
	case key := <-checked:
		t.Fatalf("a shell was checked: %s", key)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestObserveChecksAnAgentAtRestOnce(t *testing.T) {
	s := New(t.TempDir())
	checked := make(chan string, 4)
	s.Check = func(machine, pane string) (Hit, bool) { checked <- pane; return Hit{}, false }
	snap := &herdr.Snapshot{Panes: []herdr.PaneInfo{{PaneID: "p1", Agent: "claude", AgentStatus: herdr.StatusIdle}, {PaneID: "p2", Agent: "claude", AgentStatus: herdr.StatusWorking}, {PaneID: "p3", Agent: "codex", AgentStatus: herdr.StatusIdle}}}
	s.Observe("gog", snap)
	s.Observe("gog", snap)
	if got := <-checked; got != "p1" {
		t.Fatalf("checked %s", got)
	}
	select {
	case got := <-checked:
		t.Fatalf("checked again: %s", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestABookingIsSaidWhenDueAndOnlyThen(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	place := Place{Agent: "claude", Session: "s1", Status: herdr.StatusIdle, Workspace: "api"}
	var said []string
	var got pushes
	dir := t.TempDir()
	s := New(dir)
	s.now = func() time.Time { return now }
	s.Notify = got.notify
	s.Pane = func(machine, pane string) (Place, bool) { return place, pane == "p1" }
	s.Submit = func(machine, pane, text string) error { said = append(said, pane+":"+text); return nil }

	if err := s.Book("gog", "p1", now.Add(-time.Minute), ""); err == nil {
		t.Fatal("a moment past was booked")
	}
	if err := s.Book("gog", "p1", now.Add(9*24*time.Hour), ""); err == nil {
		t.Fatal("a booking past a week was kept")
	}
	if err := s.Book("gog", "nope", now.Add(time.Hour), ""); err == nil {
		t.Fatal("a pane that is not there was booked")
	}
	if err := s.Book("gog", "p1", now.Add(time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	if p := s.PaneOf("gog", "p1"); p == nil || p.ContinueAt == nil || !p.ContinueAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("booking: %+v", p)
	}
	s.Tick()
	if len(said) != 0 {
		t.Fatalf("said before due: %v", said)
	}
	if kept := New(dir); len(kept.kept.Bookings) != 1 {
		t.Fatalf("a restart lost the booking: %+v", kept.kept.Bookings)
	}
	now = now.Add(time.Hour)
	s.Tick()
	if len(said) != 1 || said[0] != "p1:continue" {
		t.Fatalf("said: %v", said)
	}
	if titles := got.titles(); len(titles) != 1 || titles[0] != "Continued api after the limit reset" {
		t.Fatalf("pushes: %v", titles)
	}
	s.Tick()
	if len(said) != 1 || s.PaneOf("gog", "p1") != nil {
		t.Fatalf("said twice, or kept: %v", said)
	}
}

func TestABookingStandsDownForAnotherConversationAQuestionOrWork(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name   string
		place  Place
		gone   bool
		pushed string
	}{
		{"working again", Place{Session: "s1", Status: herdr.StatusWorking, Workspace: "api"}, false, ""},
		{"a question", Place{Session: "s1", Status: herdr.StatusBlocked, Workspace: "api"}, false, "Nothing continued in api"},
		{"another conversation", Place{Session: "s2", Status: herdr.StatusIdle, Workspace: "api"}, false, "Nothing continued in api"},
		{"closed", Place{}, true, "Nothing continued after the limit reset"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got pushes
			submitted := false
			s := New(t.TempDir())
			s.now = func() time.Time { return now }
			s.Notify = got.notify
			s.Submit = func(string, string, string) error { submitted = true; return nil }
			s.kept.Bookings["gog|p1"] = Booking{Machine: "gog", Pane: "p1", Session: "s1", At: now, Text: "continue"}
			s.Pane = func(string, string) (Place, bool) { return c.place, !c.gone }
			s.Tick()
			if submitted {
				t.Fatal("said it")
			}
			titles := got.titles()
			if c.pushed == "" && len(titles) != 0 || c.pushed != "" && (len(titles) != 1 || titles[0] != c.pushed) {
				t.Fatalf("pushes: %v", titles)
			}
		})
	}
}

func TestARefusedBookingSaysWhy(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	var got pushes
	s := New(t.TempDir())
	s.now = func() time.Time { return now }
	s.Notify = got.notify
	s.Pane = func(string, string) (Place, bool) {
		return Place{Agent: "claude", Session: "s1", Status: herdr.StatusIdle, Workspace: "api"}, true
	}
	s.Submit = func(string, string, string) error { return errors.New("the agent is busy with a menu") }
	s.kept.Bookings["gog|p1"] = Booking{Machine: "gog", Pane: "p1", Session: "s1", At: now, Text: "continue"}
	s.Tick()
	if len(got.sent) != 1 || got.sent[0].Title != "Could not continue api" || got.sent[0].Body != "the agent is busy with a menu" {
		t.Fatalf("pushes: %+v", got.sent)
	}
}

func TestAClosedPaneTakesItsBookingAndHitWithIt(t *testing.T) {
	s := New(t.TempDir())
	s.hits["gog|p1"] = Hit{At: time.Now()}
	s.kept.Bookings["gog|p1"] = Booking{Machine: "gog", Pane: "p1", At: time.Now().Add(time.Hour)}
	s.OnStatus(events.PaneStatus{Pane: events.PaneRef{Machine: "gog", Pane: "p1"}, From: herdr.StatusIdle, To: events.Gone})
	if s.PaneOf("gog", "p1") != nil {
		t.Fatal("kept after the pane closed")
	}
	s.kept.Bookings["roofr|p1"] = Booking{Machine: "roofr", Pane: "p1", At: time.Now().Add(time.Hour)}
	s.readings["roofr"] = Reading{Wrapped: true}
	s.Forget("roofr")
	if s.PaneOf("roofr", "p1") != nil || s.Of("roofr").Wrapped {
		t.Fatal("kept after the machine went")
	}
}

func TestResetWords(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.Local)
	if got := ResetWords(now.Add(72*time.Minute), now); got != "at 11:12, in 1 h 12 min" {
		t.Fatalf("today: %q", got)
	}
	if got := ResetWords(now.Add(3*24*time.Hour), now); !strings.HasSuffix(got, "10:00, in 3 days") || strings.HasPrefix(got, "at ") {
		t.Fatalf("days ahead: %q", got)
	}
	for d, want := range map[time.Duration]string{20 * time.Second: "1 min", 40 * time.Minute: "40 min", 2 * time.Hour: "2 h", 0: "a moment"} {
		if got := Left(d); got != want {
			t.Fatalf("Left(%v) = %q, want %q", d, got, want)
		}
	}
}
