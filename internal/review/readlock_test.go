package review

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestTheLensReadsLeaveTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		local := shell.Local{Name: "local"}
		head, err := shell.Text(local, script.InDir(dir)+"git rev-parse HEAD", 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		store := New(t.TempDir())
		summary := store.Summary(local, dir)
		if len(summary.Files) != 2 {
			t.Fatalf("the summary lists what changed and not what was only touched: %+v", summary.Files)
		}
		if _, blobs, err := Snapshot(local, dir); err != nil || len(blobs) != 2 || blobs["edited.txt"] == "" {
			t.Fatalf("the uncommitted blobs: %v %v", err, blobs)
		}
		store.Badges(local, []string{dir})
		if _, err := shell.Text(local, script.InDir(dir)+statusScript, 30*time.Second); err != nil {
			t.Fatal(err)
		}
		for _, side := range []string{SideUnstaged, SideStaged} {
			if _, err := store.DiffSide(local, dir, "edited.txt", side); err != nil {
				t.Fatal(err)
			}
		}
		if diff, err := BaseDiff(local, dir, strings.TrimSpace(head), "edited.txt"); err != nil || !strings.Contains(diff, "+two") {
			t.Fatalf("the branch's diff: %v %q", err, diff)
		}
		if _, err := ScanWork(local, dir, true); err != nil {
			t.Fatal(err)
		}
	})
}
