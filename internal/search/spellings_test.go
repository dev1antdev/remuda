package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestTheFourSpellingsFindACyrillicQuery(t *testing.T) {
	projects := t.TempDir()
	path := filepath.Join(projects, "-p", "s.jsonl")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	user := func(uuid, text string) string {
		return `{"type":"user","uuid":"` + uuid + `","cwd":"/p","timestamp":"2026-10-07T10:00:00Z","message":{"role":"user","content":"` + text + `"}}` + "\n"
	}
	body := user("lower", "давай подключим задачи") + user("upper", "ДАВАЙ ПОДКЛЮЧИМ") + user("capital", "Давай подключим") +
		`{"type":"user","uuid":"tool","cwd":"/p","timestamp":"2026-10-07T10:00:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"давай"}]}}` + "\n"
	_ = os.WriteFile(path, []byte(body), 0o644)
	for _, q := range []string{"давай", "ДАВАЙ", "Давай", "дАвАй"} {
		var b strings.Builder
		if err := Scan(projects, q, "", time.Time{}, 10, &b); err != nil {
			t.Fatal(err)
		}
		for _, uuid := range []string{"lower", "upper", "capital"} {
			if !strings.Contains(b.String(), `"uuid":"`+uuid+`"`) {
				t.Errorf("%q missed the %s spelling", q, uuid)
			}
		}
		if strings.Contains(b.String(), `"uuid":"tool"`) {
			t.Errorf("%q matched a tool result", q)
		}
	}
	if got := spellings("дАвАй"); strings.Join(got, ",") != "дАвАй,давай,ДАВАЙ,Давай" {
		t.Fatalf("spellings = %v", got)
	}
}

func TestMatchCodexLineReadsOnlyWhatWasSaid(t *testing.T) {
	record := func(payload string) []byte {
		return []byte(`{"timestamp":"2026-10-08T09:00:00.000Z","type":"response_item","payload":` + payload + `}`)
	}
	hidden := map[string]string{
		"a tool's output":       `{"type":"function_call_output","call_id":"c","output":"needle"}`,
		"a tool's arguments":    `{"type":"function_call","call_id":"c","name":"exec_command","arguments":"{\"cmd\":\"grep needle\"}"}`,
		"the reasoning":         `{"type":"reasoning","summary":[{"type":"summary_text","text":"needle"}]}`,
		"a harness envelope":    `{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>needle</environment_context>"}]}`,
		"the developer's words": `{"type":"message","role":"developer","content":[{"type":"input_text","text":"needle"}]}`,
	}
	for name, payload := range hidden {
		if m, ok := transcript.MatchCodexLine(record(payload), "needle"); ok {
			t.Errorf("%s matched: %+v", name, m)
		}
	}
	said := record(`{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"Found the Needle."}]}`)
	if m, ok := transcript.MatchCodexLine(said, "needle"); !ok || m.UUID != "msg_1" || m.Role != "assistant" || m.Snippet != "Found the Needle." {
		t.Fatalf("the assistant's text: %+v %v", m, ok)
	}
}
