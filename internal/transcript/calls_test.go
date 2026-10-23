package transcript

import (
	"math"
	"testing"
)

func TestToolPartsSayWhenTheyRanAndHowLong(t *testing.T) {
	data := []byte(`{"type":"assistant","uuid":"a1","timestamp":"2026-10-24T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go test ./..."}},{"type":"tool_use","id":"toolu_2","name":"Read","input":{"file_path":"/x"}}]}}
{"type":"user","uuid":"u1","timestamp":"2026-10-24T10:00:01.500Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"FAIL","is_error":true}]}}
`)
	turns, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || len(turns[0].Parts) != 2 {
		t.Fatalf("turns = %+v", turns)
	}
	done := turns[0].Parts[0]
	if done.ToolID != "toolu_1" || done.At != "2026-10-24T10:00:00.000Z" || !done.IsError || done.Output != "FAIL" {
		t.Errorf("finished call = %+v", done)
	}
	if math.Abs(done.Took-1.5) > 1e-9 {
		t.Errorf("took = %v, want 1.5", done.Took)
	}
	running := turns[0].Parts[1]
	if running.ToolID != "toolu_2" || running.Took != 0 || running.Output != "" {
		t.Errorf("running call = %+v", running)
	}
}

func TestSecondsBetweenIgnoresBadOrBackwardStamps(t *testing.T) {
	if got := secondsBetween("", "2026-10-24T10:00:01Z"); got != 0 {
		t.Errorf("missing from: %v", got)
	}
	if got := secondsBetween("2026-10-24T10:00:02Z", "2026-10-24T10:00:01Z"); got != 0 {
		t.Errorf("backwards: %v", got)
	}
	if got := secondsBetween("2026-10-24T10:00:00Z", "2026-10-24T10:02:30Z"); got != 150 {
		t.Errorf("forward: %v", got)
	}
}
