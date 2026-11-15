package ghpr

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

// A comment's or a reply's text is the person's, and the script gh runs in is a command line on the machine (sh -c, or
// the ssh session's): the text travels as a file put there, read on gh's stdin, never inside the script — where any
// other user's ps reads it and Linux refuses an argument past 128 KB, which a 64 KB comment of markup reaches once JSON
// spells each < as <.

// filerFake is a machine whose gh answers through a fake and that files can be put on.
type filerFake struct {
	*shell.Fake
	mu  sync.Mutex
	put map[string]string
}

func (f *filerFake) PutFile(path string, data []byte, timeout time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.put == nil {
		f.put = map[string]string{}
	}
	f.put[path] = string(data)
	return nil
}

func TestACommentsTextNeverTravelsInsideTheScript(t *testing.T) {
	made := 0
	host := &filerFake{Fake: &shell.Fake{Name: "box", Answer: func(s string) (string, error) {
		if strings.HasPrefix(s, "mktemp") {
			made++
			return "/tmp/remuda-gh." + itoa(made) + "\n", nil
		}
		return answered(`{"data":{}}`, "", 0), nil
	}}}
	secret := "the deploy key rotates on Friday"
	if err := PostComment(host, Repo{"octo", "trips"}, 482, secret); err != nil {
		t.Fatal(err)
	}
	if err := Reply(host, "PRRT_1", secret); err != nil {
		t.Fatal(err)
	}
	for _, s := range host.Scripts() {
		if strings.Contains(s, secret) {
			t.Fatalf("the text rode inside a script, a command line on the machine: %q", s)
		}
	}
	var comment struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(host.put["/tmp/remuda-gh.1"]), &comment); err != nil || comment.Body != secret {
		t.Fatalf("the comment's file put on the machine: %q %v", host.put["/tmp/remuda-gh.1"], err)
	}
	var reply struct {
		Variables struct {
			Body string `json:"body"`
		} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(host.put["/tmp/remuda-gh.2"]), &reply); err != nil || reply.Variables.Body != secret {
		t.Fatalf("the reply's file put on the machine: %q %v", host.put["/tmp/remuda-gh.2"], err)
	}
	if s := host.Scripts()[1]; !strings.Contains(s, `<"$in"`) || !strings.Contains(s, `trap 'rm -f "$in"' EXIT`) {
		t.Fatalf("gh does not read the file, or the file is left behind: %q", s)
	}
}

// standInGh copies what gh is handed on its stdin to ~/input.json and answers as GitHub would.
const standInGh = `#!/bin/sh
cat > "$HOME/input.json"
echo '{"id": 1}'
`

// localFiler is this machine with files put here, as a *machines.Machine is.
type localFiler struct{ shell.Local }

func (localFiler) PutFile(path string, data []byte, timeout time.Duration) error {
	return os.WriteFile(path, data, 0o600)
}

func TestALongCommentOfMarkupReachesGhInARealShell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TMPDIR", home)
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(standInGh), 0o755); err != nil {
		t.Fatal(err)
	}
	if found, err := exec.Command("sh", "-c", `PATH="$HOME/.local/bin:$PATH"; command -v gh`).Output(); err != nil || strings.TrimSpace(string(found)) != filepath.Join(bin, "gh") {
		t.Fatalf("the stand-in is not the gh the scripts find: %q %v", found, err)
	}
	markup := strings.Repeat("<li>", 6000) + "and the rest" // 24 KB of text; JSON spells each < in six bytes
	if err := PostComment(localFiler{shell.Local{Name: "here"}}, Repo{"octo", "trips"}, 482, markup); err != nil {
		t.Fatalf("a 24 KB comment did not reach gh: %v", err)
	}
	var body struct {
		Body string `json:"body"`
	}
	data, _ := os.ReadFile(filepath.Join(home, "input.json"))
	if err := json.Unmarshal(data, &body); err != nil || body.Body != markup {
		t.Fatalf("gh was handed %d bytes: %v", len(data), err)
	}
	if left, _ := filepath.Glob(filepath.Join(home, "remuda-gh.*")); len(left) > 0 {
		t.Fatalf("files left on the machine: %v", left)
	}
}
