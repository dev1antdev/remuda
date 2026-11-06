package tasksapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRosterReadsTheAgentPanes(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Write([]byte(`{"version":"v0.8.4","machines":[
		  {"id":"local","label":"This Mac","state":"connected","sessions":[{"id":"w1","name":"remuda","dir":"/Users/me/remuda","tabs":[{"id":"t1","label":"1","panes":[
		    {"id":"w1:p1","title":"remuda","agent":"claude","cwd":"/Users/me/remuda","status":"run","radar":{"state":"working","since":1700000000000},"usage":{"model":"claude-opus-4-1","name":"Opus","cost":4.2},"pr":{"url":"https://github.com/o/r/pull/15","label":"PR #15"}},
		    {"id":"w1:p2","title":"zsh","agent":"","cwd":"/Users/me/remuda","status":"shell"}]}]}]},
		  {"id":"gog","label":"GOG","state":"connecting","sessions":[]}]}`))
	}))
	defer srv.Close()
	c := New(Door{URL: srv.URL, Key: "rk_test"})
	roster, err := c.Roster()
	if err != nil || len(roster.Machines) != 2 || roster.Version != "v0.8.4" {
		t.Fatalf("roster: %v %+v", err, roster)
	}
	if got.Method != "GET" || got.URL.Path != "/api/roster" || got.Header.Get("Authorization") != "Bearer rk_test" {
		t.Errorf("the request: %s %s %s", got.Method, got.URL.Path, got.Header.Get("Authorization"))
	}
	agents := roster.Machines[0].AgentPanes()
	if len(agents) != 1 || agents[0].Pane.ID != "w1:p1" || agents[0].Workspace.Name != "remuda" || agents[0].Pane.Radar.State != "working" || agents[0].Pane.Usage.Cost != 4.2 || agents[0].Pane.PR.Label != "PR #15" {
		t.Errorf("the agent panes, shells left out: %+v", agents)
	}
	if len(roster.Machines[1].AgentPanes()) != 0 || roster.Machines[1].State != "connecting" {
		t.Errorf("a machine without panes: %+v", roster.Machines[1])
	}
}

func TestRecapAndStandupSendTheirQueries(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		w.Write([]byte(`{"since":"2026-10-27T00:00:00Z","markdown":"# Recap\n\n- 3 prompts\n"}`))
	}))
	defer srv.Close()
	c := New(Door{URL: srv.URL, Key: "rk_test"})
	since := time.Date(2026, 10, 27, 0, 0, 0, 0, time.FixedZone("x", 3*3600))
	rec, err := c.Recap("gog", "w3:p1", "", since)
	if err != nil || rec.Markdown != "# Recap\n\n- 3 prompts\n" || rec.Since != "2026-10-27T00:00:00Z" {
		t.Fatalf("recap: %v %+v", err, rec)
	}
	if _, err := c.Recap("local", "w1:p1", "0123", time.Time{}); err != nil {
		t.Fatal(err)
	}
	standup, err := c.Standup(time.Time{}, "Europe/Kyiv")
	if err != nil || standup.Markdown == "" {
		t.Fatalf("standup: %v %+v", err, standup)
	}
	if _, err := c.Standup(since, ""); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/api/recap?machine=gog&pane=w3%3Ap1&since=2026-10-26T21%3A00%3A00Z",
		"/api/recap?machine=local&pane=w1%3Ap1&session=0123",
		"/api/standup?tz=Europe%2FKyiv",
		"/api/standup?since=2026-10-26T21%3A00%3A00Z",
	}
	for i, q := range want {
		if i >= len(queries) || queries[i] != q {
			t.Errorf("query %d: %q, want %q", i, queries, q)
		}
	}
}
