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

// TestACheckoutHoldingAClosedPlaceShowsNoHistory: its commits name files of every folder of it, the closed one's too.
func TestACheckoutHoldingAClosedPlaceShowsNoHistory(t *testing.T) {
	reach := Of([]string{"/home/me/dotfiles", "/home/me/app"}, "/home/me", "/home/me/dotfiles/remuda", rpath.POSIX, false)
	if !reach.Allows("/home/me/dotfiles") || reach.AllowsAll("/home/me/dotfiles") {
		t.Error("the checkout that holds the state dir")
	}
	if !reach.AllowsAll("/home/me/app") || !reach.AllowsAll("/home/me/app/web") {
		t.Error("an ordinary checkout")
	}
	if reach.AllowsAll("/home/me/other") {
		t.Error("a folder out of reach")
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
