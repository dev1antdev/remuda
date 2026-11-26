package machines

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestARealPathScriptResolvesTheFolderAndRefusesALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script is POSIX")
	}
	dir := t.TempDir()
	real, _ := filepath.EvalSymlinks(dir)
	if err := os.MkdirAll(filepath.Join(real, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "real", "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, "real"), filepath.Join(real, "link")); err != nil {
		t.Fatal(err)
	}
	out, err := shell.Text(shell.Local{}, realPathScript(filepath.Join(real, "link", "file")), 5*time.Second)
	if err != nil || strings.TrimSpace(out) != filepath.Join(real, "real", "file") {
		t.Fatalf("through a folder link: %q, %v", out, err)
	}
	if err := os.Symlink(filepath.Join(real, "real", "file"), filepath.Join(real, "filelink")); err != nil {
		t.Fatal(err)
	}
	if out, err := shell.Text(shell.Local{}, realPathScript(filepath.Join(real, "filelink")), 5*time.Second); err == nil {
		t.Fatalf("a final link was read as %q", out)
	}
}
