package sent

import (
	"strings"
	"testing"
)

func TestARowNamesTheSenderTheTextAndThePlace(t *testing.T) {
	e := Entry("iPhone", "yes", false, "local", "w3:p1", "api")
	if e.Kind != Kind || !e.Quiet || e.Machine != "local" || e.Pane != "w3:p1" {
		t.Fatalf("entry: %+v", e)
	}
	if e.Title != "iPhone sent «yes» to api · w3:p1" {
		t.Fatalf("title: %q", e.Title)
	}
	if got := Entry("remuda", "/restart", false, "gog", "w1:p2", "").Title; got != "remuda sent «/restart» to w1:p2" {
		t.Fatalf("no workspace: %q", got)
	}
	if got := Entry("Chrome on macOS", KeyWords([]string{"esc", "shift+tab", "up", "ctrl+c", "f9"}), true, "local", "w1:p1", "ws").Title; got != "Chrome on macOS sent Esc ⇧Tab ↑ Ctrl+C f9 to ws · w1:p1" {
		t.Fatalf("keys: %q", got)
	}
}

func TestATextIsOneLineAndCut(t *testing.T) {
	long := strings.Repeat("слово ", 30)
	cut := Cut("first line\n\n  second   line")
	if cut != "first line second line" {
		t.Fatalf("lines: %q", cut)
	}
	if got := Cut(long); !strings.HasSuffix(got, "…") || len([]rune(got)) > MaxText+1 {
		t.Fatalf("cut: %q (%d runes)", got, len([]rune(got)))
	}
}
