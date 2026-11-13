package handover

import (
	"reflect"
	"testing"

	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestAMessageSentWhileItWorkedStaysInItsPlace(t *testing.T) {
	turns := []transcript.Turn{
		{Role: "user", Parts: []transcript.Part{text("fix the tests")}},
		{Role: "assistant", Parts: []transcript.Part{
			tool("Bash", `{"command":"go test ./..."}`, true),
			{Type: "queued", Text: "skip the flaky one\nit fails on main too"},
			{Type: "queued", Text: "pkg/x is broken upstream", From: "general-purpose"},
			text("Done."),
		}},
	}
	got := Exchanges(turns)[0].Lines
	want := []string{
		"- Bash: go test ./... (failed)",
		"> Me, while you worked: skip the flaky one\n> it fails on main too",
		"> general-purpose, while you worked: pkg/x is broken upstream",
		"Done.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}
