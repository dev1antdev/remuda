package feed

import (
	"testing"
)

func TestAddListKeepsNewestFirstAndBounded(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if s.LastID() != 0 || len(s.List(10)) != 0 {
		t.Fatalf("a fresh feed is empty")
	}
	for i := 0; i < Max+5; i++ {
		s.Add(Entry{Kind: "done", Machine: "local", Pane: "w1:p1", Title: "ws finished"})
	}
	all := s.List(0)
	if len(all) != Max {
		t.Fatalf("kept %d, want %d", len(all), Max)
	}
	if all[0].ID != int64(Max+5) || all[1].ID != int64(Max+4) {
		t.Errorf("newest first: %d, %d", all[0].ID, all[1].ID)
	}
	if s.LastID() != int64(Max+5) {
		t.Errorf("LastID %d", s.LastID())
	}
	if got := s.List(2); len(got) != 2 || got[0].At.IsZero() {
		t.Errorf("List(2): %+v", got)
	}
	again := New(dir)
	if again.LastID() != int64(Max+5) || len(again.List(0)) != Max {
		t.Errorf("not persisted: last %d, %d entries", again.LastID(), len(again.List(0)))
	}
}

func TestAQuietEntryIsListedButIsNoNews(t *testing.T) {
	s := New(t.TempDir())
	news := s.Add(Entry{Kind: "done", Machine: "local", Pane: "w1:p1", Title: "ws finished"})
	s.Add(Entry{Kind: "sent", Quiet: true, Machine: "local", Pane: "w1:p1", Title: "iPhone sent «yes» to ws · w1:p1"})
	if got := s.LastID(); got != news.ID {
		t.Fatalf("LastID = %d, want the news entry %d", got, news.ID)
	}
	if list := s.List(0); len(list) != 2 || !list[0].Quiet || list[1].Quiet {
		t.Fatalf("List: %+v", list)
	}
	only := New(t.TempDir())
	only.Add(Entry{Kind: "sent", Quiet: true, Title: "quiet"})
	if only.LastID() != 0 {
		t.Fatalf("a feed of quiet entries alone has no news")
	}
}
