package machines

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

// A file's changes in the file viewer are a read, a file only touched since the index last saw it among them.
func TestTheFileViewersDiffLeavesTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		edited, touched := filepath.Join(dir, "edited.txt"), filepath.Join(dir, "touched.txt")
		out, err := shell.Text(shell.Local{}, gitDiffScript(dir, script.Quote(edited)), 20*time.Second)
		if err != nil || !strings.Contains(out, "+two") {
			t.Fatalf("the diff: %v %q", err, out)
		}
		out, err = shell.Text(shell.Local{}, gitDiffScript(dir, script.Quote(touched)), 20*time.Second)
		if _, rest, _ := strings.Cut(out, "\n"); err != nil || strings.TrimSpace(rest) != "tracked" {
			t.Fatalf("the diff of a file only touched: %v %q", err, out)
		}
	})
}
