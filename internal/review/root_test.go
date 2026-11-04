package review

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestASubmoduleAndASubfolderAreTheirOwnCheckouts(t *testing.T) {
	lib, libGit, libWrite := scratchCheckout(t)
	libWrite("lib.txt", "lib")
	libGit("add", "-A")
	libGit("commit", "-qm", "lib")
	dir, git, write := scratchCheckout(t)
	write("a.txt", "a")
	git("add", "-A")
	git("commit", "-qm", "init")
	git("-c", "protocol.file.allow=always", "submodule", "add", "-q", lib, "vendor/lib")
	git("commit", "-qm", "the submodule")
	web := filepath.Join(dir, "web")
	if err := os.Mkdir(web, 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "vendor", "lib")

	store := New(t.TempDir())
	store.Badges(shell.Local{Name: "local"}, []string{sub, web})
	if root := store.Root("local", sub); realPath(t, root) != realPath(t, sub) {
		t.Fatalf("a submodule's folder is %q, want the submodule itself %q", root, sub)
	}
	if root := store.Root("local", web); root != dir {
		t.Fatalf("a subfolder's repository is %q, want %q", root, dir)
	}
}
