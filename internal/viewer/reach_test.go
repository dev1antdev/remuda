package viewer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/rpath"
)

func check(t *testing.T, reach Reach, want map[string]bool) {
	t.Helper()
	for path, allowed := range want {
		if got := reach.Allows(path); got != allowed {
			t.Errorf("%s: %v, want %v", path, got, allowed)
		}
	}
}

// TestAViewerReachesTheWorkspacesAndNoSecret: a workspace at a home or above one opens nothing, and the places keys live
// in stay closed inside any workspace, whatever the case they are asked in.
func TestAViewerReachesTheWorkspacesAndNoSecret(t *testing.T) {
	dirs := []string{"/", "/home", "/home/me", "/home/me/", "/HOME/ME", "/home/other", "/root", "/home/me/app", "/home/me/dotfiles", "/home/me/.ssh"}
	check(t, Of(dirs, "/home/me", "/home/me/dotfiles/remuda", rpath.POSIX, false), map[string]bool{
		"/home/me/app/main.go":            true,
		"/home/me/app":                    true,
		"/home/me/app2/main.go":           false,
		"/home/me/dotfiles/vimrc":         true,
		"/home/me/notes.txt":              false,
		"/etc/passwd":                     false,
		"/home/other/x":                   false,
		"/root/x":                         false,
		"/home/me/.ssh/id_ed25519":        false,
		"/home/me/.SSH/id_ed25519":        false,
		"/home/me/dotfiles/remuda/token":  false,
		"/home/me/dotfiles/REMUDA/token":  false,
		"/home/me/app/../.ssh/id_ed25519": false,
	})
	check(t, Of([]string{"/Users/me", "/users/me", "/Users/other", "/Users/me/Library"}, "/Users/me", "/Users/me/Library/Application Support/remuda", rpath.POSIX, false), map[string]bool{
		"/Users/me/Library/Notes/a":                          true,
		"/Users/me/Library/Application Support/remuda/token": false,
		"/users/me/secret":                                   false,
		"/Users/other/x":                                     false,
	})
	check(t, Of([]string{`C:\Users\me`, `C:\Users\me\AppData`, `C:\Users\me\app\`}, `C:\Users\me`, `C:\Users\me\AppData\Roaming\remuda`, rpath.Style{Windows: true}, false), map[string]bool{
		`C:\Users\me\app\src\main.go`:              true,
		`c:\users\me\APP\README.md`:                true,
		`C:\Users\me\app`:                          true,
		`C:\Users\me\AppData\Local\x`:              true,
		`C:\Users\me\application\x.go`:             false,
		`c:\users\me\appdata\roaming\remuda\token`: false,
		`C:\Users\me\.ssh\id_ed25519`:              false,
		`C:\Users\me\app\..\.ssh\id_rsa`:           false,
		`C:\Users\me\notes.txt`:                    false,
	})
}

func TestAMachineNotReadYetReachesNothing(t *testing.T) {
	for _, reach := range []Reach{Of([]string{"/w/app"}, "", "/s", rpath.POSIX, false), Of([]string{"/w/app"}, "/home/me", "", rpath.POSIX, false), {}} {
		if reach.Allows("/w/app/x") {
			t.Errorf("%+v reached a file", reach)
		}
	}
}

// TestACheckoutHoldingAHomeOrAClosedPlaceShowsNoHistory: its commits name files of every folder of it.
func TestACheckoutHoldingAHomeOrAClosedPlaceShowsNoHistory(t *testing.T) {
	reach := Of([]string{"/home/me/dotfiles/nvim", "/home/me/app"}, "/home/me", "/home/me/dotfiles/remuda", rpath.POSIX, false)
	for top, want := range map[string]bool{"/home/me/app": true, "/home/me/mono": true, "": true, "/home/me/dotfiles": false, "/home/me": false, "/": false} {
		if got := reach.AllowsCheckout(top); got != want {
			t.Errorf("%q: %v, want %v", top, got, want)
		}
	}
	if (Reach{}).AllowsCheckout("/home/me/app") {
		t.Error("a machine not read yet")
	}
}

// TestTheUsualStateDirsStayClosedBesideTheOneReported: the agents' door stands there whatever the service uses.
func TestTheUsualStateDirsStayClosedBesideTheOneReported(t *testing.T) {
	check(t, Of([]string{"/home/me/.config", "/home/me/Library", "/home/me/AppData"}, "/home/me", "/srv/remuda", rpath.POSIX, false), map[string]bool{
		"/home/me/.config/gh/hosts":                            true,
		"/home/me/.config/remuda/agent-url":                    false,
		"/home/me/Library/Application Support/remuda/tasks.db": false,
		"/home/me/AppData/Roaming/remuda/tasks.db":             false,
	})
}

func TestRemudasSecretsAreClosedByNameAnywhere(t *testing.T) {
	check(t, Of([]string{"/home/me/app"}, "/home/me", "/home/me/.config/remuda", rpath.POSIX, false), map[string]bool{
		"/home/me/app/run/token":           false,
		"/home/me/app/run/TOKEN":           false,
		"/home/me/app/run/agent-key":       false,
		"/home/me/app/run/agent-keys.json": false,
		"/home/me/app/run/agent-key.new":   false,
		"/home/me/app/run/tokens.go":       true,
	})
}

// TestOpeningLocallyStaysInTheWorkspace: a link leading out is refused by the open itself, whatever was checked before.
func TestOpeningLocallyStaysInTheWorkspace(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "people", "somebody", "me")
	app := filepath.Join(home, "app")
	if err := os.MkdirAll(app, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	reach := Of([]string{app}, home, filepath.Join(root, "state"), rpath.POSIX, true)
	nothing := func(string) bool { return false }
	f, err := reach.OpenLocal(filepath.Join(app, "main.go"), nothing)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if f, err := reach.OpenLocal(filepath.Join(app, "main.go"), func(p string) bool { return p == filepath.Join(app, "main.go") }); err == nil {
		f.Close()
		t.Error("a file the caller refuses opened")
	}
	if err := os.WriteFile(filepath.Join(app, "token"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	if f, err := reach.OpenLocal(filepath.Join(app, "token"), nothing); err == nil {
		f.Close()
		t.Error("a closed file inside the workspace opened")
	}
	if _, err := reach.OpenLocal(filepath.Join(home, "notes"), nothing); err == nil {
		t.Error("a file out of reach opened")
	}
	secret := filepath.Join(root, "secret")
	if err := os.WriteFile(secret, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(app, "swapped")); err != nil {
		t.Fatal(err)
	}
	if f, err := reach.OpenLocal(filepath.Join(app, "swapped"), nothing); err == nil {
		f.Close()
		t.Error("a link out of the folder opened")
	}
}

func TestReachFollowsLinksOnThisMachine(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "people", "somebody", "me")
	app, outside, keys := filepath.Join(home, "app"), filepath.Join(root, "outside"), filepath.Join(home, ".ssh")
	for _, dir := range []string{app, outside, keys} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		filepath.Join(app, "out"):  filepath.Join(outside, "secret"),
		filepath.Join(app, "keys"): keys,
		filepath.Join(root, "up"):  home,
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	reach := Of([]string{app, filepath.Join(root, "up")}, home, filepath.Join(root, "state"), rpath.POSIX, true)
	check(t, reach, map[string]bool{
		filepath.Join(app, "main.go"):            true,
		filepath.Join(app, "not-yet.png"):        true,
		app:                                      true,
		filepath.Join(app, "out"):                false,
		filepath.Join(app, "keys", "id_ed25519"): false,
		filepath.Join(app, "..", "notes"):        false,
		filepath.Join(root, "up", "notes"):       false,
		root:                                     false,
	})
}

// TestAPatchShowingASecretIsRefused: a token committed once stays in history, whatever became of the folder since.
func TestAPatchShowingASecretIsRefused(t *testing.T) {
	file := func(old, new string) string {
		return "diff --git a/x b/x\n--- " + old + "\n+++ " + new + "\n@@ -1 +1 @@\n-a\n+b\n"
	}
	for patch, want := range map[string]bool{
		"":                                   true,
		file("a/web/app.js", "b/web/app.js"): true,
		file("a/docs/token handling.md\t", "b/docs/token handling.md\t"):                true,
		"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n--- a/token\n+++ b/token\n": true,
		file("/dev/null", "b/run/token"):                                                false,
		file("a/run/token", "/dev/null"):                                                false,
		file("a/web/app.js", "b/web/app.js") + file("a/run/TOKEN", "b/run/TOKEN"):       false,
		file(`"a/caf\303\251/token"`, `"b/caf\303\251/token"`):                          false,
		file("a/run/webhooks.json", "b/run/webhooks.json"):                              false,
		file("a/run/vapid.json", "b/run/vapid.json"):                                    false,
		file("a/run/jira.json", "b/run/jira.json"):                                      false,
		file("a/home/.ssh/id_ed25519", "b/home/.ssh/id_ed25519"):                        false,
		file("a/.config/remuda/tasks.db", "b/.config/remuda/tasks.db"):                  false,
		"diff --git a/old/agent-key b/new/notes\nsimilarity index 90%\nrename from old/agent-key\nrename to new/notes\n--- a/old/agent-key\n+++ b/new/notes\n": false,
		"diff --git a/o b/n\nsimilarity index 100%\nrename from run/serve.lock\nrename to n\n":                                                                 false,
	} {
		if got := PatchAllowed(patch); got != want {
			t.Errorf("%q: %v, want %v", patch, got, want)
		}
	}
}
