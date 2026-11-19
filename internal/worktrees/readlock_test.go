package worktrees

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestLookingAtAWorktreeLeavesTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		for _, read := range []string{InspectScript(dir), LookScript(dir)} {
			if _, err := shell.Text(shell.Local{Name: "local"}, read, 30*time.Second); err != nil {
				t.Fatal(err)
			}
		}
	})
}
