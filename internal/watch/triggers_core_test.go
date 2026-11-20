package watch_test

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/machines"
)

// TestATriggerFiresOnceOnRemudaCore is firesOnce on a remuda-core of the test's own, reached as a remuda with -core
// reaches its machine: through `remuda-core connect` under the core's environment.
func TestATriggerFiresOnceOnRemudaCore(t *testing.T) {
	coretest.Server(t).Use(t)
	fleet := machines.NewCoreManager(t.TempDir(), "", nil)
	t.Cleanup(fleet.Close)
	firesOnce(t, fleet)
}
