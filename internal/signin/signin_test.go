package signin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/push"
)

// clocked is a service on a clock the test moves, with the pushes it sends kept
func clocked(dir string, now *time.Time) (*Service, *[]push.Payload) {
	s := New(dir)
	s.now = func() time.Time { return *now }
	var sent []push.Payload
	s.Notify = func(p push.Payload) { sent = append(sent, p) }
	return s, &sent
}

func TestLevel(t *testing.T) {
	expires := time.Date(2026, 12, 5, 16, 0, 0, 0, time.UTC)
	cases := []struct {
		before time.Duration
		want   int
	}{
		{10 * 24 * time.Hour, 0},
		{72*time.Hour + time.Minute, 0},
		{72 * time.Hour, Soon},
		{25 * time.Hour, Soon},
		{24 * time.Hour, Day},
		{time.Minute, Day},
		{0, Ended},
		{-48 * time.Hour, Ended},
	}
	for _, c := range cases {
		if got := Level(expires, expires.Add(-c.before)); got != c.want {
			t.Errorf("%v before the end: level %d, want %d", c.before, got, c.want)
		}
	}
}

func TestOneWarningPerThresholdKeptAcrossARestart(t *testing.T) {
	dir := t.TempDir()
	expires := time.Date(2026, 12, 5, 16, 0, 0, 0, time.UTC)
	now := expires.Add(-5 * 24 * time.Hour)
	s, sent := clocked(dir, &now)
	s.Observe("gog", "GOG", expires)
	if len(*sent) != 0 {
		t.Fatalf("five days ahead is no push yet: %+v", *sent)
	}
	now = expires.Add(-70 * time.Hour)
	s.Observe("gog", "GOG", expires)
	s.Observe("gog", "GOG", expires)
	if len(*sent) != 1 || (*sent)[0].Title != "Claude Code's sign-in on GOG ends in 3 days" || (*sent)[0].Kind != "signin" || (*sent)[0].Machine != "gog" || (*sent)[0].Tag != "signin|gog" {
		t.Fatalf("three days: %+v", *sent)
	}

	restarted, again := clocked(dir, &now)
	restarted.Observe("gog", "GOG", expires)
	if len(*again) != 0 {
		t.Fatalf("a restart warned again: %+v", *again)
	}
	now = expires.Add(-20 * time.Hour)
	restarted.Observe("gog", "GOG", expires)
	now = expires.Add(time.Hour)
	restarted.Observe("gog", "GOG", expires)
	restarted.Observe("gog", "GOG", expires)
	if len(*again) != 2 || (*again)[0].Title != "Claude Code's sign-in on GOG ends in 20 hours" || (*again)[1].Title != "Claude Code's sign-in on GOG has ended" {
		t.Fatalf("a day, then the end: %+v", *again)
	}
}

func TestANewSignInStartsAfreshAndAMachineFirstSeenLateHearsTheGraverAlone(t *testing.T) {
	expires := time.Date(2026, 12, 5, 16, 0, 0, 0, time.UTC)
	now := expires.Add(-2 * time.Hour)
	s, sent := clocked("", &now)
	s.Observe("gog", "GOG", expires)
	if len(*sent) != 1 || (*sent)[0].Title != "Claude Code's sign-in on GOG ends in 2 hours" {
		t.Fatalf("first seen two hours before: %+v", *sent)
	}
	renewed := expires.Add(30 * 24 * time.Hour)
	s.Observe("gog", "GOG", renewed)
	now = renewed.Add(-60 * time.Hour)
	s.Observe("gog", "GOG", renewed)
	if len(*sent) != 2 || (*sent)[1].Title != "Claude Code's sign-in on GOG ends in 3 days" {
		t.Fatalf("the new sign-in's own warning: %+v", *sent)
	}
	s.Observe("gog", "GOG", time.Time{})
	s.Observe("pi", "Pi", now.Add(-time.Hour))
	if len(*sent) != 3 || (*sent)[2].Machine != "pi" || (*sent)[2].Body != "Its agents cannot reach Claude until you sign in again: run claude auth login on Pi." {
		t.Fatalf("no end known says nothing, an ended one on another machine its own: %+v", *sent)
	}
}

func TestForgetAndADamagedFile(t *testing.T) {
	dir := t.TempDir()
	expires := time.Date(2026, 12, 5, 16, 0, 0, 0, time.UTC)
	now := expires.Add(-time.Hour)
	s, _ := clocked(dir, &now)
	s.Observe("gog", "GOG", expires)
	s.Forget("gog")
	again, sent := clocked(dir, &now)
	again.Observe("gog", "GOG", expires)
	if len(*sent) != 1 {
		t.Fatal("a forgotten machine is warned anew once it comes back")
	}
	if err := os.WriteFile(filepath.Join(dir, stateFile), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if broken, _ := clocked(dir, &now); len(broken.kept) != 0 {
		t.Fatal("a damaged file reads as nothing given")
	}
}

func TestLeft(t *testing.T) {
	for d, want := range map[time.Duration]string{71 * time.Hour: "3 days", 47 * time.Hour: "47 hours", 90 * time.Minute: "90 minutes", 30 * time.Second: "a minute"} {
		if got := Left(d); got != want {
			t.Errorf("Left(%v) = %q, want %q", d, got, want)
		}
	}
}
