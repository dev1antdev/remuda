package quick

import (
	"encoding/json"
	"testing"
)

func TestEmptyListIsNotNull(t *testing.T) {
	st := New(t.TempDir())
	b, _ := json.Marshal(st.List())
	if string(b) != "[]" {
		t.Fatalf("got %s", b)
	}
}

func TestReplaceFillsLabelAndIDAndPersists(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	out, err := st.Replace([]Prompt{{Text: "  Run the tests and sum up what failed.\nThen stop.  "}, {Label: "Status", Text: "One line.", Dir: "/tmp/x"}})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Label != "Run the tests and sum up what failed." || out[0].ID == "" || out[1].Dir != "/tmp/x" {
		t.Fatalf("got %+v", out)
	}
	if _, err := st.Replace([]Prompt{{Label: "x", Text: "  "}}); err == nil {
		t.Fatal("an empty prompt was accepted")
	}
	again := New(dir).List()
	if len(again) != 2 || again[1].Label != "Status" {
		t.Fatalf("reloaded %+v", again)
	}
}

func TestIDsAreUniqueAndLabelsCutOnRunes(t *testing.T) {
	st := New(t.TempDir())
	out, err := st.Replace([]Prompt{{ID: "same", Text: "a"}, {ID: "same", Text: "b"}, {Text: "Прогони тесты и коротко скажи, что упало, по строке на падение"}})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].ID != "same" || out[1].ID == "same" || out[1].ID == "" || out[1].ID == out[2].ID {
		t.Fatalf("ids %q %q %q", out[0].ID, out[1].ID, out[2].ID)
	}
	if l := []rune(out[2].Label); len(l) != 41 || l[40] != '…' {
		t.Fatalf("label %q", out[2].Label)
	}
}
