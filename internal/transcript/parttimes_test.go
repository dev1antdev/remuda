package transcript

import (
	"strings"
	"testing"
)

// An answer's texts and thoughts say when they were written, as its calls do, so the page can tell a long thought from
// a pause in which nothing was written.
func TestAnAnswersTextsAndThoughtsCarryTheirRecordsTime(t *testing.T) {
	lines := []string{
		`{"type":"user","uuid":"u1","timestamp":"2026-11-13T10:00:00.000Z","message":{"role":"user","content":"look"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-11-13T10:00:04.000Z","message":{"id":"m1","content":[{"type":"thinking","thinking":"where to start"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-11-13T10:00:06.000Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/x"}}]}}`,
		`{"type":"user","uuid":"u2","timestamp":"2026-11-13T10:00:07.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
		`{"type":"assistant","uuid":"a3","timestamp":"2026-11-13T10:02:30.000Z","message":{"id":"m2","content":[{"type":"text","text":"Read it."}]}}`,
	}
	turns, err := ParseBytes([]byte(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || len(turns[1].Parts) != 3 {
		t.Fatalf("turns %+v", turns)
	}
	want := []string{"2026-11-13T10:00:04.000Z", "2026-11-13T10:00:06.000Z", "2026-11-13T10:02:30.000Z"}
	for i, part := range turns[1].Parts {
		if part.At != want[i] {
			t.Errorf("part %d (%s) at %q, want %q", i, part.Type, part.At, want[i])
		}
	}
	if turns[0].Parts[0].At != "" {
		t.Errorf("a prompt's time is its turn's: %+v", turns[0].Parts[0])
	}
}
