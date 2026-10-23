package transcript

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTurnsSayWhatTheySpent(t *testing.T) {
	usage := `"usage":{"input_tokens":100,"output_tokens":%d,"cache_read_input_tokens":1000}`
	lines := []string{
		`{"type":"user","uuid":"u1","timestamp":"2026-10-23T10:00:00.000Z","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-10-23T10:00:05.000Z","message":{"id":"msg_1","model":"claude-fable-5-1","content":[{"type":"text","text":"Looking."}],` + strings.Replace(usage, "%d", "10", 1) + `}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-10-23T10:00:08.000Z","message":{"id":"msg_1","model":"claude-fable-5-1","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/x"}}],` + strings.Replace(usage, "%d", "40", 1) + `}}`,
		`{"type":"user","uuid":"u2","timestamp":"2026-10-23T10:00:09.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
		`{"type":"assistant","uuid":"a3","timestamp":"2026-10-23T10:04:17.000Z","message":{"id":"msg_2","model":"claude-fable-5-1","content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":50,"output_tokens":20}}}`,
		`{"type":"user","uuid":"u3","timestamp":"2026-10-23T10:05:00.000Z","message":{"role":"user","content":"thanks"}}`,
		`{"type":"assistant","uuid":"a4","timestamp":"2026-10-23T10:05:02.000Z","message":{"id":"msg_3","model":"<synthetic>","content":[{"type":"text","text":"Welcome."}]}}`,
	}
	turns, err := ParseBytes([]byte(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 4 {
		t.Fatalf("%d turns", len(turns))
	}
	answer := turns[1]
	if answer.Role != "assistant" || len(answer.Responses) != 2 || answer.Responses[0].Output != 40 || answer.Responses[1].MessageID != "msg_2" {
		t.Fatalf("responses %+v", answer.Responses)
	}
	if answer.Tokens != 100+40+1000+50+20 {
		t.Fatalf("tokens %d", answer.Tokens)
	}
	if answer.Took != 257 {
		t.Fatalf("took %v", answer.Took)
	}
	bare := turns[3]
	if bare.Tokens != 0 || bare.Took != 2 || bare.Cost != 0 {
		t.Fatalf("a turn without usage: %+v", bare)
	}
	out, _ := json.Marshal(bare)
	if strings.Contains(string(out), "tokens") || strings.Contains(string(out), "cost") || !strings.Contains(string(out), `"took":2`) {
		t.Fatalf("json %s", out)
	}
	if turns[0].Took != 0 || turns[0].Tokens != 0 {
		t.Fatalf("a user turn spends nothing: %+v", turns[0])
	}
}
