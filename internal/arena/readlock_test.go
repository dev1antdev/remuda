package arena

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestALanesStatsAndPatchLeaveTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		local := shell.Local{Name: "local"}
		head, err := shell.Text(local, HeadScript(dir), 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		base := strings.TrimSpace(head)
		for _, read := range []string{StatsScript([]Lane{{ID: 1, Dir: dir, Base: base}}), PatchScript(dir, base)} {
			if _, err := shell.Text(local, read, 30*time.Second); err != nil {
				t.Fatal(err)
			}
		}
	})
}
