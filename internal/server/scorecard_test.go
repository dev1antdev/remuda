package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev1antdev/remuda/internal/auth"
	"github.com/dev1antdev/remuda/internal/scorecard"
	"github.com/dev1antdev/remuda/internal/tasks"
)

// The scorecard route: the person's card names the task to try a model on, a viewer's names none.
func TestScorecardRoute(t *testing.T) {
	taskStore, err := tasks.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer taskStore.Close()
	projects, _ := taskStore.Projects()
	title := "Fix the login page"
	task, _ := taskStore.Create(projects[0].ID, tasks.Patch{Title: &title})
	run, _ := taskStore.StartRun(tasks.Run{TaskID: task.ID, Machine: "studio", Workspace: "w1", Pane: "w1:p1", Dir: "/w/task-1", Agent: "claude"})
	_ = taskStore.SetRun(run.ID, tasks.RunFinished, "rec-1")
	_ = taskStore.SetRunUsage(run.ID, 1, 10, "Opus 4.5", 60)
	s := &Server{Scorecard: &scorecard.Service{Tasks: taskStore}}
	mux := http.NewServeMux()
	s.scorecardRoutes(mux)

	read := func(viewer bool) scorecard.Card {
		req := httptest.NewRequest("GET", "/api/scorecard?days=7&tz=Europe/Berlin", nil)
		if viewer {
			req = req.WithContext(auth.WithViewer(req.Context()))
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var card scorecard.Card
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &card) != nil {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
		return card
	}
	card := read(false)
	if card.Days != 7 || card.Since.Format("15:04 -07:00") != "00:00 +01:00" || len(card.Rows) != 1 || card.Rows[0].Try == nil || card.Rows[0].Try.Title != title {
		t.Fatalf("the person's card: %+v", card)
	}
	if viewer := read(true); len(viewer.Rows) != 1 || viewer.Rows[0].Try != nil || viewer.Rows[0].Runs != 1 {
		t.Fatalf("a viewer's card: %+v", viewer)
	}
}
