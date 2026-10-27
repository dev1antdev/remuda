package recipes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEmptyListIsNotNull(t *testing.T) {
	st := New(t.TempDir())
	b, _ := json.Marshal(st.List())
	if string(b) != "[]" {
		t.Fatalf("got %s", b)
	}
}

func TestReplaceTrimsNamesAndPersists(t *testing.T) {
	dir := t.TempDir()
	st := New(dir)
	out, err := st.Replace([]Recipe{
		{Name: "  Planner ", Agent: "claude", Model: " opus ", Args: `--permission-mode plan`, Prompt: " Review the diff. "},
		{ID: "same", Name: "Scout", Agent: "codex", Prompt: "Look around.", Dir: "/home/me/app", Machine: "gog"},
		{ID: "same", Name: "Shell", Agent: "shell"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Name != "Planner" || out[0].Model != "opus" || out[0].Prompt != "Review the diff." || out[0].ID == "" {
		t.Fatalf("got %+v", out[0])
	}
	if out[1].ID != "same" || out[2].ID == "same" || out[2].ID == "" {
		t.Fatalf("ids %q %q %q", out[0].ID, out[1].ID, out[2].ID)
	}
	again := New(dir).List()
	if len(again) != 3 || again[1].Machine != "gog" || again[1].Dir != "/home/me/app" {
		t.Fatalf("reloaded %+v", again)
	}
}

func TestCheckRefusesWhatTheCreateRoutesRefuse(t *testing.T) {
	bad := map[string]Recipe{
		"no name":                {Agent: "claude"},
		"no agent":               {Name: "x"},
		"unknown agent":          {Name: "x", Agent: "clippy"},
		"shell with a prompt":    {Name: "x", Agent: "shell", Prompt: "hi"},
		"shell with a model":     {Name: "x", Agent: "shell", Model: "opus"},
		"unclosed quote":         {Name: "x", Agent: "claude", Args: `--foo "bar`},
		"control character":      {Name: "x", Agent: "claude", Args: "a\x01b"},
		"machine without folder": {Name: "x", Agent: "claude", Machine: "gog"},
	}
	for what, r := range bad {
		if _, err := Check(r); err == nil {
			t.Errorf("%s: accepted %+v", what, r)
		}
	}
	good := []Recipe{
		{Name: "x", Agent: "claude", Model: "opus", Args: `--permission-mode plan -p "two words"`},
		{Name: "x", Agent: "shell"},
		{Name: "x", Agent: "kimi", Model: "kimi-k2-thinking", Dir: "/x"},
	}
	for _, r := range good {
		if _, err := Check(r); err != nil {
			t.Errorf("refused %+v: %v", r, err)
		}
	}
	_, err := Check(Recipe{Name: "Nope", Agent: "shell", Args: "-x"})
	if err == nil || !strings.Contains(err.Error(), `"Nope"`) {
		t.Errorf("the error names the recipe: %v", err)
	}
}
