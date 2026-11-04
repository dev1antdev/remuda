package tasks

import (
	"strings"
	"testing"
)

func TestImportFromMakesTheTaskOnceWithItsBodyAndLabels(t *testing.T) {
	st := open(t)
	inbox, _ := st.Projects()
	in := Imported{
		Title:    "CRMOPS-6279 Conflicts with the base branch",
		URL:      "https://roofr.atlassian.net/browse/CRMOPS-6279",
		Lead:     "CRMOPS-6279 · Bug · In Review · by Ann <script>",
		Markdown: "Steps:\n\n- open the job\n- **save** it",
		Labels:   []string{"backend", ""},
	}
	task, err := ImportFrom(st, inbox[0].ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != in.Title {
		t.Fatalf("title %q", task.Title)
	}
	if !strings.HasPrefix(task.Description, `<p>Issue: <a href="https://roofr.atlassian.net/browse/CRMOPS-6279">`) || strings.Contains(task.Description, "<script>") || !strings.Contains(task.Description, "<strong>save</strong>") {
		t.Fatalf("description %q", task.Description)
	}
	if len(task.Labels) != 1 || task.Labels[0].Title != "backend" {
		t.Fatalf("labels %+v", task.Labels)
	}
	if _, err := ImportFrom(st, inbox[0].ID, in); err == nil || !strings.Contains(err.Error(), "already task") {
		t.Fatalf("a second import of the same issue: %v", err)
	}
	if _, err := ImportFrom(st, inbox[0].ID, Imported{Title: "x"}); err == nil {
		t.Fatal("an issue without an address was made a task")
	}
}
