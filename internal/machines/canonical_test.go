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
	if got, ok := mc.Canonical(link); !ok || got != physical {
		t.Fatalf("a symlinked folder is named %q (resolved %v), not %q", got, ok, physical)
	}
	if got, ok := mc.Canonical(link + "/./"); !ok || got != physical {
		t.Fatalf("a folder with a trailing . is named %q (resolved %v), not %q", got, ok, physical)
	}
	if got, ok := mc.Canonical("/no/such/folder/."); ok || got != "/no/such/folder/." {
		t.Fatalf("a folder that is not there is kept as given (resolved %v): %q", ok, got)
	}
	if got, ok := mc.Canonical(""); ok || got != "" {
		t.Fatalf("no folder is named %q (resolved %v)", got, ok)
	}
}
