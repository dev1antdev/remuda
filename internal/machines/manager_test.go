package machines

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestAServersHostIsNoMachineOfTheFleet(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
	m.machines["gog"] = newRemote(Config{ID: "gog", Label: "GOG", Host: "gog.remote"})
	m.order = append(m.order, "gog")
	if ids := idsOf(m.List()); ids != "local,gog" {
		t.Fatalf("a cockpit's fleet is %s", ids)
	}
	m.SetServer(true)
	if !m.Server() || idsOf(m.List()) != "gog" || idsOf(m.All()) != "local,gog" {
		t.Fatalf("a server's fleet %s, every machine %s", idsOf(m.List()), idsOf(m.All()))
	}
	host, err := m.Get(LocalID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := host.Ready(); !errors.Is(err, ErrServerHost) {
		t.Fatalf("the host's herdr was offered: %v", err)
	}
	m.SetServer(false)
	if _, _, err := host.Ready(); err != nil || idsOf(m.List()) != "local,gog" {
		t.Fatalf("back to a cockpit: %v, %s", err, idsOf(m.List()))
	}
}

func idsOf(list []*Machine) string {
	ids := make([]string, len(list))
	for i, mc := range list {
		ids[i] = mc.ID()
	}
	return strings.Join(ids, ",")
}

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
