package machines

import (
	"strings"
	"testing"
)

// TestRenameCountsCharactersNotBytes: the page lets sixty characters through, so the server counts the same way —
// a Cyrillic name of thirty-one letters is not "over sixty".
func TestRenameCountsCharactersNotBytes(t *testing.T) {
	m := &Manager{machines: map[string]*Machine{}, names: map[string]string{}, dir: t.TempDir()}
	mc := newRemote(Config{ID: "box1", Label: "Box"})
	m.machines["box1"] = mc
	name := strings.Repeat("я", 60)
	if _, err := m.Rename("box1", name); err != nil {
		t.Fatalf("sixty characters refused: %v", err)
	}
	if mc.Label() != name {
		t.Fatalf("Label = %q", mc.Label())
	}
	if _, err := m.Rename("box1", name+"я"); err == nil {
		t.Fatal("sixty-one characters must be refused")
	}
	if _, err := m.Rename("box1", ""); err != nil || mc.Label() != "Box" {
		t.Fatalf("an empty name goes back to the registry's: %v %q", err, mc.Label())
	}
}

func TestTheLocalMachineIsNamedForItsSystem(t *testing.T) {
	for _, c := range []struct{ goos, host, want string }{
		{"darwin", "Sergiis-MacBook-Pro.local", "This Mac · Sergiis-MacBook-Pro"},
		{"linux", "rp-orange", "This machine · rp-orange"},
		{"windows", "desktop-pevp5ap", "This machine · desktop-pevp5ap"},
	} {
		if got := localLabel(c.goos, c.host); got != c.want {
			t.Errorf("localLabel(%q, %q) = %q, want %q", c.goos, c.host, got, c.want)
		}
	}
}
