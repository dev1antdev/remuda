package server

import (
	"net/http"
	"testing"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/jobs"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/updates"
)

// A remuda on remuda-core (-core) neither lists nor removes leftover worktrees, nor moves work onto a core machine,
// before any script reaches the machine: herdr's agents there, which it does not see, may hold the checkouts. A new
// worktree is the core adapter's to make (Core 20), as herdr's worktree.create made one beside herdr's agents.
func TestNoCheckoutIsTakenDownOrWrittenOnAMachineReachedThroughCore(t *testing.T) {
	fleet := machines.NewCoreManager(t.TempDir(), "", nil, true) // a page-only host: no core of its own is started
	defer fleet.Close()
	s := &Server{Machines: fleet, Updates: &updates.Service{Jobs: jobs.New(events.NewBus())}}
	mux := http.NewServeMux()
	s.leftoverRoutes(mux)
	s.teleportRoutes(mux)
	s.sessionRoutes(mux)
	if code, out := taskCall(mux, "GET", "/api/machines/local/leftovers", "", ""); code != http.StatusConflict {
		t.Errorf("leftovers listed on remuda-core: %d %v", code, out)
	}
	if code, out := taskCall(mux, "POST", "/api/machines/local/leftovers/remove", "", `{"paths":["/w/x"]}`); code != http.StatusConflict {
		t.Errorf("leftovers removed on remuda-core: %d %v", code, out)
	}
	if code, out := taskCall(mux, "POST", "/api/machines/gone/leftovers/remove?machine=local", "", `{"paths":["/w/x"]}`); code != http.StatusConflict {
		t.Errorf("leftovers removed on remuda-core named by the query past an unknown path: %d %v", code, out)
	}
	move := `{"machine":"local","pane":"w1:p1","target":"local","folder":"/w/app","agent":"claude"}`
	if code, out := taskCall(mux, "POST", "/api/teleport", "", move); code != http.StatusBadRequest {
		t.Errorf("a move onto remuda-core: %d %v", code, out)
	}
}
