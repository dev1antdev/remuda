package limits

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// snapshotAt is a machine's answer carrying Claude's 5-hour window at the use given, reported at a moment
func snapshotAt(at time.Time, used float64, resets time.Time) string {
	return script.RS + "\n" + `{"at":"` + at.UTC().Format(time.RFC3339) + `","rate_limits":{"five_hour":{"used_percentage":` + strconv.FormatFloat(used, 'f', -1, 64) + `,"resets_at":` + strconv.FormatInt(resets.Unix(), 10) + `}}}` + "\n" + script.RS + "\n\n"
}

// paced is a service that read Claude's 5-hour window on gog every five minutes, rising by step from start, up to now
func paced(t *testing.T, dir string, now *time.Time, resets time.Time, start, step float64, got *pushes) (*Service, *shell.Fake) {
	t.Helper()
	answer := ""
	host := &shell.Fake{Name: "gog", Remote: true, Answer: func(string) (string, error) { return answer, nil }}
	s := New(dir)
	s.now = func() time.Time { return *now }
	if got != nil {
		s.Notify = got.notify
	}
	end := *now
	for i := 0; i < 6; i++ {
		*now = end.Add(-time.Duration(5-i) * 5 * time.Minute)
		answer = snapshotAt(*now, start+float64(i)*step, resets)
		s.Read(host, "GOG")
	}
	return s, host
}

func TestAWindowForecastToRunOutEarlyIsPushedOnce(t *testing.T) {
	now := time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)
	resets := now.Add(3 * time.Hour)
	var got pushes
	s, host := paced(t, t.TempDir(), &now, resets, 40, 2, &got) // 24 % an hour: out at 16:05, nearly an hour early
	w := s.Of("gog").Windows[0]
	if w.Forecast == nil || w.Forecast.RunsOutAt == nil {
		t.Fatalf("no forecast on the reading: %+v", w)
	}
	forecasts := forecastPushes(&got)
	if len(forecasts) != 1 || !strings.Contains(forecasts[0].Title, "runs out ≈") || !strings.Contains(forecasts[0].Body, "before it resets") || forecasts[0].Kind != "limits" {
		t.Fatalf("forecast pushes %+v", forecasts)
	}
	s.Read(host, "GOG")
	if n := len(forecastPushes(&got)); n != 1 {
		t.Fatalf("%d forecast pushes", n)
	}
}

func forecastPushes(got *pushes) []push.Payload {
	got.mu.Lock()
	defer got.mu.Unlock()
	var out []push.Payload
	for _, p := range got.sent {
		if strings.HasPrefix(p.Tag, "limits-forecast|") {
			out = append(out, p)
		}
	}
	return out
}

func TestAWindowThatLastsIsNotPushed(t *testing.T) {
	now := time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)
	var got pushes
	s, _ := paced(t, t.TempDir(), &now, now.Add(2*time.Hour), 20, 0.5, &got)
	if w := s.Of("gog").Windows[0]; w.Forecast == nil || !w.Forecast.Lasts {
		t.Fatalf("forecast %+v", w.Forecast)
	}
	if len(got.sent) != 0 {
		t.Fatalf("pushed %v", got.titles())
	}
}

func TestHoldsAtNinetyFiveAndOnARunOutSoon(t *testing.T) {
	now := time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)
	resets := now.Add(3 * time.Hour)
	s, _ := paced(t, t.TempDir(), &now, resets, 60, 1, nil)
	if hold, ok := s.Holding("gog", "claude"); ok {
		t.Fatalf("held at 65 %%: %+v", hold)
	}
	s2, _ := paced(t, t.TempDir(), &now, resets, 91, 1, nil) // 96 %
	hold, ok := s2.Holding("gog", "claude")
	if !ok || !strings.Contains(hold.Why, "is at 96 %") || hold.Episode == "" {
		t.Fatalf("hold at 96 %%: %+v %v", hold, ok)
	}
	if _, ok := s2.Holding("gog", "codex"); ok {
		t.Fatal("Codex held on Claude's window")
	}
	s3, _ := paced(t, t.TempDir(), &now, resets, 70, 4, nil) // 90 %, 48 % an hour: out in about 12 minutes
	hold, ok = s3.Holding("gog", "claude")
	if !ok || !strings.Contains(hold.Why, "runs out ≈") {
		t.Fatalf("hold on a run-out soon: %+v %v", hold, ok)
	}
	if o := hold.Offer(Offer{Kind: OfferTask, Task: 12}); o.Window != "5h" || o.Used != 90 || o.RunsOut == nil || !o.ResetsAt.Equal(resets) || o.Episode != hold.Episode || o.Task != 12 {
		t.Fatalf("the offer of a forecast hold: %+v", o)
	}
	if o := (Hold{Window: Window{Name: "5h", Used: 97, Forecast: &Outlook{}}}).Offer(Offer{}); o.RunsOut != nil || o.Used != 97 {
		t.Fatalf("a hold at 97 %% offered as a forecast: %+v", o)
	}
	if hold, ok := s3.Holding("roofr", "claude"); !ok || hold.Episode == "" {
		t.Fatalf("a machine with no reading of its own is not held by the account's: %+v %v", hold, ok)
	}
}

func offerFor(task int64, episode string, resets time.Time) Offer {
	return Offer{Kind: OfferTask, Project: 1, Task: task, Ref: "#" + strconv.FormatInt(task, 10), Title: "Fix the login page", Machine: "gog", From: "claude", To: "codex", Why: "Claude's 5 h window is at 96 %", Episode: episode, ResetsAt: resets}
}

func TestAnOfferIsMadeOncePerCardAndEpisode(t *testing.T) {
	now := time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	var got pushes
	s := New(dir)
	s.now = func() time.Time { return now }
	s.Notify = got.notify
	resets := now.Add(time.Hour)
	if !s.Offer(offerFor(12, "claude|5h|1", resets)) {
		t.Fatal("no offer made")
	}
	if s.Offer(offerFor(12, "claude|5h|1", resets)) {
		t.Fatal("the same card offered twice in one episode")
	}
	if !s.Offer(offerFor(13, "claude|5h|1", resets)) {
		t.Fatal("the next card was not offered")
	}
	if p := got.sent[0]; p.Offer == "" || len(p.Actions) != 1 || p.Actions[0].Action != push.ActionOffer || p.Actions[0].Title != "Run on Codex" || p.Kind != "limits" {
		t.Fatalf("the push %+v", p)
	}
	again := New(dir) // a restart remembers the offers made
	again.now = s.now
	if again.Offer(offerFor(12, "claude|5h|1", resets)) || len(again.Offers()) != 2 {
		t.Fatalf("after a restart: %+v", again.Offers())
	}
	now = resets.Add(time.Minute)
	again.Tick()
	if len(again.Offers()) != 0 {
		t.Fatal("an offer outlived its window's reset")
	}
}

func TestAnOfferRunsOnceAndAFailureLeavesItOpen(t *testing.T) {
	now := time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)
	s := New(t.TempDir())
	s.now = func() time.Time { return now }
	s.Offer(offerFor(12, "claude|5h|1", now.Add(time.Hour)))
	id := s.Offers()[0].ID
	fail := errors.New("the card left Queued")
	var ran []Offer
	s.Run = func(o Offer) (string, error) {
		ran = append(ran, o)
		if len(ran) == 1 {
			return "", fail
		}
		return "started #12 on Codex", nil
	}
	if _, err := s.RunOffer(id); !errors.Is(err, fail) || len(s.Offers()) != 1 {
		t.Fatalf("a failed run: %v, %d open", err, len(s.Offers()))
	}
	if words, err := s.RunOffer(id); err != nil || words != "started #12 on Codex" {
		t.Fatalf("run: %q %v", words, err)
	}
	if _, err := s.RunOffer(id); !errors.Is(err, ErrNoOffer) || len(s.Offers()) != 0 {
		t.Fatalf("a taken offer ran again: %v", err)
	}
	if ran[1].To != "codex" || ran[1].Task != 12 {
		t.Fatalf("ran %+v", ran[1])
	}
}

func TestADeclinedOfferIsNotMadeAgain(t *testing.T) {
	now := time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)
	s := New(t.TempDir())
	s.now = func() time.Time { return now }
	s.Offer(offerFor(12, "claude|5h|1", now.Add(time.Hour)))
	id := s.Offers()[0].ID
	if err := s.DeclineOffer(id); err != nil || len(s.Offers()) != 0 {
		t.Fatalf("declined: %v", err)
	}
	if s.Offer(offerFor(12, "claude|5h|1", now.Add(time.Hour))) {
		t.Fatal("a declined offer came back in the same episode")
	}
	if err := s.DeclineOffer(id); !errors.Is(err, ErrNoOffer) {
		t.Fatalf("declined twice: %v", err)
	}
}

func TestOtherVendors(t *testing.T) {
	if Other("claude") != "codex" || Other("codex") != "claude" || Other("gemini") != "" {
		t.Fatal("the other vendors")
	}
}
