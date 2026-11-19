package teleport

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestLookingAtACheckoutLeavesTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		source, err := Inspect(shell.Local{Name: "local"}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if source.Unstaged != 1 {
			t.Errorf("a file only touched counts as changed: %+v", source)
		}
		if _, err := shell.Text(shell.Local{Name: "local"}, packScript(dir, false)+" >/dev/null", 30*time.Second); err != nil {
			t.Fatal(err)
		}
	})
}
