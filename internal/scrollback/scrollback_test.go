package scrollback

import (
	"strings"
	"testing"
)

func TestRowsDropTheTrailingNewlineOnly(t *testing.T) {
	if got := Rows("a\nb\n"); len(got) != 2 || got[1] != "b" {
		t.Errorf("Rows = %q", got)
	}
	if got := Rows("a\n\n"); len(got) != 2 || got[1] != "" {
		t.Errorf("an empty last row stays: %q", got)
	}
	if got := Rows(""); len(got) != 1 {
		t.Errorf("an empty text is one empty row: %q", got)
	}
}

func TestFindIsCaseFoldedAndNeverOverlaps(t *testing.T) {
	rows := []string{"Error: build failed", "", "no error here, ERROR there", "aaaa"}
	got := Find(rows, "error")
	want := []Match{{0, 0, 5}, {2, 3, 5}, {2, 15, 5}}
	if len(got) != len(want) {
		t.Fatalf("Find = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("match %d = %v, want %v", i, got[i], want[i])
		}
	}
	if got := Find(rows, "aa"); len(got) != 2 || got[1].Col != 2 {
		t.Errorf("overlapping: %v", got)
	}
	if got := Find(rows, ""); got != nil {
		t.Errorf("an empty query finds nothing, got %v", got)
	}
}

func TestFindCountsColumnsInCharacters(t *testing.T) {
	rows := []string{"привет мир", "İstanbul ist", "日本語 err"}
	if got := Find(rows, "мир"); len(got) != 1 || got[0].Col != 7 || got[0].Len != 3 {
		t.Errorf("cyrillic: %v", got)
	}
	// İ folds to i, and the line's byte length moves under it: the columns still count characters
	if got := Find(rows, "ist"); len(got) != 2 || got[0] != (Match{1, 0, 3}) || got[1] != (Match{1, 9, 3}) {
		t.Errorf("a line whose folding moves the offsets: %v", got)
	}
	if got := Find(rows, "err"); len(got) != 1 || got[0].Col != 4 {
		t.Errorf("after wide characters: %v", got)
	}
}

func TestFindStopsAtTheBound(t *testing.T) {
	rows := []string{strings.Repeat("x ", MaxMatches+50)}
	if got := Find(rows, "x"); len(got) != MaxMatches {
		t.Errorf("%d matches, want the bound %d", len(got), MaxMatches)
	}
	many := make([]string, MaxMatches+10)
	for i := range many {
		many[i] = "x"
	}
	if got := Find(many, "x"); len(got) != MaxMatches {
		t.Errorf("%d matches over rows, want the bound %d", len(got), MaxMatches)
	}
}
