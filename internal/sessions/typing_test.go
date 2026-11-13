package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/events"
	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/herdr/herdrtest"
)

// typist records herdr's calls in order, as "text:…", "input:…|keys" and "keys:…".
type typist struct{ calls []string }

func (f *typist) PaneSendText(paneID, text string) error {
	f.calls = append(f.calls, "text:"+text)
	return nil
}

func (f *typist) PaneSendInput(paneID, text string, keys []string) error {
	f.calls = append(f.calls, "input:"+text+"|"+strings.Join(keys, ","))
	return nil
}

func (f *typist) PaneSendKeys(paneID string, keys []string) error {
	f.calls = append(f.calls, "keys:"+strings.Join(keys, ","))
	return nil
}

func (f *typist) AgentPrompt(target, text string) error { return nil }

func TestTheKeyGrammar(t *testing.T) {
	for _, good := range []string{"Enter", "esc", "shift+tab", "ctrl+a", "ctrl+z", "alt+b", "alt+enter", "shift+enter", "ctrl+left", "alt+backspace", "ctrl+shift+up", "home", "pagedown", "delete", "ctrl+5"} {
		if _, err := Keys([]string{good}); err != nil {
			t.Errorf("%s refused: %v", good, err)
		}
	}
	for _, bad := range []string{"a", "shift+a", "ctrl+ctrl+a", "meta+a", "ctrl+alt+del", "ctrl+home", "f1", "ctrl+", "+enter", "ctrl+ж", ""} {
		if _, err := Keys([]string{bad}); err == nil {
			t.Errorf("%q allowed", bad)
		}
	}
}

func TestATypedRequestIsChecked(t *testing.T) {
	if _, err := NewTyped("", false, nil); err == nil {
		t.Fatal("an empty request allowed")
	}
	if _, err := NewTyped(strings.Repeat("x", MaxTypedText+1), true, nil); err == nil {
		t.Fatal("a paste over the cap allowed")
	}
	if _, err := NewTyped("ls", false, []string{"f13"}); err == nil {
		t.Fatal("a key outside the grammar allowed")
	}
	typed, err := NewTyped("ls\x1b", false, []string{"ENTER"})
	if err != nil || typed.Text != "ls\x1b" || typed.Keys[0] != "enter" {
		t.Fatalf("any characters and the keys lowercased: %+v %v", typed, err)
	}
}

func TestTypingGoesAsKeystrokesAndAPasteAsAPaste(t *testing.T) {
	in := NewInput(nil, nil)
	ref := events.PaneRef{Machine: "local", Pane: "w1:p1"}
	for _, try := range []struct {
		typed Typed
		calls string
	}{
		{Typed{Text: "ls -la", Keys: []string{"enter"}}, "text:ls -la input:|enter"},
		{Typed{Text: "abc"}, "text:abc"},
		{Typed{Keys: []string{"up", "home"}}, "input:|up text:\x1bOH"},
		{Typed{Text: "one\ntwo", Paste: true, Keys: []string{"enter"}}, "input:one\ntwo|enter"},
		{Typed{Text: "one", Paste: true}, "input:one|"},
		{Typed{Text: "one", Paste: true, Keys: []string{"delete", "left", "left"}}, "input:one| text:\x1b[3~ input:|left,left"},
	} {
		client := &typist{}
		if err := in.SendInput(client, ref, try.typed); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(client.calls, " "); got != try.calls {
			t.Errorf("%+v: %q, want %q", try.typed, got, try.calls)
		}
	}
	client := &typist{}
	if err := in.SendKeys(client, ref, []string{"esc", "pageup", "pageup", "enter"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(client.calls, " "); got != "keys:esc text:\x1b[5~ text:\x1b[5~ keys:enter" {
		t.Errorf("keys herdr has no word for go as their bytes: %q", got)
	}
}

// TestTypingOnAThrowawayHerdr types a command and Enter into a shell through pane.send_text and pane.send_input, as
// herdr 0.9.5 takes them, and reads the line back off the screen; a paste reaches the app between the paste markers.
func TestTypingOnAThrowawayHerdr(t *testing.T) {
	sock := herdrtest.Server(t)
	client := herdr.NewClient(sock)
	dir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	made, err := client.WorkspaceCreate(dir, "typing")
	if err != nil {
		t.Fatal(err)
	}
	defer client.WorkspaceClose(made.Workspace.WorkspaceID)
	in := NewInput(nil, nil)
	ref := events.PaneRef{Machine: "local", Pane: made.RootPane.PaneID}
	screen := func(want string) string {
		for i := 0; i < 40; i++ {
			text, err := client.PaneRead(ref.Pane)
			if err == nil && strings.Contains(text, want) {
				return text
			}
			time.Sleep(150 * time.Millisecond)
		}
		text, _ := client.PaneRead(ref.Pane)
		t.Fatalf("%q not on the screen:\n%s", want, text)
		return ""
	}
	screen("%") // the shell's prompt
	type step struct {
		text  string
		paste bool
		keys  []string
	}
	// "cho typed-ok", the o deleted and typed again, then the e put in front: Home and Delete go as a terminal's bytes
	for _, s := range []step{{text: "cho typed-"}, {text: "ok", keys: []string{"left", "left", "delete"}}, {text: "o", keys: []string{"home"}}, {text: "e", keys: []string{"end", "enter"}}} {
		typed, err := NewTyped(s.text, s.paste, s.keys)
		if err != nil {
			t.Fatal(err)
		}
		if err := in.SendInput(client, ref, typed); err != nil {
			t.Fatal(err)
		}
	}
	screen("\ntyped-ok")
	typed, _ := NewTyped("printf '\\033[?2004h'; cat -v", false, []string{"enter"})
	if err := in.SendInput(client, ref, typed); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	pasted, _ := NewTyped("one\ntwo", true, []string{"enter"})
	if err := in.SendInput(client, ref, pasted); err != nil {
		t.Fatal(err)
	}
	screen("^[[200~one")
	interrupt, _ := NewTyped("", false, []string{"ctrl+c"})
	if err := in.SendInput(client, ref, interrupt); err != nil {
		t.Fatal(err)
	}
	screen("^C")
}
