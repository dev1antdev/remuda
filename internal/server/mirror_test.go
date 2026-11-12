package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/mirror"
)

func TestTheMirrorRoutesReadAndSaveTheSwitches(t *testing.T) {
	s := &Server{Mirror: mirror.New(t.TempDir(), mirror.Inputs{}, "")}
	rec := httptest.NewRecorder()
	s.handleMirror(rec, httptest.NewRequest(http.MethodGet, "/api/mirror", nil))
	var st mirror.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || !st.Figures || st.Toasts || !strings.Contains(st.Snippet, "$remuda_cost") {
		t.Fatalf("the figures go into herdr unless turned off, the toasts do not unless turned on: %v %s", err, rec.Body)
	}

	rec = httptest.NewRecorder()
	s.handleMirrorPut(rec, httptest.NewRequest(http.MethodPut, "/api/mirror", strings.NewReader(`{"figures":false,"toasts":true,"quiet":{"from":"22:00","to":"07:00","tz":"Europe/Berlin"}}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"toasts":true`) || !strings.Contains(rec.Body.String(), `"figures":false`) {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	if got := s.Mirror.Settings(); got.Quiet == nil || got.Quiet.TZ != "Europe/Berlin" {
		t.Fatalf("the quiet hours are not kept: %+v", got)
	}

	rec = httptest.NewRecorder()
	s.handleMirrorTest(rec, httptest.NewRequest(http.MethodPost, "/api/mirror/test", nil))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("a test with no machine connected: %d %s", rec.Code, rec.Body)
	}

	s.Mirror = mirror.New(t.TempDir(), mirror.Inputs{}, "a scratch instance")
	rec = httptest.NewRecorder()
	s.handleMirrorTest(rec, httptest.NewRequest(http.MethodPost, "/api/mirror/test", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("a scratch instance pops a test toast: %d %s", rec.Code, rec.Body)
	}
}
