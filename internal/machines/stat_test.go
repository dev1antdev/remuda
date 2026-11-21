package machines

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// The stat a remote runs, run here in sh: a folder that is not there is missing, and so is a link to nothing; one
// behind a folder the user may not search, directly, through a link or through more links than are followed, or under
// a loop of links, is unreadable — not known gone — and a folder is a folder.
func TestTheRemoteStatTellsAPathNotThereFromOneNotReadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root searches a folder whatever its mode")
	}
	here := t.TempDir()
	locked := filepath.Join(here, "locked")
	if err := os.MkdirAll(filepath.Join(locked, "api"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(locked, "No such file or directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })
	stat := func(p string) (FileInfo, error) {
		out, err := shell.Text(shell.Local{}, statScript(p), 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return readStat(p, out)
	}
	if info, err := stat(here); err != nil || info.Kind != "dir" {
		t.Fatalf("a folder: %+v %v", info, err)
	}
	if _, err := stat(filepath.Join(here, "gone")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a folder not there: %v", err)
	}
	if _, err := stat(filepath.Join(locked, "api")); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a folder behind one not searchable: %v", err)
	}
	if _, err := stat(filepath.Join(locked, "No such file or directory")); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a folder named as the error, behind one not searchable: %v", err)
	}
	link := filepath.Join(here, "link")
	if err := os.Symlink(filepath.Join(locked, "api"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := stat(link); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a link into a folder not searchable: %v", err)
	}
	chain := filepath.Join(locked, "api")
	for i := range 9 {
		next := filepath.Join(here, fmt.Sprintf("hop%d", i))
		if err := os.Symlink(chain, next); err != nil {
			t.Fatal(err)
		}
		chain = next
	}
	if _, err := stat(chain); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("links past the hops followed, into a folder not searchable: %v", err)
	}
	loopA, loopB := filepath.Join(here, "a"), filepath.Join(here, "b")
	if err := os.Symlink(loopB, loopA); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(loopA, loopB); err != nil {
		t.Fatal(err)
	}
	if _, err := stat(filepath.Join(loopA, "api")); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a folder under a loop of links: %v", err)
	}
	dangling := filepath.Join(here, "dangling")
	if err := os.Symlink(filepath.Join(here, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := stat(dangling); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a link to nothing: %v", err)
	}
}
