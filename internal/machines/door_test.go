package machines

import (
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

// TestDoorScriptPlacesTheFilesPrivately: the script finds remuda's state dir the way state.Dir does on either system,
// takes the key on stdin and leaves both files readable by the user alone.
func TestDoorScriptPlacesTheFilesPrivately(t *testing.T) {
	host := &shell.Fake{Name: "gog", Dir: "/home/me", Remote: true}
	if _, err := host.Run(DoorScript("http://127.0.0.1:7318"), 0); err != nil {
		t.Fatal(err)
	}
	ran := host.Scripts()
	if len(ran) != 1 {
		t.Fatalf("%d scripts", len(ran))
	}
	s := ran[0]
	for _, want := range []string{
		`umask 077`,
		`Darwin) d="$HOME/Library/Application Support/remuda"`,
		`d="${XDG_CONFIG_HOME:-$HOME/.config}/remuda"`,
		`chmod 700 "$d"`,
		`cat > "$d/agent-key.new"`,
		`mv -f "$d/agent-key.new" "$d/agent-key"`,
		`printf '%s\n' 'http://127.0.0.1:7318' > "$d/agent-url"`,
		`chmod 600 "$d/agent-key" "$d/agent-url"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the script lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "rk_") {
		t.Error("a key must never be in the script itself")
	}
}
