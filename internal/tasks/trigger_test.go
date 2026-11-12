package tasks

import (
	"strings"
	"testing"
)

func TestATriggerMovesTheTaskWhoseAgentRunsInThePane(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	tk, _ := st.Create(inbox[0].ID, Patch{Title: str("Ship it")})
	if _, err := st.StartRun(Run{TaskID: tk.ID, Machine: "studio", Workspace: "wX", Pane: "wX:p1", Dir: "/w/x", Branch: "task-1-ship-it", Agent: "claude"}); err != nil {
		t.Fatal(err)
	}
	if moved, err := st.OnTrigger("studio", "wX:p9", "Testing", "trigger “QA”"); moved != nil || err != nil {
		t.Fatalf("a pane without a run: %+v %v", moved, err)
	}
	if moved, err := st.OnTrigger("studio", "wX:p1", "No such column", "trigger “QA”"); moved != nil || err != nil {
		t.Fatalf("a column the project lacks: %+v %v", moved, err)
	}
	moved, err := st.OnTrigger("studio", "wX:p1", "testing", "trigger “QA”: the pane printed ready for qa")
	if err != nil || moved == nil || moved.To.Title != "Testing" || moved.Task.StatusID != moved.To.ID {
		t.Fatalf("moved: %+v %v", moved, err)
	}
	cs, _ := st.Comments(tk.ID)
	if len(cs) != 1 || cs[0].Author != "remuda" || !strings.HasPrefix(cs[0].Body, "Moved to Testing by trigger “QA”") {
		t.Fatalf("comment: %+v", cs)
	}
	if again, _ := st.OnTrigger("studio", "wX:p1", "Testing", "trigger “QA”"); again != nil {
		t.Fatal("a task already there stays without a word")
	}

	project, _ := st.Project(inbox[0].ID)
	statuses := append([]Status(nil), project.Statuses...)
	statuses = append(statuses, Status{Title: "Queued", Kind: KindOpen, OnQueue: true})
	if _, err := st.SetStatuses(inbox[0].ID, statuses, nil); err != nil {
		t.Fatal(err)
	}
	if moved, _ := st.OnTrigger("studio", "wX:p1", "Queued", "trigger “QA”"); moved != nil {
		t.Fatal("the Queued column takes only the person's own moves")
	}
}
