package watch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/core/coretest"
	"github.com/dev1antdev/remuda/internal/machines"
)

// TestATriggerFiresOnceOnRemudaCore is the same on a remuda-core of the test's own, reached as a remuda with -core
// reaches its machine: through `remuda-core connect` under this process's environment, set to the throwaway core's.
func TestATriggerFiresOnceOnRemudaCore(t *testing.T) {
	c := coretest.Server(t)
	for _, kv := range c.Env {
		name, value, _ := strings.Cut(kv, "=")
		t.Setenv(name, value)
	}
	t.Setenv("PATH", filepath.Dir(c.Bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	fleet := machines.NewCoreManager(t.TempDir(), "", nil)
	t.Cleanup(fleet.Close)
	firesOnce(t, fleet)
}
