package undo

import "testing"

func TestUndoTakesPathsGitQuotes(t *testing.T) {
	names := []string{"my file.txt", "quo\"te.txt", "tab\there.txt", "ünï.txt"}
	dir := repo(t, map[string]string{names[0]: "one\n", names[1]: "q\n", names[2]: "t\n", names[3]: "u\n"})
	f := newFixture(t)
	turn := f.turn(t, dir, func() {
		for _, path := range names {
			write(t, dir, path, "agent\n")
		}
		write(t, dir, "new dir/new \"file\".txt", "agent\n")
	})
	if len(turn.Files) != 5 {
		t.Fatalf("files %+v", turn.Files)
	}
	if _, err := f.service.Undo("local", turn.ID, false); err != nil {
		t.Fatalf("undo: %v", err)
	}
	want := map[string]string{names[0]: "one\n", names[1]: "q\n", names[2]: "t\n", names[3]: "u\n", "new dir/new \"file\".txt": "<gone>"}
	for path, text := range want {
		if got := readFile(t, dir, path); got != text {
			t.Errorf("%q is %q", path, got)
		}
	}
}

func TestHeaderPathReadsBothPrefixesAndQuotes(t *testing.T) {
	for header, want := range map[string]string{
		"diff --git a/x b/x\n":                            "x",
		"diff --git b/a b/c a/a b/c\n":                    "a b/c",
		"diff --git \"a/t\\tq\\\"\" \"b/t\\tq\\\"\"\n":    "t\tq\"",
		"diff --git \"a/\\303\\274\" \"b/\\303\\274\"\n":  "ü",
		"diff --git a/new dir/f.txt b/new dir/f.txt\nnew": "new dir/f.txt",
	} {
		if got, ok := headerPath(header); !ok || got != want {
			t.Errorf("%q: %q %v, want %q", header, got, ok, want)
		}
	}
	if _, ok := headerPath("diff --git a/x b/y\n"); ok {
		t.Error("two different paths read as one")
	}
}
