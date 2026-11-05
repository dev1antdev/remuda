package state

import (
	"path/filepath"
	"testing"
)

func TestTheStateDirIsHeldWhileItIsServed(t *testing.T) {
	dir := t.TempDir()
	if Held(dir) {
		t.Fatal("a dir nobody serves reads as held")
	}
	release, err := Hold(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !Held(dir) {
		t.Fatal("a served dir reads as free")
	}
	if _, err := Hold(dir); err == nil {
		t.Fatal("a second holder got the dir")
	}
	release()
	if Held(dir) {
		t.Fatal("the dir stayed held after its holder let go")
	}
	if Held(filepath.Join(dir, "missing")) {
		t.Fatal("a dir that is not there reads as held")
	}
}
