package scenes

import (
	"testing"
)

func TestAWindowsStepsFoldersAreKeptRelativeAndLaidOutAgain(t *testing.T) {
	dir := `C:\Users\me\app\`
	for folder, want := range map[string]string{
		`C:\Users\me\app`:           "",
		`C:\Users\me\app\web`:       "web",
		`c:/users/me/app/.remuda/x`: ".remuda/x",
		`D:\elsewhere`:              `D:\elsewhere`,
	} {
		if got := relativeTo(dir, folder); got != want {
			t.Errorf("relativeTo(%q) = %q, want %q", folder, got, want)
		}
	}
	tree := Layout{Root: Node{Type: "split", Direction: "right", Ratio: 0.5, First: &Node{Type: "pane", Dir: "web"}, Second: &Node{Type: "pane", Dir: `D:\elsewhere`}}}.Tree(dir)
	if tree.First == nil || tree.First.CWD != `C:\Users\me\app\web` || tree.Second.CWD != `D:\elsewhere` {
		t.Fatalf("laid out again: %+v / %+v", tree.First, tree.Second)
	}
}
