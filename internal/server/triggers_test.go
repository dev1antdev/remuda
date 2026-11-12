package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/triggers"
)

// The trigger routes: the person's rules replaced whole and read back with the built-in watches; a rule herdr could
// not watch, or one on a machine the fleet does not know, is refused with its reason.
func TestTriggerRoutes(t *testing.T) {
	dir := t.TempDir()
	resubscribed := 0
	svc := triggers.New(dir)
	svc.Resubscribe = func() { resubscribed++ }
	s := &Server{Triggers: svc, Machines: machines.NewManager(dir, dir+"/none.sock", "", nil)}
	mux := http.NewServeMux()
	s.triggerRoutes(mux)

	code, out := taskCall(mux, "PUT", "/api/triggers", "", `[{"name":"Deployed","match":"substring","pattern":"DEPLOYED","action":"push","enabled":true},
		{"name":"QA","machine":"local","dir":"/w/api","match":"regex","pattern":"^ready for qa$","action":"task","column":"Testing","enabled":true}]`)
	rules, _ := out["rules"].([]any)
	if code != 200 || len(rules) != 2 || resubscribed != 1 {
		t.Fatalf("put: %d %v, %d resubscriptions", code, out, resubscribed)
	}
	if id, _ := rules[0].(map[string]any)["id"].(string); id == "" {
		t.Fatalf("an id is made for a rule without one: %v", rules[0])
	}
	code, out = taskCall(mux, "GET", "/api/triggers", "", "")
	builtin, _ := out["builtin"].(map[string]any)
	if code != 200 || len(out["rules"].([]any)) != 2 || builtin["tests"] != triggers.TestsPattern || out["max"] != float64(triggers.MaxRules) {
		t.Fatalf("get: %d %v", code, out)
	}

	refused := map[string]string{
		"a look-ahead":     `[{"name":"x","match":"regex","pattern":"(?=x)","action":"push"}]`,
		"no column":        `[{"name":"x","match":"substring","pattern":"x","action":"task"}]`,
		"another machine":  `[{"name":"x","machine":"nope","match":"substring","pattern":"x","action":"push"}]`,
		"not a list":       `{"name":"x"}`,
		"an action of its": `[{"name":"x","match":"substring","pattern":"x","action":"shell"}]`,
	}
	for what, body := range refused {
		code, out := taskCall(mux, "PUT", "/api/triggers", "", body)
		if msg, _ := out["error"].(string); code != 400 || msg == "" {
			t.Errorf("%s: %d %v, want 400 with a reason", what, code, out)
		}
	}
	if got := svc.Rules.List(); len(got) != 2 {
		t.Fatalf("a refused list leaves the rules: %+v", got)
	}

	code, out = taskCall(mux, "POST", "/api/triggers/try", "", `{"machine":"local","rule":{"match":"substring","pattern":"x","action":"push"}}`)
	if msg, _ := out["error"].(string); code != 400 || !strings.Contains(msg, "pane") {
		t.Fatalf("a try without a pane: %d %v", code, out)
	}
}
