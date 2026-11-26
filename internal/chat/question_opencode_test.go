package chat

import (
	"strings"
	"testing"
)

// opencode's permission dialog, as its TUI draws it in a pane (the borders stripped by screenLines).
const opencodePermissionScreen = `  ┃  read the file .env and tell me what FOO is
  ┃
  ┃
  ┃  △ Permission required
  ┃
  ┃  $ ls -la /private/tmp/oc-perm
  ┃
  ┃
  ┃   Allow once   Always allow   Reject  ctrl+f fullscreen  ⇆ select  enter con
  ┃
`

func TestOpencodePermissionIsAQuestionAlongARow(t *testing.T) {
	q, ok := ParseQuestion(opencodePermissionScreen)
	if !ok {
		t.Fatal("the permission dialog read as no question")
	}
	if !strings.Contains(q.Text, "Permission required") || !strings.Contains(q.Text, "$ ls -la") {
		t.Errorf("the words: %q", q.Text)
	}
	if len(q.Options) != 3 || q.Options[0].Label != "Allow once" || q.Options[1].Label != "Always allow" || q.Options[2].Label != "Reject" {
		t.Fatalf("the choices: %+v", q.Options)
	}
	if !q.Options[0].Picked || q.Options[1].Picked || q.Options[2].Picked || q.Axis != "horizontal" {
		t.Errorf("cursor and axis: %+v %q", q.Options, q.Axis)
	}
	keys, err := Answer(opencodePermissionScreen, 2, "Reject", DialogKey(q.Text))
	if err != nil || strings.Join(keys, " ") != "right right enter" {
		t.Fatalf("the answer: %v, %v", keys, err)
	}
	if keys, err := Answer(opencodePermissionScreen, 0, "Allow once", DialogKey(q.Text)); err != nil || strings.Join(keys, " ") != "enter" {
		t.Fatalf("Allow once: %v, %v", keys, err)
	}
}

func TestAnotherDialogWithTheSameWordsIsNotAnswered(t *testing.T) {
	q, _ := ParseQuestion(opencodePermissionScreen)
	if _, err := Answer(opencodePermissionScreen, 1, "Always allow", "not this dialog"); err == nil {
		t.Fatal("a stale offer answered a different dialog")
	}
	if _, err := Answer(opencodePermissionScreen, 1, "Reject", DialogKey(q.Text)); err == nil {
		t.Fatal("a stale label answered the wrong choice")
	}
}
