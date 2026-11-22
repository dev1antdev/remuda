package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/herdr/herdrfake"
	"github.com/dev1antdev/remuda/internal/triggers"
)

// The trigger routes: the person's rules replaced whole and read back with the built-in watches; a rule herdr could
// not watch, or one on a machine the fleet does not know, is refused with its reason.
func TestTriggerRoutes(t *testing.T) {
	dir := t.TempDir()
	resubscribed := 0
	svc := triggers.New(dir)
	svc.Resubscribe = func() { resubscribed++ }
	s := &Server{Triggers: svc, Machines: herdrfake.Manager(dir)}
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

// TestARuleSavedOnASymlinkedFolderIsKeptAsTheMachineNamesIt: a folder is one spelling in the rules, the one the panes
// of that folder are named in, so a rule saved through a symlink still covers them.
func TestARuleSavedOnASymlinkedFolderIsKeptAsTheMachineNamesIt(t *testing.T) {
	dir := t.TempDir()
	s := &Server{Triggers: triggers.New(dir), Machines: herdrfake.Manager(dir)}
	mux := http.NewServeMux()
	s.triggerRoutes(mux)
	real := t.TempDir()
	physical, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`[{"name":"QA","machine":"local","dir":%q,"match":"regex","pattern":"^ready for qa$","action":"push","enabled":true}]`, link)
	code, out := taskCall(mux, "PUT", "/api/triggers", "", body)
	rules, _ := out["rules"].([]any)
	if code != 200 || len(rules) != 1 {
		t.Fatalf("put: %d %v", code, out)
	}
	if got, _ := rules[0].(map[string]any)["dir"].(string); got != physical {
		t.Fatalf("the rule's folder is %q, not the machine's %q", got, physical)
	}
}
