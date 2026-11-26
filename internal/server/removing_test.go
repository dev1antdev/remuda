package server

import "testing"

func TestAFolderBeingRemovedRefusesAWorkspaceUntilTheRemovalEnds(t *testing.T) {
	var r removing
	done := r.begin("m1", "/home/me/.herdr/worktrees/app/fix")
	if !r.under("m1", "/home/me/.herdr/worktrees/app/fix") {
		t.Fatal("the folder being removed is open")
	}
	if !r.under("m1", "/home/me/.herdr/worktrees/app/fix/sub") {
		t.Fatal("a folder inside it is open")
	}
	if r.under("m2", "/home/me/.herdr/worktrees/app/fix") {
		t.Fatal("another machine's folder is taken for this one's")
	}
	if r.under("m1", "/home/me/.herdr/worktrees/app") {
		t.Fatal("its parent is refused")
	}
	done()
	if r.under("m1", "/home/me/.herdr/worktrees/app/fix") {
		t.Fatal("the end of the removal left the folder held")
	}
}

func TestTwoRemovalsOfOneFolderHoldItUntilBothEnd(t *testing.T) {
	var r removing
	first := r.begin("m1", "/w")
	second := r.begin("m1", "/w/")
	first()
	if !r.under("m1", "/w") {
		t.Fatal("the second removal's hold was dropped by the first's end")
	}
	second()
	if r.under("m1", "/w") {
		t.Fatal("the folder stayed held")
	}
}
