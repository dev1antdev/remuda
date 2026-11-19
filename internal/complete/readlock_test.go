package complete

import (
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script/readtest"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestListingAFoldersFilesLeavesTheIndexAlone(t *testing.T) {
	readtest.LeavesIndex(t, func(dir string) {
		if _, err := shell.Text(shell.Local{Name: "local"}, pathsScript(dir), 30*time.Second); err != nil {
			t.Fatal(err)
		}
	})
}
