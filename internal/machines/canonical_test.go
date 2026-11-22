package machines

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCanonicalIsTheMachinesOwnSpellingOfAFolder: a symlinked folder is named as the machine names it, so a trigger,
// a repository's settings and the documents on the core meet the roster's own sessions; a folder that is not there is
// kept as given, cleaned.
func TestCanonicalIsTheMachinesOwnSpellingOfAFolder(t *testing.T) {
	mc, err := newTestManager(t.TempDir()).Get(LocalID)
	if err != nil {
		t.Fatal(err)
	}
	real := t.TempDir()
	physical, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if got := mc.Canonical(link); got != physical {
		t.Fatalf("a symlinked folder is named %q, not %q", got, physical)
	}
	if got := mc.Canonical(link + "/./"); got != physical {
		t.Fatalf("a folder with a trailing . is named %q, not %q", got, physical)
	}
	if got := mc.Canonical("/no/such/folder/."); got != "/no/such/folder" {
		t.Fatalf("a folder that is not there is kept cleaned: %q", got)
	}
	if got := mc.Canonical(""); got != "" {
		t.Fatalf("no folder is named %q", got)
	}
}
