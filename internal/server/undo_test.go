package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/undo"
)

func TestTheUndoRoutesListPlanAndRefuse(t *testing.T) {
	dir := t.TempDir()
	store := undo.OpenStore(dir)
	turn := undo.Turn{ID: undo.NewID(time.Now()), Machine: machines.LocalID, Dir: "/repo/app", Pane: "w1:p1", Ended: time.Now(), Files: []undo.File{{Path: "a.go"}, {Path: "b.go"}}, Why: "the turn's changes weigh 20.0 MB, past the 16.0 MB an undo keeps"}
	if err := store.Add(turn, "", ""); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Machines: machines.NewManager(dir, filepath.Join(dir, "no-herdr.sock"), "herdr", nil),
		Undo: &undo.Service{Store: store, Tracker: undo.NewTracker(store, undo.Inputs{}),
			Host: func(string) (undo.Host, error) { return nil, errors.New("not reached") }},
	}
	api := http.NewServeMux()
	s.undoRoutes(api)

	rec := ask(api, "GET", "/api/undo?machine=local&dir=/repo/app", "")
	var listed struct {
		Turns     []undo.Listed `json:"turns"`
		Recording bool          `json:"recording"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil || rec.Code != 200 || len(listed.Turns) != 1 || listed.Turns[0].Files != 2 || !listed.Recording {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "GET", "/api/undo?machine=local&dir=repo", ""); rec.Code != 400 {
		t.Fatalf("a relative folder: %d", rec.Code)
	}
	rec = ask(api, "GET", "/api/undo/"+turn.ID+"/plan?machine=local", "")
	var plan undo.Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil || rec.Code != 200 || plan.Refused == nil || !strings.Contains(plan.Refused.Why, "20.0 MB") {
		t.Fatalf("plan: %d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "POST", "/api/undo/"+turn.ID, `{"machine":"local"}`); rec.Code != 409 || !strings.Contains(rec.Body.String(), "20.0 MB") {
		t.Fatalf("a turn that cannot be undone: %d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "POST", "/api/undo/20261112T000000Z-abcdef", `{"machine":"local"}`); rec.Code != 404 {
		t.Fatalf("an unknown turn: %d %s", rec.Code, rec.Body)
	}
	if rec := ask(api, "GET", "/api/undo/"+turn.ID+"/plan?machine=elsewhere", ""); rec.Code != 404 {
		t.Fatalf("another machine's turn: %d %s", rec.Code, rec.Body)
	}
	var _ shell.Host = (*machines.Machine)(nil) // a fleet machine is what an undo runs on
	var _ undo.Host = (*machines.Machine)(nil)
}
