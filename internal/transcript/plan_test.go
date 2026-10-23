package transcript

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// toolCall is one assistant record calling a tool; toolResult the user record answering it.
func toolCall(id, name, input string) string {
	return fmt.Sprintf(`{"type":"assistant","uuid":"a-%s","timestamp":"2026-10-24T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"%s","name":"%s","input":%s}]}}`, id, id, name, input)
}

func toolResult(id, text string) string {
	return fmt.Sprintf(`{"type":"user","uuid":"u-%s","timestamp":"2026-10-24T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"%s","content":%q}]}}`, id, id, text)
}

func parseLines(t *testing.T, lines ...string) []Turn {
	t.Helper()
	turns, err := ParseBytes([]byte(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return turns
}

func TestPlanFollowsTheTaskTools(t *testing.T) {
	turns := parseLines(t,
		toolCall("t1", "TaskCreate", `{"subject":"Scaffold the server","description":"Go, one binary","activeForm":"Scaffolding the server"}`),
		toolResult("t1", "Task #1 created successfully: Scaffold the server"),
		toolCall("t2", "TaskCreate", `{"subject":"Build the page","activeForm":"Building the page"}`),
		toolResult("t2", "Task #2 created successfully: Build the page"),
		toolCall("t3", "TaskUpdate", `{"taskId":"1","status":"in_progress"}`),
		toolResult("t3", "Updated task #1 status"),
		toolCall("t4", "TaskUpdate", `{"taskId":"1","status":"completed"}`),
		toolResult("t4", "Updated task #1 status"),
		toolCall("t5", "TaskUpdate", `{"taskId":"2","subject":"Build the React page","status":"in_progress"}`),
		toolResult("t5", "Updated task #2"),
		toolCall("t6", "TaskUpdate", `{"taskId":"9","status":"completed"}`),
		toolResult("t6", "Updated task #9 status"),
		toolCall("t7", "TaskUpdate", `{"taskId":"2","status":"on_hold"}`),
		toolResult("t7", "Updated task #2 status"),
		toolCall("t8", "TaskCreate", `{"subject":"Ship it"}`),
	)
	want := []PlanItem{
		{ID: "1", Text: "Scaffold the server", Active: "Scaffolding the server", Status: "completed"},
		{ID: "2", Text: "Build the React page", Active: "Building the page", Status: "in_progress"},
		{ID: "3", Text: "Ship it", Status: "pending"},
	}
	if got := Plan(turns); !reflect.DeepEqual(got, want) {
		t.Errorf("plan = %+v\nwant %+v", got, want)
	}
}

func TestPlanTodoWriteReplacesTheListAndDeletedTakesOneOut(t *testing.T) {
	turns := parseLines(t,
		toolCall("t1", "TodoWrite", `{"todos":[{"content":"Read the code","status":"completed","activeForm":"Reading the code"},{"content":"Fix it","status":"in_progress","activeForm":"Fixing it"}]}`),
		toolResult("t1", "Todos have been modified successfully"),
		toolCall("t2", "TodoWrite", `{"todos":[{"id":"a","content":"Read the code","status":"completed"},{"id":"b","content":"Fix it","status":"completed"},{"id":"c","content":"Test it","status":"weird"}]}`),
		toolResult("t2", "Todos have been modified successfully"),
		toolCall("t3", "TodoWrite", `"{\"todos\":[{\"content\":\"a list the parser cut"`),
		toolResult("t3", "Todos have been modified successfully"),
		toolCall("t4", "TaskUpdate", `{"taskId":"b","status":"deleted"}`),
		toolResult("t4", "Deleted task #b"),
	)
	want := []PlanItem{
		{ID: "a", Text: "Read the code", Status: "completed"},
		{ID: "c", Text: "Test it", Status: "pending"},
	}
	if got := Plan(turns); !reflect.DeepEqual(got, want) {
		t.Errorf("plan = %+v\nwant %+v", got, want)
	}
}

func TestPlanIsNilWithoutOne(t *testing.T) {
	turns := parseLines(t,
		toolCall("t1", "Bash", `{"command":"go test ./..."}`),
		toolResult("t1", "ok"),
		toolCall("t2", "TaskUpdate", `{"taskId":"1","status":"completed"}`),
		toolResult("t2", "Updated task #1 status"),
	)
	if got := Plan(turns); got != nil {
		t.Errorf("plan = %+v, want none", got)
	}
}
