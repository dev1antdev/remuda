package rpath

import "testing"

var windows = Style{Windows: true}

func TestAMachineIsKnownByItsHome(t *testing.T) {
	for home, want := range map[string]bool{
		`C:\Users\me`:     true,
		`c:/Users/me`:     true,
		`\\nas\home\me`:   true,
		"/Users/me":       false,
		"/home/me":        false,
		"/c/Users/me":     false, // Git Bash's own spelling never reaches a home: the connection reads Windows' one
		"":                false,
		"C:":              true,
		"Cx/Users":        false,
		"relative/C:\\no": false,
	} {
		if got := For(home).Windows; got != want {
			t.Errorf("For(%q).Windows = %v, want %v", home, got, want)
		}
	}
}

func TestAWindowsPathIsCleanedIntoWindowsOwnSpelling(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Users\runneradmin\`:          `C:\Users\runneradmin`, // herdr names a home folder with its separator at the end
		`c:/Users/me/app`:                `C:\Users\me\app`,
		`/c/Users/me/app`:                `C:\Users\me\app`,
		`/c`:                             `C:\`,
		`C:\`:                            `C:\`,
		`C:`:                             `C:\`,
		`C:\Users\me\app\..\lib\.\x.go`:  `C:\Users\me\lib\x.go`,
		`C:\Users\me/.claude/projects\x`: `C:\Users\me\.claude\projects\x`,
		`\\nas\share\dir\`:               `\\nas\share\dir`,
		`C:\Users\me\\app`:               `C:\Users\me\app`,
		``:                               ``,
	} {
		if got := windows.Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := POSIX.Clean("/c/Users/me/"); got != "/c/Users/me" {
		t.Errorf("a POSIX path stays POSIX: %q", got)
	}
}

func TestJoinDirAndBaseFollowTheMachine(t *testing.T) {
	if got := windows.Join(`C:\Users\me`, ".claude", "projects"); got != `C:\Users\me\.claude\projects` {
		t.Errorf("Join = %q", got)
	}
	if got := windows.Join(`C:\Users\me\app`, "src/main.go"); got != `C:\Users\me\app\src\main.go` {
		t.Errorf("Join of a relative path with slashes = %q", got)
	}
	if got := POSIX.Join("/home/me", "app", "x.go"); got != "/home/me/app/x.go" {
		t.Errorf("POSIX Join = %q", got)
	}
	for in, want := range map[string]string{`C:\Users\me\app`: `C:\Users\me`, `C:\app`: `C:\`, `C:\`: `C:\`, `app`: "."} {
		if got := windows.Dir(in); got != want {
			t.Errorf("Dir(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{`C:\Users\me\app\`: "app", `C:\Users\me\app`: "app", `C:\`: `C:\`} {
		if got := windows.Base(in); got != want {
			t.Errorf("Base(%q) = %q, want %q", in, got, want)
		}
	}
	if got := POSIX.Base("/home/me/app"); got != "app" {
		t.Errorf("POSIX Base = %q", got)
	}
}

func TestAWindowsPathIsAbsoluteFromItsDrive(t *testing.T) {
	for p, want := range map[string]bool{`C:\x`: true, "C:/x": true, "/c/x": true, `\\nas\x`: true, `x\y`: false, "x/y": false, "~/x": false} {
		if got := windows.IsAbs(p); got != want {
			t.Errorf("windows.IsAbs(%q) = %v", p, got)
		}
	}
	if !POSIX.IsAbs("/home/me") || POSIX.IsAbs(`C:\x`) {
		t.Error("a POSIX machine is absolute from / alone")
	}
}

func TestWithinIgnoresCaseAndSeparatorsOnWindows(t *testing.T) {
	root := `C:\Users\me\.herdr\worktrees`
	cases := []struct {
		p    string
		rel  string
		ok   bool
		what string
	}{
		{`C:\Users\me\.herdr\worktrees\app\feature`, "app/feature", true, "under it"},
		{`c:/users/ME/.herdr/worktrees/app/feature/`, "app/feature", true, "another spelling"},
		{`C:\Users\me\.herdr\worktrees`, ".", true, "the root itself"},
		{`C:\Users\me\.herdr\worktrees2\app`, "", false, "a sibling sharing the prefix"},
		{`D:\Users\me\.herdr\worktrees\app`, "", false, "another drive"},
	}
	for _, c := range cases {
		rel, ok := windows.Rel(root, c.p)
		if ok != c.ok || rel != c.rel {
			t.Errorf("%s: Rel(%q) = %q, %v; want %q, %v", c.what, c.p, rel, ok, c.rel, c.ok)
		}
	}
	if !POSIX.Within("/home/me", "/home/me/app") || POSIX.Within("/home/me", "/home/meow") || POSIX.Within("/home/me", "/HOME/me/app") {
		t.Error("POSIX Within is exact and stops at a separator")
	}
	if !windows.Equal(`C:\Users\Me\App\`, "c:/users/me/app") {
		t.Error("two spellings of one Windows folder are equal")
	}
}

func TestSlashIsHowGitBashAndGitReadAWindowsPath(t *testing.T) {
	if got := windows.Slash(`C:\Users\me\app`); got != "C:/Users/me/app" {
		t.Errorf("Slash = %q", got)
	}
	if got := POSIX.Slash("/home/me"); got != "/home/me" {
		t.Errorf("POSIX Slash = %q", got)
	}
}
