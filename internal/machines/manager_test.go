package machines

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/coreapi"
)

// silentAPI answers nothing: a machine of a test that never asks it, and reaches no core of anyone's.
type silentAPI struct{ coreapi.API }

// newTestManager is the fleet of a test that only reads the registry.
func newTestManager(dir string) *Manager { return NewManagerWith(dir, nil, false, &silentAPI{}, nil) }

func TestAServersHostIsNoMachineOfTheFleet(t *testing.T) {
	dir := t.TempDir()
	m := newTestManager(dir)
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

// The person's order: the machines it names first, as they dragged them; one added since after them, as it arrived.
func TestTheFleetIsListedInThePersonsOrder(t *testing.T) {
	dir := t.TempDir()
	m := newTestManager(dir)
	for _, id := range []string{"gog", "roofr", "windows"} {
		m.machines[id] = newRemote(Config{ID: id, Label: id, Host: id + ".remote"})
		m.order = append(m.order, id)
	}
	if ids := idsOf(m.List()); ids != "local,gog,roofr,windows" {
		t.Fatalf("before any order: %s", ids)
	}
	if err := m.SetOrder([]string{"windows", "local", "roofr", "gog"}); err != nil {
		t.Fatal(err)
	}
	if ids := idsOf(m.List()); ids != "windows,local,roofr,gog" {
		t.Fatalf("after the order: %s", ids)
	}
	if ids := idsOf(m.All()); ids != "local,gog,roofr,windows" {
		t.Fatalf("every machine keeps the order it arrived in: %s", ids)
	}
	m.machines["pi"] = newRemote(Config{ID: "pi", Label: "pi", Host: "pi.remote"})
	m.order = append(m.order, "pi")
	if ids := idsOf(m.List()); ids != "windows,local,roofr,gog,pi" {
		t.Fatalf("a machine added since does not come last: %s", ids)
	}
	m.drop("roofr")
	if ids := idsOf(m.List()); ids != "windows,local,gog,pi" {
		t.Fatalf("a machine gone is still listed, or the rest moved: %s", ids)
	}
	m.SetServer(true)
	if ids := idsOf(m.List()); ids != "windows,gog,pi" {
		t.Fatalf("a server's host is listed in the order: %s", ids)
	}
}

func TestAnOrderNamesKnownMachinesOnce(t *testing.T) {
	dir := t.TempDir()
	m := newTestManager(dir)
	m.machines["gog"] = newRemote(Config{ID: "gog", Label: "GOG", Host: "gog.remote"})
	m.order = append(m.order, "gog")
	if err := m.SetOrder([]string{"gog", "nowhere"}); err == nil {
		t.Fatal("an order naming a machine the registry lacks was kept")
	}
	if err := m.SetOrder([]string{"gog", "gog"}); err == nil {
		t.Fatal("an order naming a machine twice was kept")
	}
	if err := m.SetOrder(make([]string, maxPlaced+1)); err == nil {
		t.Fatal("an order longer than any fleet was kept")
	}
	if ids := idsOf(m.List()); ids != "local,gog" {
		t.Fatalf("a refused order moved the fleet: %s", ids)
	}
	if err := m.SetOrder([]string{"gog"}); err != nil {
		t.Fatal(err)
	}
	if ids := idsOf(m.List()); ids != "gog,local" {
		t.Fatalf("an order naming one machine: %s", ids)
	}
}

// The order outlives a restart: the next registry on the same state dir reads it.
func TestTheOrderIsKeptAcrossARestart(t *testing.T) {
	dir := t.TempDir()
	m := newTestManager(dir)
	m.machines["gog"] = newRemote(Config{ID: "gog", Label: "GOG", Host: "gog.remote"})
	m.order = append(m.order, "gog")
	if err := m.SetOrder([]string{"gog", "local"}); err != nil {
		t.Fatal(err)
	}
	next := newTestManager(dir)
	next.machines["gog"] = newRemote(Config{ID: "gog", Label: "GOG", Host: "gog.remote"})
	next.order = append(next.order, "gog")
	if ids := idsOf(next.List()); ids != "gog,local" {
		t.Fatalf("after a restart: %s", ids)
	}
}

func TestArrangedKeepsEveryMachineOnce(t *testing.T) {
	for _, c := range []struct{ arrival, placed, want string }{
		{"a,b,c", "", "a,b,c"},
		{"a,b,c", "c,a", "c,a,b"},
		{"a,b,c", "c,x,a,c", "c,a,b"},
		{"", "a", ""},
	} {
		got := strings.Join(arranged(split(c.arrival), split(c.placed)), ",")
		if got != c.want {
			t.Errorf("arranged(%q, %q) = %q, want %q", c.arrival, c.placed, got, c.want)
		}
	}
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// TestARemovalThatCouldNotBeKeptLeavesTheMachine: the list without it is written before it goes, so a disk that refused
// the write leaves the machine a machine of the fleet, its panes and checkpoints with it.
func TestARemovalThatCouldNotBeKeptLeavesTheMachine(t *testing.T) {
	dir := t.TempDir()
	m := newTestManager(dir)
	// in the registry, never connected: a host named for a test must reach no ssh of the person's
	m.machines["box"] = newRemote(Config{ID: "box", Label: "Box", Host: "box.remote"})
	m.order = append(m.order, "box")
	if err := os.Mkdir(filepath.Join(dir, "machines.json"), 0o755); err != nil { // a write over a folder fails
		t.Fatal(err)
	}
	if err := m.Remove("box"); err == nil {
		t.Fatal("a removal that could not be written went ahead")
	}
	if _, err := m.Get("box"); err != nil || m.Gone("box") {
		t.Fatalf("the machine left the fleet for a removal that did not land: %v", err)
	}
}

// TestTwoRemovalsBothStayRemoved: a removal writes the list without its machine before the machine goes, one at a time,
// so the second can never write the first back — not in the running fleet, and not in the file a restart reads.
func TestTwoRemovalsBothStayRemoved(t *testing.T) {
	dir := t.TempDir()
	m := newTestManager(dir)
	for _, id := range []string{"one", "two"} {
		m.machines[id] = newRemote(Config{ID: id, Label: id, Host: id + ".remote"})
		m.order = append(m.order, id)
	}
	if err := m.save(); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("one"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("two"); err != nil {
		t.Fatal(err)
	}
	if ids := idsOf(m.List()); ids != "local" {
		t.Fatalf("the fleet after both removals: %s", ids)
	}
	// the file a restart reads, decoded as it stands: no manager, so a regression here reaches no ssh and no core
	saved, err := os.ReadFile(filepath.Join(dir, "machines.json"))
	if err != nil {
		t.Fatal(err)
	}
	var remotes []Config
	if err := json.Unmarshal(saved, &remotes); err != nil {
		t.Fatalf("machines.json: %v\n%s", err, saved)
	}
	if len(remotes) != 0 {
		t.Fatalf("a restart brings back %+v", remotes)
	}
}

// TestARemovalOfOneAlreadyRemovedLocksNothing: two tabs removing one machine, the second finds it gone; the fleet's
// next write must not wait behind anything the second left held.
func TestARemovalOfOneAlreadyRemovedLocksNothing(t *testing.T) {
	m := newTestManager(t.TempDir())
	m.machines["box"] = newRemote(Config{ID: "box", Label: "Box", Host: "box.remote"})
	m.order = append(m.order, "box")
	if err := m.Remove("box"); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("box"); err == nil {
		t.Fatal("a machine removed twice went twice")
	}
	done := make(chan error, 1)
	go func() { done <- m.save() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the fleet's next write waits behind the removal of a machine already gone")
	}
}
