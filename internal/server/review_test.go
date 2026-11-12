package server

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/review"
)

func TestTheSecretCheckAnswersThePersonAlone(t *testing.T) {
	for _, key := range []auth.Key{{Scope: auth.ScopeTasks}, {Scope: auth.ScopeViewer}} {
		if key.Allows("GET", "/api/review/secrets") {
			t.Errorf("a %s key reaches the secret check", key.Scope)
		}
	}
	if auth.ViewerAllows("GET", "/api/review/secrets") {
		t.Error("a viewer's device reaches the secret check")
	}
}

func TestTheSecretCheckReadsTheFolder(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	empty := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", empty)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(empty, "gitconfig"))
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "-q")
	const shuffled = "Zq8vK2mWp4Lr7Tb9Xs3Nd6Hc1J" // a key made when the test runs, its letters in another order
	var key strings.Builder
	for i := range len(shuffled) {
		key.WriteByte(shuffled[i*7%len(shuffled)])
	}
	_ = os.WriteFile(filepath.Join(dir, "deploy.env"), []byte("REGION=eu\nAPI_TOKEN="+key.String()+"\n"), 0o644)

	s := &Server{Machines: machines.NewManager(t.TempDir(), "/nowhere.sock", "herdr", nil), Review: review.New(t.TempDir())}
	mux := http.NewServeMux()
	s.reviewRoutes(mux)
	code, out := taskCall(mux, "GET", "/api/review/secrets?machine=local&dir="+dir, "", "")
	findings, _ := out["findings"].([]any)
	if code != 200 || len(findings) != 1 || out["files"] != float64(1) {
		t.Fatalf("%d %v", code, out)
	}
	finding := findings[0].(map[string]any)
	if finding["file"] != "deploy.env" || finding["line"] != float64(2) || finding["rule"] != "assigned-secret" || strings.Contains(finding["excerpt"].(string), key.String()[:8]) {
		t.Errorf("finding: %v", finding)
	}
	if code, _ := taskCall(mux, "GET", "/api/review/secrets?machine=local&dir=relative", "", ""); code != 400 {
		t.Errorf("a relative folder: %d", code)
	}
	if code, _ := taskCall(mux, "GET", "/api/review/secrets?machine=nowhere&dir="+dir, "", ""); code != 404 {
		t.Errorf("a machine the fleet does not know: %d", code)
	}
}
