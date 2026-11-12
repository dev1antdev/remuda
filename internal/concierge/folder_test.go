package concierge

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

func TestTheFolderIsMadeUnderTheCockpitsOwnStateDirWithRemudasMemory(t *testing.T) {
	state := filepath.Join(t.TempDir(), "remuda's state")
	folder, err := Prepare(shell.Local{Name: "local"}, state)
	if err != nil {
		t.Fatal(err)
	}
	if folder != filepath.Join(state, Folder) {
		t.Errorf("the folder: %q", folder)
	}
	memory, err := os.ReadFile(filepath.Join(folder, "CLAUDE.md"))
	if err != nil || string(memory) != Memory {
		t.Errorf("remuda's CLAUDE.md, as written: %v %q", err, memory)
	}
	if info, err := os.Stat(folder); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("private to the user: %v %v", err, info.Mode())
	}
	if _, err := Prepare(shell.Local{Name: "local"}, state); err != nil {
		t.Errorf("made again over itself: %v", err)
	}
}

// homeShell runs the scripts with HOME set to a folder of the test's own, the way a remote machine's home would be
type homeShell struct {
	shell.Local
	home string
}

func (h homeShell) Run(body string, timeout time.Duration) ([]byte, error) {
	return h.Local.Run("HOME="+script.Quote(h.home)+"; XDG_CONFIG_HOME=; export HOME XDG_CONFIG_HOME\n"+body, timeout)
}

func TestARemoteFolderIsUnderTheDirRemudaKeepsThere(t *testing.T) {
	home := t.TempDir()
	folder, err := Prepare(homeShell{home: home}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "remuda", Folder)
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Application Support", "remuda", Folder)
	}
	if folder != want {
		t.Errorf("%q, want %q", folder, want)
	}
	if memory, _ := os.ReadFile(filepath.Join(folder, "CLAUDE.md")); !strings.Contains(string(memory), "concierge") {
		t.Errorf("remuda's CLAUDE.md there: %q", memory)
	}
}

func TestTheMemorySaysWhoItIsWhatItMayDoAndHowToAnswer(t *testing.T) {
	for _, want := range []string{"concierge", "Proposed", "cannot read or change files, run commands, go on the web", "briefly, in the language the person writes in"} {
		if !strings.Contains(Memory, want) {
			t.Errorf("the memory does not say %q", want)
		}
	}
	for _, tool := range Tools {
		if !strings.Contains(Memory, tool) {
			t.Errorf("the memory does not name %s", tool)
		}
	}
}
