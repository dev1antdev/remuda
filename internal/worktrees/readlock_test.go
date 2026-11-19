package worktrees

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestLookingAtAWorktreeLeavesTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		if _, err := shell.Text(shell.Local{Name: "local"}, InspectScript(dir), 30*time.Second); err != nil {
			t.Fatal(err)
		}
	})
}
