package transcript

import (
	"os"
	"strings"
	"testing"
)

// testdata/denied.jsonl is a Claude Code 2.1.322 session run under permissions.denyInsteadOfPrompting, anonymized: two
// calls refused without asking, a read that went through, a call the auto mode classifier refused, a failed command,
// the answer; then a second prompt whose only call is refused and the turn stopped there.
func deniedTurns(t *testing.T) []Turn {
	t.Helper()
	data, err := os.ReadFile("testdata/denied.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	turns, err := ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	return turns
}

func TestDeniedReadsTheCallsRefusedWithoutAsking(t *testing.T) {
	turns := deniedTurns(t)
	got := DeniedAfter(turns, "")
	want := []Denial{
		{Tool: "Bash", What: "touch probe.txt", At: "2026-11-12T03:00:02.000Z", ToolID: "toolu_bash_1"},
		{Tool: "Write", What: "/home/me/project/notes.txt", At: "2026-11-12T03:00:05.000Z", ToolID: "toolu_write_1"},
		{Tool: "Bash", What: "git commit -am 'Add the probe files'", At: "2026-11-12T03:05:02.000Z", ToolID: "toolu_bash_4"},
	}
	if len(got.Calls) != len(want) {
		t.Fatalf("calls: %+v", got.Calls)
	}
	for i := range want {
		if got.Calls[i] != want[i] {
			t.Errorf("call %d: %+v, want %+v", i, got.Calls[i], want[i])
		}
	}
	if !got.EndsTurn {
		t.Error("the last turn stopped on a refused call")
	}
}

func TestDeniedAfterARecordCountsOnlyWhatCameLater(t *testing.T) {
	turns := deniedTurns(t)
	later := DeniedAfter(turns, "a-6") // the first answer's record: the second prompt's refusal alone
	if len(later.Calls) != 1 || later.Calls[0].ToolID != "toolu_bash_4" || !later.EndsTurn {
		t.Errorf("after the first answer: %+v", later)
	}
	first := DeniedAfter(turns[:2], "")
	if len(first.Calls) != 2 || first.EndsTurn {
		t.Errorf("the first exchange ends in an answer: %+v", first)
	}
	if none := DeniedAfter(turns, "u-8-unknown"); len(none.Calls) != 3 {
		t.Errorf("an unknown record reads the whole conversation: %+v", none)
	}
	if none := DeniedAfter(nil, ""); len(none.Calls) != 0 || none.EndsTurn {
		t.Errorf("nothing: %+v", none)
	}
}

func TestIsDeniedLeavesOtherRefusalsAndErrorsOut(t *testing.T) {
	call := func(out string, isError bool) Part {
		return Part{Type: "tool", ToolName: "Bash", Output: out, IsError: isError}
	}
	for name, part := range map[string]Part{
		"the classifier":   call("Permission for this action was denied by the Claude Code auto mode classifier. Reason: x", true),
		"the person":       call("The user doesn't want to proceed with this tool use. The tool use was rejected.", true),
		"a deny rule":      call("Permission to use Bash with command rm has been denied.", true),
		"a failed command": call("Exit code 1", true),
		"not an error":     call("Permission for this tool use was denied. It requires approval, and this session has no approval surface", false),
		"text":             {Type: "text", Text: "Permission for this tool use was denied: it requires interactive approval", IsError: true},
	} {
		if IsDenied(part) {
			t.Errorf("%s counts as denied without asking", name)
		}
	}
	for name, out := range map[string]string{
		"no prompt in this session": "Permission for this tool use was denied: it requires interactive approval, and permission prompts are not available in this session.",
		"don't ask mode":            "Permission to use Bash has been denied because Claude Code is running in don't ask mode.",
		"as the record says it":     "Error: Permission for this tool use was denied. It requires approval, and this session has no approval surface — nobody can answer",
	} {
		if !IsDenied(call(out, true)) {
			t.Errorf("%s: not read as denied", name)
		}
	}
}

func TestDeniedWords(t *testing.T) {
	calls := DeniedAfter(deniedTurns(t), "").Calls
	if got := DeniedWords(calls, 2); got != "3 calls denied: Bash touch probe.txt; Write /home/me/project/notes.txt; +1 more" {
		t.Errorf("words: %q", got)
	}
	if got := DeniedWords(calls[:1], 2); got != "1 call denied: Bash touch probe.txt" {
		t.Errorf("one: %q", got)
	}
	if DeniedWords(nil, 2) != "" {
		t.Error("none says nothing")
	}
	long := Denial{Tool: "Bash", What: strings.Repeat("x", 300)}
	if got := DeniedWords([]Denial{long}, 2); len([]rune(got)) > 90 {
		t.Errorf("a long command is cut: %q", got)
	}
}
