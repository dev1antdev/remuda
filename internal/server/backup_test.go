package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/backup"
	"github.com/dev1antdev/remuda/internal/machines"
)

func putBackups(s *Server, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.handleBackupsPut(rec, httptest.NewRequest(http.MethodPut, "/api/backups", strings.NewReader(body)))
	return rec
}

func TestAServerKeepsNoCopiesOnItsOwnDisk(t *testing.T) {
	s := serverModeServer(t)
	s.Backup = backup.Open(t.TempDir(), "rp-orange", "v-test")
	if rec := putBackups(s, `{"machine":"`+machines.LocalID+`","at":"03:30","keep":14}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "no machine of the fleet") {
		t.Fatalf("the server's own host taken as the target: %d %s", rec.Code, rec.Body)
	}
	if rec := putBackups(s, `{"machine":"","at":"03:30","keep":14}`); rec.Code != http.StatusOK {
		t.Fatalf("no target refused: %d %s", rec.Code, rec.Body)
	}
	s.Machines.SetServer(false)
	if rec := putBackups(s, `{"machine":"`+machines.LocalID+`","at":"03:30","keep":14}`); rec.Code != http.StatusOK {
		t.Fatalf("a cockpit's own machine refused: %d %s", rec.Code, rec.Body)
	}
	if rec := putBackups(s, `{"machine":"nowhere","at":"03:30","keep":14}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown machine taken: %d %s", rec.Code, rec.Body)
	}
}
