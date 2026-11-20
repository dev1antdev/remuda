package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/schedule"
)

// A schedule the store could not delete on disk is not said to be gone: the route answers the failure.
func TestADeleteThatCouldNotBeSavedIsAFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schedules.json"), []byte(`[{"id":"a","enabled":"true"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{Schedules: schedule.New(dir, nil, nil)}
	api := http.NewServeMux()
	s.scheduleRoutes(api)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/schedules/a", nil))
	if rec.Code == http.StatusOK || !strings.Contains(rec.Body.String(), "unreadable") {
		t.Fatalf("a delete the store could not save: %d %s", rec.Code, rec.Body)
	}
}
