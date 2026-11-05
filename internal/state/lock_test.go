package state

import (
	"os"
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
	if os.Geteuid() != 0 {
		locked := filepath.Join(t.TempDir(), "remuda")
		if err := os.Mkdir(locked, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(locked, 0o700) })
		if !Held(locked) {
			t.Fatal("a dir whose lock cannot be taken reads as free")
		}
	}
}
