package transcript

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestMessagesSentWhileItWorkedStayInTheirTurn(t *testing.T) {
	turns, err := Parse("testdata/queued.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Role != "user" || turns[1].Role != "assistant" {
		t.Fatalf("the mid-turn messages split the turn: %+v", turns)
	}
	var queued []Part
	for _, p := range turns[1].Parts {
		if p.Type == "queued" {
			queued = append(queued, p)
		}
	}
	if len(queued) != 2 {
		t.Fatalf("want the person's and the peer's message, the task notification left out: %+v", queued)
	}
	if queued[0].Text != "Also update the README while you are at it." || queued[0].From != "" || queued[0].At != "2026-11-13T09:00:06.000Z" {
		t.Errorf("the person's message: %+v", queued[0])
	}
	if queued[1].Text != "The tests in pkg/x fail on main too." || queued[1].From != "general-purpose" {
		t.Errorf("the peer's message, unwrapped and named: %+v", queued[1])
	}
	order := []string{}
	for _, p := range turns[1].Parts {
		order = append(order, p.Type)
	}
	if got := strings.Join(order, " "); got != "text tool queued queued tool text" {
		t.Errorf("the parts are not in the order they came: %s", got)
	}
	if !slices.Contains(turns[1].IDs, "q-m1") || !slices.Contains(turns[1].IDs, "q-m3") || slices.Contains(turns[1].IDs, "q-m2") {
		t.Errorf("a search jump lands on the turn by the message's record: %v", turns[1].IDs)
	}
	if turns[1].Answer() != "Renamed the flag and updated the README. The pkg/x tests fail on main as well, so I left them." {
		t.Errorf("the answer: %q", turns[1].Answer())
	}
	if turns[1].Took != 30 {
		t.Errorf("the turn's time runs from the prompt to its last record: %v", turns[1].Took)
	}
	if turns[1].Output != 180+420+95 {
		t.Errorf("output tokens: %d", turns[1].Output)
	}
}

func TestAMessageSentBeforeAnyTurnOpensOneOfThePersons(t *testing.T) {
	line := `{"type":"attachment","attachment":{"type":"queued_command","prompt":[{"type":"text","text":"and this too"},{"type":"image","source":{}}],"commandMode":"prompt","origin":{"kind":"human"}},"uuid":"m-0","timestamp":"2026-11-13T09:00:00.000Z"}`
	turns, err := ParseBytes([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 || turns[0].Role != "user" || turns[0].Parts[0].Type != "queued" || turns[0].Parts[0].Text != "and this too\n[image]" {
		t.Fatalf("%+v", turns)
	}
}

func TestTheHarnessesQueuedNotesAreNoMessage(t *testing.T) {
	for _, raw := range []string{
		`{"type":"queued_command","prompt":"<task-notification>done</task-notification>","commandMode":"task-notification","origin":{"kind":"task-notification"}}`,
		`{"type":"queued_command","prompt":"From the parent session: carry on","origin":{"kind":"coordinator"},"isMeta":true}`,
		`{"type":"queued_command","prompt":"<system-reminder>x</system-reminder>","commandMode":"prompt","origin":{"kind":"human"}}`,
		`{"type":"queued_command","prompt":"   ","commandMode":"prompt","origin":{"kind":"human"}}`,
		`{"type":"total_tokens_reminder","used":1}`,
		`not json`,
	} {
		if text, _, ok := queuedOf([]byte(raw)); ok {
			t.Errorf("%s read as %q", raw, text)
		}
	}
}

func TestASearchFindsAMessageSentWhileItWorked(t *testing.T) {
	f, err := os.Open("testdata/queued.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	found := map[string]string{}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 1<<20), 1<<20)
	for scan.Scan() {
		for _, q := range []string{"readme while", "fail on main too", "background command"} {
			if m, ok := MatchLine(scan.Bytes(), q); ok {
				found[q] = m.UUID + " " + m.Role
			}
		}
	}
	if found["readme while"] != "q-m1 user" {
		t.Errorf("the person's mid-turn message: %q", found["readme while"])
	}
	if found["fail on main too"] != "q-m3 user" {
		t.Errorf("the peer's mid-turn message: %q", found["fail on main too"])
	}
	if found["background command"] != "" {
		t.Errorf("a task notification is the harness's: %q", found["background command"])
	}
}
