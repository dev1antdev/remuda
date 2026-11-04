package teleport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMoveLeavesNothingBehindUnderATempFolderWithASpace(t *testing.T) {
	needGit(t)
	temp := filepath.Join(t.TempDir(), "temp dir")
	if err := os.Mkdir(temp, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", temp)
	_, source, target := clones(t)
	working(t, source)
	from, to := newMachine(t, "mac", "This Mac"), newMachine(t, "gog", "GOG")
	var lines []string
	_, err := Move{Source: from, SourceDir: source, Target: to, TargetDir: target, Start: func(Landed, Source) error { return nil }}.Run(func(line string) { lines = append(lines, line) })
	if err != nil {
		t.Fatalf("move: %v\n%s", err, strings.Join(lines, "\n"))
	}
	if left, _ := filepath.Glob(filepath.Join(temp, "remuda-teleport.*")); len(left) > 0 {
		t.Fatalf("the pack or its unpacking stayed: %v\n%s", left, strings.Join(lines, "\n"))
	}

	pack := filepath.Join(temp, "remuda-teleport.pack")
	if err := os.WriteFile(pack, []byte("work"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Land(to, filepath.Join(temp, "nowhere"), pack, filepath.Join(to.home, "w"), "from-mac", func(string) {}); err == nil {
		t.Fatal("landed in a folder that is not there")
	}
	if _, err := os.Stat(pack); !os.IsNotExist(err) {
		t.Fatalf("a landing refused early left the pack on the machine: %v", err)
	}
}
