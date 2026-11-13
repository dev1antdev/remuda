package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/usage"
)

// The year's days: every machine of the fleet, the days cut in the viewer's zone; the person's alone.
func TestUsageDaysRoute(t *testing.T) {
	fleet := machines.NewManager(t.TempDir(), "/nowhere.sock", "herdr", nil)
	book := usage.OpenBook(t.TempDir())
	book.Read = func(shell.Host, time.Time, *time.Location, string) (usage.Report, error) { return usage.Report{}, nil }
	s := &Server{Machines: fleet, UsageDays: book}
	mux := http.NewServeMux()
	s.usageRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/usage/days?tz=Europe/Kyiv", nil))
	book.Wait()
	var year usage.Year
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &year) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if year.Zone != "Europe/Kyiv" || len(year.Machines) != 1 || year.Machines[0].ID != "local" || year.To < year.From {
		t.Fatalf("year %+v", year)
	}
	if auth.ViewerAllows("GET", "/api/usage/days") {
		t.Fatal("a viewer link reads the year: it stays as narrow as it was")
	}
}
