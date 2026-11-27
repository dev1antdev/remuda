package undo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestACheckpointLeavesTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		read := readScript(dir, readOptions{patch: forwardPatch, staged: true})
		if _, err := shell.Text(shell.Local{Name: "local"}, read, 30*time.Second); err != nil {
			t.Fatal(err)
		}
	})
}

func TestABigUntrackedFileIsNamedNotCopied(t *testing.T) {
	dir := repo(t, map[string]string{"a.txt": "one\n"})
	big := make([]byte, maxCheckpointFile+1024)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(dir, "dataset.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "small.txt", "kept\n")
	got, err := read(local{shell.Local{Name: "local"}}, dir, readOptions{patch: forwardPatch})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Skipped) != 1 || got.Skipped[0] != "dataset.bin" {
		t.Fatalf("skipped: %v", got.Skipped)
	}
	if _, ok := got.Files["dataset.bin"]; ok {
		t.Fatal("a file past the checkpoint's size was hashed and staged")
	}
	if _, ok := got.Files["small.txt"]; !ok {
		t.Fatalf("the small file was left out too: %v", got.Files)
	}
}
