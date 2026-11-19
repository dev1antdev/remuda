package undo

import (
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
