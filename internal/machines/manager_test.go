package machines

import (
	"errors"
	"os"
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

// The person's order: the machines it names first, as they dragged them; one added since after them, as it arrived.
func TestTheFleetIsListedInThePersonsOrder(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
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
	m := NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
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
	m := NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
	m.machines["gog"] = newRemote(Config{ID: "gog", Label: "GOG", Host: "gog.remote"})
	m.order = append(m.order, "gog")
	if err := m.SetOrder([]string{"gog", "local"}); err != nil {
		t.Fatal(err)
	}
	next := NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil)
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

func TestTheFleetCallsAMachineGoneOnlyOnceHerdrsWereRead(t *testing.T) {
	dir := t.TempDir()
	herdr := filepath.Join(dir, "herdr")
	if err := os.WriteFile(herdr, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager(dir, filepath.Join(dir, "no-herdr.sock"), herdr, nil)
	m.SyncHerdr()
	if m.Gone("mini") {
		t.Fatal("gone while herdr's machines could not be read")
	}
	if err := os.WriteFile(herdr, []byte("#!/bin/sh\necho '[]'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.SyncHerdr()
	if !m.Gone("mini") || m.Gone(LocalID) {
		t.Fatal("a machine of no list is not gone, or the local one is")
	}
	failing := NewManager(t.TempDir(), filepath.Join(dir, "no-herdr.sock"), herdr+"-failing", nil)
	if err := os.WriteFile(herdr+"-failing", []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	failing.SyncHerdr()
	added, err := failing.Add("Mini", "mini.invalid")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(failing.dir, removedFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := failing.Remove(added.ID()); err == nil || failing.Gone(added.ID()) {
		t.Fatalf("a removal that could not be noted went through: %v", err)
	}
	if err := os.Remove(filepath.Join(failing.dir, removedFile)); err != nil {
		t.Fatal(err)
	}
	if err := failing.Remove(added.ID()); err != nil {
		t.Fatal(err)
	}
	if !failing.Gone(added.ID()) {
		t.Fatal("a machine the person removed is not gone while herdr fails")
	}
	again := NewManager(failing.dir, filepath.Join(dir, "no-herdr.sock"), herdr+"-failing", nil)
	again.SyncHerdr()
	if !again.Gone(added.ID()) {
		t.Fatal("a removal is forgotten across a restart while herdr fails")
	}
	if _, err := failing.Add("Mini", "mini.invalid"); err != nil || failing.Gone(added.ID()) {
		t.Fatalf("a machine added again is still gone: %v", err)
	}
	for _, bin := range []string{filepath.Join(dir, "no-such-herdr"), ""} {
		none := NewManager(dir, filepath.Join(dir, "no-herdr.sock"), bin, nil)
		none.SyncHerdr()
		if !none.Gone("mini") {
			t.Fatalf("without herdr (%q) the fleet is whole", bin)
		}
	}
}
