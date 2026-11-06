package radar

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
)

// step is one poll of herdr (or a look from the page, seen) at so many milliseconds, and the state shown after it.
type step struct {
	at      int
	status  string
	focused bool
	seen    bool
	want    string
}

func TestRadarRules(t *testing.T) {
	const (
		work  = herdr.StatusWorking
		done  = herdr.StatusDone
		idle  = herdr.StatusIdle
		ask   = herdr.StatusBlocked
		unkno = herdr.StatusUnknown
	)
	cases := []struct {
		name  string
		steps []step
	}{
		{"a blink through idle within 2.5 s is still working", []step{
			{at: 0, status: work, want: work}, {at: 1000, status: idle, want: work}, {at: 2400, status: idle, want: work}, {at: 2600, status: idle, want: done}}},
		{"a blink through done within the grace, then done for good", []step{
			{at: 0, status: work, want: work}, {at: 2000, status: done, want: work}, {at: 4000, status: done, want: done}}},
		{"done is held until seen, and stays seen while herdr says done", []step{
			{at: 0, status: work, want: work}, {at: 5000, status: done, want: done}, {at: 9000, status: idle, want: done},
			{at: 10000, seen: true, status: idle, want: idle}, {at: 12000, status: done, want: idle}, {at: 14000, status: done, want: idle},
			{at: 16000, status: work, want: work}, {at: 20000, status: done, want: done}}},
		{"herdr's own focus counts as a look", []step{
			{at: 0, status: work, want: work}, {at: 5000, status: done, want: done}, {at: 7000, status: done, focused: true, want: idle},
			{at: 9000, status: done, want: idle}}},
		{"a question is held until the agent works again", []step{
			{at: 0, status: work, want: work}, {at: 5000, status: ask, want: ask}, {at: 7000, status: idle, want: ask},
			{at: 9000, status: done, want: ask}, {at: 11000, status: work, want: work}, {at: 15000, status: idle, want: done}}},
		{"a question with no work before it simply closes", []step{
			{at: 0, status: ask, want: ask}, {at: 2000, status: idle, want: idle}}},
		{"a pane first seen finished is no news", []step{
			{at: 0, status: done, want: idle}, {at: 2000, status: done, want: idle}, {at: 4000, status: work, want: work},
			{at: 9000, status: done, want: done}}},
		{"a pane first seen idle, then a finish", []step{
			{at: 0, status: idle, want: idle}, {at: 2000, status: work, want: work}, {at: 8000, status: idle, want: done}}},
		{"an unknown status is shown as such", []step{
			{at: 0, status: unkno, want: unkno}}},
	}
	for _, c := range cases {
		tr := New(t.TempDir(), nil)
		start := time.Unix(1_800_000_000, 0)
		for i, s := range c.steps {
			if s.seen {
				tr.Seen("m", "p1")
			}
			snapshot := snap(s.status, s.focused)
			tr.Observe("m", snapshot, start.Add(time.Duration(s.at)*time.Millisecond))
			if got := display(tr, snapshot).State; got != s.want {
				t.Errorf("%s: step %d (%s at %d ms): %s, want %s", c.name, i+1, s.status, s.at, got, s.want)
				break
			}
		}
	}
}

func TestRadarSinceMarksWhenTheShownStateBegan(t *testing.T) {
	tr := New(t.TempDir(), nil)
	start := time.Unix(1_800_000_000, 0)
	tr.Observe("m", snap(herdr.StatusWorking, false), start)
	tr.Observe("m", snap(herdr.StatusWorking, false), start.Add(time.Minute))
	if d := display(tr, snap(herdr.StatusWorking, false)); d.Since != start.UnixMilli() || d.LastActive != start.Add(time.Minute).Truncate(time.Minute).UnixMilli() {
		t.Fatalf("working: %+v", d)
	}
	tr.Observe("m", snap(herdr.StatusBlocked, false), start.Add(2*time.Minute))
	tr.SetAlert("m", "p1", "waiting for your answer for 15 min")
	if eps := tr.Episodes(); len(eps) != 1 || eps[0].State != herdr.StatusBlocked || !eps[0].Since.Equal(start.Add(2*time.Minute)) {
		t.Fatalf("episodes: %+v", eps)
	}
	tr.Observe("m", snap(herdr.StatusWorking, false), start.Add(3*time.Minute))
	if d := display(tr, snap(herdr.StatusWorking, false)); d.Alert != "" || d.Since != start.Add(3*time.Minute).UnixMilli() {
		t.Fatalf("a new state drops the note and starts its clock: %+v", d)
	}
}
