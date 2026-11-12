package collisions

import (
	"reflect"
	"testing"
)

func TestOneRepositoryIsOneKeyOverSshAndHttps(t *testing.T) {
	ssh, https := RepoKey("gog", "/home/a/app", "git@github.com:Example/app.git"), RepoKey("mac", "/Users/a/app", "https://github.com/Example/app")
	if ssh == "" || ssh != https {
		t.Fatalf("one remote, two spellings: %q vs %q", ssh, https)
	}
	if local := RepoKey("mac", "/Users/a/notes", ""); local != "root mac|/Users/a/notes" {
		t.Fatalf("a repository with no remote is its main folder on its machine: %q", local)
	}
	if none := RepoKey("mac", "", ""); none != "" {
		t.Fatalf("outside git: %q", none)
	}
}

func TestGroupsAreTheRepositoriesTwoActiveFoldersShare(t *testing.T) {
	workspaces := []Workspace{
		{Machine: "local", ID: "w1", Dir: "/app", Repo: "r", Agents: true},
		{Machine: "local", ID: "w2", Dir: "/app", Repo: "r", Agents: true}, // a second workspace on the same folder: one checkout
		{Machine: "local", ID: "w3", Dir: "/notes", Repo: "n", Agents: true},
		{Machine: "local", ID: "w4", Dir: "/notes-old", Repo: "n"}, // parked: no agent, nothing uncommitted
		{Machine: "gog", ID: "w5", Dir: "/srv/app", Repo: "r", Changed: 2},
		{Machine: "local", ID: "w6", Dir: "/tmp", Agents: true}, // outside git
	}
	groups := Groups(workspaces)
	if len(groups) != 1 {
		t.Fatalf("groups: %+v", groups)
	}
	var ids []string
	for _, ws := range groups[0] {
		ids = append(ids, ws.ID)
	}
	if want := []string{"w5", "w1", "w2"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("members: %v, want %v (by machine and folder)", ids, want)
	}
	if got := Groups(workspaces[:2]); len(got) != 0 {
		t.Fatalf("two workspaces on one folder are no collision: %+v", got)
	}
}

func TestHunksTouchOnTheSameLinesOrTheLinesBesideThem(t *testing.T) {
	change := func(hunks ...Hunk) Change { return Change{Blob: "x", Hunks: hunks} }
	other := func(hunks ...Hunk) Change { return Change{Blob: "y", Hunks: hunks} }
	cases := []struct {
		name     string
		a, b     Change
		lines    bool
		from, to int
	}{
		{"the same lines", change(Hunk{Start: 40, Count: 10}), other(Hunk{Start: 45, Count: 3}), true, 40, 49},
		{"lines side by side", change(Hunk{Start: 5, Count: 1}), other(Hunk{Start: 6, Count: 1}), true, 5, 6},
		{"a line between", change(Hunk{Start: 5, Count: 1}), other(Hunk{Start: 7, Count: 1}), false, 0, 0},
		{"an insertion after a changed line", change(Hunk{Start: 5, Count: 0}), other(Hunk{Start: 6, Count: 1}), true, 5, 6},
		{"an insertion with a line between", change(Hunk{Start: 5, Count: 0}), other(Hunk{Start: 7, Count: 2}), false, 0, 0},
		{"far apart", change(Hunk{Start: 1, Count: 3}, Hunk{Start: 200, Count: 1}), other(Hunk{Start: 100, Count: 5}), false, 0, 0},
		{"one of several", change(Hunk{Start: 1, Count: 3}, Hunk{Start: 99, Count: 2}), other(Hunk{Start: 100, Count: 5}), true, 99, 104},
	}
	for _, c := range cases {
		got, ok := overlap("a.go", c.a, c.b)
		if !ok || got.Lines != c.lines || got.From != c.from || got.To != c.to {
			t.Errorf("%s: %+v %v", c.name, got, ok)
		}
	}
	if _, ok := overlap("a.go", Change{Blob: "same", Hunks: []Hunk{{Start: 1, Count: 1}}}, Change{Blob: "same", Hunks: []Hunk{{Start: 1, Count: 1}}}); ok {
		t.Fatal("two works leaving a file the same blob merge cleanly")
	}
	if _, ok := overlap("a.go", Change{Blob: Gone, Hunks: []Hunk{{Start: 1, Count: 9}}}, Change{Blob: Gone, Hunks: []Hunk{{Start: 1, Count: 9}}}); ok {
		t.Fatal("two deletions of one file merge cleanly")
	}
	if got, ok := overlap("logo.png", Change{Whole: true}, change(Hunk{Start: 1, Count: 1})); !ok || !got.Lines || got.From != 0 {
		t.Fatalf("a file changed whole touches every line: %+v", got)
	}
}

func TestCompareListsTheSharedLinesFirst(t *testing.T) {
	a := Work{Files: map[string]Change{
		"b.go":    {Blob: "1", Hunks: []Hunk{{Start: 10, Count: 2}}},
		"a.go":    {Blob: "1", Hunks: []Hunk{{Start: 1, Count: 1}}},
		"z.go":    {Blob: "1", Hunks: []Hunk{{Start: 50, Count: 1}}},
		"only.go": {Blob: "1", Hunks: []Hunk{{Start: 1, Count: 1}}},
	}}
	b := Work{Files: map[string]Change{
		"b.go": {Blob: "2", Hunks: []Hunk{{Start: 300, Count: 2}}},
		"a.go": {Blob: "2", Hunks: []Hunk{{Start: 90, Count: 1}}},
		"z.go": {Blob: "2", Hunks: []Hunk{{Start: 50, Count: 3}}},
	}}
	var got []string
	for _, o := range Compare(a, b) {
		got = append(got, o.Path)
	}
	if want := []string{"z.go", "a.go", "b.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order: %v, want %v", got, want)
	}
}

func TestTheOneCloserToDoneLandsFirst(t *testing.T) {
	cases := []struct {
		a, b  Standing
		first bool
		why   string
	}{
		{Standing{PR: true, Working: true, Lines: 900, Name: "web"}, Standing{Lines: 10, Name: "api"}, true, WhyPR},
		{Standing{Working: true, Lines: 10, Name: "web"}, Standing{Lines: 900, Name: "api"}, false, WhyRest},
		{Standing{Lines: 120, Name: "web"}, Standing{Lines: 300, Name: "api"}, true, WhySmaller},
		{Standing{Lines: 12, Name: "Web"}, Standing{Lines: 12, Name: "api"}, false, WhyName},
	}
	for _, c := range cases {
		if first, why := Order(c.a, c.b); first != c.first || why != c.why {
			t.Errorf("Order(%+v, %+v) = %v %s, want %v %s", c.a, c.b, first, why, c.first, c.why)
		}
		if first, why := Order(c.b, c.a); first == c.first || why != c.why {
			t.Errorf("the other way round: %v %s", first, why)
		}
	}
}

func TestWordsSayWhereAndWhoGoesFirst(t *testing.T) {
	title, body := Words("web", "api", Collision{Files: []Overlap{{Path: "src/auth.ts", Lines: true, From: 40, To: 58}, {Path: "go.mod"}}, More: 1, Why: WhyPR})
	if title != "web and api change the same lines" || body != "src/auth.ts, lines 40–58, and 2 more files. Land web first: it has a pull request open; api rebases on it." {
		t.Fatalf("%q / %q", title, body)
	}
	if _, body := Words("a", "b", Collision{Files: []Overlap{{Path: "x.png", Lines: true}}, Why: WhyName}); body != "x.png (the whole file). Land a first; b rebases on it." {
		t.Fatalf("%q", body)
	}
}
