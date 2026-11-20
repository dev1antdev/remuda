package watch_test

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
	"github.com/dev1antdev/remuda/internal/machines"
)

// TestATriggerFiresOnceThroughNewStreams is firesOnce on a herdr of the test's own.
func TestATriggerFiresOnceThroughNewStreams(t *testing.T) {
	sock := herdrtest.Server(t)
	firesOnce(t, machines.NewManager(t.TempDir(), sock, "", nil))
}
