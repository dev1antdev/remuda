package server

import (
	"context"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/feed"
)

// A sending becomes a quiet activity row that names the caller and never counts as news.
func TestASendingIsAQuietRowNamingTheCaller(t *testing.T) {
	s := &Server{Feed: feed.New(t.TempDir())}
	s.noteSent(context.Background(), "local", "w1:p1", "echo hi", false)
	s.noteSent(auth.WithCallerName(context.Background(), "GOG"), "local", "w1:p1", "Esc", true)
	rows := s.Feed.List(0)
	if len(rows) != 2 || !rows[0].Quiet || !rows[1].Quiet {
		t.Fatalf("rows: %+v", rows)
	}
	if rows[1].Title != "an API client sent «echo hi» to w1:p1" || rows[1].Kind != "sent" || rows[1].Pane != "w1:p1" {
		t.Errorf("a bearer without a device: %+v", rows[1])
	}
	if rows[0].Title != "GOG sent Esc to w1:p1" {
		t.Errorf("a named caller: %q", rows[0].Title)
	}
	if s.Feed.LastID() != 0 {
		t.Errorf("the rows should be no news, LastID = %d", s.Feed.LastID())
	}
	(&Server{}).noteSent(context.Background(), "local", "w1:p1", "nothing kept", false) // no feed: nothing happens
}
