package collisions

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestTheRadarLeavesTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		if _, err := shell.Text(shell.Local{Name: "local"}, Script([]string{dir}), 30*time.Second); err != nil {
			t.Fatal(err)
		}
	})
}
