package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/scenes"
)

// The scene routes: the list saved whole and checked, a machine the fleet lacks refused, a run of a scene that is not
// there or a tidy with nothing picked refused, a run that ended long ago not found.
func TestSceneRoutes(t *testing.T) {
	fleet := machines.NewManager(t.TempDir(), "/nowhere.sock", "herdr", nil)
	runner := &scenes.Service{Store: scenes.New(t.TempDir()), Machines: func() []scenes.Machine { return nil }}
	s := &Server{Machines: fleet, Scenes: runner}
	mux := http.NewServeMux()
	s.sceneRoutes(mux)

	morning := `[{"name":"Morning","steps":[{"machine":"local","dir":"/w/api","agent":"claude","model":"opus"}]},{"name":"Evening","kind":"tidy","tidy":{"shells":true}}]`
	code, _ := taskCall(mux, "PUT", "/api/scenes", "", morning)
	if code != 200 || len(runner.Store.List()) != 2 {
		t.Fatalf("saved: %d %v", code, runner.Store.List())
	}
	for name, body := range map[string]string{
		"a machine the fleet lacks":    `[{"name":"Morning","steps":[{"machine":"gog","dir":"/w/api","agent":"claude"}]}]`,
		"a tidy of a machine it lacks": `[{"name":"Evening","kind":"tidy","tidy":{"shells":true,"machines":["gog"]}}]`,
		"a step a recipe would refuse": `[{"name":"Morning","steps":[{"machine":"local","dir":"/w/api","agent":"shell","prompt":"hi"}]}]`,
	} {
		code, out := taskCall(mux, "PUT", "/api/scenes", "", body)
		if msg, _ := out["error"].(string); code != 400 || msg == "" {
			t.Errorf("%s: %d %v", name, code, out)
		}
	}
	if len(runner.Store.List()) != 2 {
		t.Fatal("a refused list replaced the saved one")
	}

	evening := runner.Store.List()[1].ID
	if code, _ := taskCall(mux, "POST", "/api/scenes/nope/run", "", `{}`); code != 404 {
		t.Fatalf("a scene that is not there: %d", code)
	}
	if code, out := taskCall(mux, "POST", "/api/scenes/"+evening+"/run", "", `{}`); code != 400 || !strings.Contains(out["error"].(string), "pick") {
		t.Fatalf("a tidy with nothing picked: %d %v", code, out)
	}
	if code, out := taskCall(mux, "GET", "/api/scenes/"+evening+"/plan", "", ""); code != 200 || out["actions"] == nil {
		t.Fatalf("a tidy's plan: %d %v", code, out)
	}
	if code, _ := taskCall(mux, "GET", "/api/scenes/"+runner.Store.List()[0].ID+"/plan", "", ""); code != 400 {
		t.Fatalf("a start scene has no plan: %d", code)
	}
	if code, _ := taskCall(mux, "GET", "/api/scene-runs/nope", "", ""); code != 404 {
		t.Fatalf("a run that is not there: %d", code)
	}
	if code, _ := taskCall(mux, "POST", "/api/scenes/capture", "", `{"machine":"local"}`); code != 400 {
		t.Fatalf("a capture without a workspace: %d", code)
	}
}
