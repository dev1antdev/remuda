package filetree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsTheTreeListsAWindowsFolderWithItsMarks reads a checkout on the runner over its OpenSSH, Git Bash listing
// the folder and git reading its status, the folder named in Windows' spelling.
func TestOnWindowsTheTreeListsAWindowsFolderWithItsMarks(t *testing.T) {
	wintest.Live(t)
	host := wintest.Host(t)
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@remuda.invalid", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	files := map[string]string{`src\App.xaml.cs`: "class App {}\n", `src\Form1.cs`: "class Form1 {}\n", `.gitignore`: "bin/\n"}
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "src", "Form1.cs"), []byte("class Form1 { }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "app.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	var svc Service
	root, err := svc.List(host, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	got := byName(root.Entries)
	if !root.Repo || !got["src"].Changes || !got["bin"].Ignored || names(root.Entries) != "bin src .gitignore" {
		t.Fatalf("the root reads %+v", root)
	}
	src, err := svc.List(host, dir, "src")
	if err != nil {
		t.Fatal(err)
	}
	if inSrc := byName(src.Entries); inSrc["Form1.cs"].Mark != "M" || inSrc["App.xaml.cs"].Mark != "" {
		t.Fatalf("src reads %+v", src.Entries)
	}
}
