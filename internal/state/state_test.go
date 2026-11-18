package state

import (
	"path/filepath"
	"testing"
)

// TestARelativeStateDirIsMadeAbsolute: what is kept out of a viewer's reach is compared with absolute paths.
func TestARelativeStateDirIsMadeAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("REMUDA_STATE_DIR", "here/state")
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(dir) || filepath.Base(dir) != "state" {
		t.Fatalf("the state dir is %q", dir)
	}
}
