package machines

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dev1antdev/remuda/internal/rpath"
	"github.com/dev1antdev/remuda/internal/wintest"
)

// TestOnWindowsTheFileViewersScriptsReadAWindowsFolder runs the viewer's stat, its search by suffix and the folder
// picker's listing over the runner's OpenSSH on a folder in Windows' spelling.
func TestOnWindowsTheFileViewersScriptsReadAWindowsFolder(t *testing.T) {
	wintest.Live(t)
	remote := wintest.Remote(t)
	windows := rpath.Style{Windows: true}
	dir := t.TempDir()
	for name, data := range map[string]string{`src\main.go`: "package main\n", `docs\demo.mp4`: "\x00\x00\x00\x18ftyp"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	text := filepath.Join(dir, "src", "main.go")
	info, err := readStat(text, wintest.Text(t, remote, statScript(text)))
	if err != nil || info.Kind != "text" || info.Size != 13 {
		t.Fatalf("stat of %s: %+v, %v", text, info, err)
	}
	if info, _ := readStat(dir, wintest.Text(t, remote, statScript(dir))); info.Kind != "dir" {
		t.Fatalf("a folder: %+v", info)
	}
	found := readFound(windows, wintest.Text(t, remote, findScript(windows, dir, "demo.mp4")))
	if len(found) != 1 || !windows.Equal(found[0], filepath.Join(dir, "docs", "demo.mp4")) {
		t.Fatalf("found by suffix: %q", found)
	}
	listing, err := readListing(wintest.Text(t, remote, listDirsScript(dir, false, true)), true)
	if err != nil || len(listing.Dirs) != 2 || !rpath.IsWindows(listing.Path) || !rpath.IsWindows(listing.Home) {
		t.Fatalf("the listing: %+v, %v", listing, err)
	}
	same, _ := os.Stat(dir)
	there, err := os.Stat(listing.Path)
	if err != nil || !os.SameFile(same, there) {
		t.Fatalf("the listing names %s for %s: %v", listing.Path, dir, err)
	}
}
