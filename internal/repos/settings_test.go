package repos

import (
	"strings"
	"testing"
)

func TestCheckRefusesWhatAWorktreeOrARunCouldNotTake(t *testing.T) {
	root := "/Users/me/code/app"
	cases := map[string]Settings{
		"a base git refuses":                {Base: "-x"},
		"a relative location":               {Location: "worktrees"},
		"a branch pattern git refuses":      {IssueBranch: "dev feat/{number}"},
		"an agent herdr does not know":      {Agent: "nobody"},
		"an unknown when":                   {SetupWhen: "sometimes"},
		"an action with no name":            {Actions: []Action{{Command: "npm run dev"}}},
		"an action with no command":         {Actions: []Action{{Name: "Dev"}}},
		"an action over two lines":          {Actions: []Action{{Name: "Dev", Command: "npm i\nnpm run dev"}}},
		"an action climbing out":            {Actions: []Action{{Name: "Dev", Command: "make", Folder: "../other"}}},
		"an action in a home":               {Actions: []Action{{Name: "Dev", Command: "make", Folder: "~/x"}}},
		"an action in an unknown place":     {Actions: []Action{{Name: "Dev", Command: "make", Place: "left"}}},
		"an action wearing an unknown icon": {Actions: []Action{{Name: "Dev", Command: "make", Icon: "rocket"}}},
		"a prompt too long":                 {IssuePrompt: strings.Repeat("x", MaxPrompt+1)},
	}
	for name, s := range cases {
		if _, err := Check(root, s); err == nil {
			t.Errorf("%s was kept", name)
		}
	}
}

func TestCheckTrimsAndFillsTheDefaults(t *testing.T) {
	s, err := Check("/Users/me/code/app", Settings{
		Base: " origin/main ", Location: "/Users/me/trees/", SetupWhen: "run",
		Actions: []Action{{Name: " Dev ", Command: " npm run dev ", Folder: "./"}, {Name: "Test", Command: "go test ./...", Folder: "web/", Place: "down", Icon: "circle-check"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Base != "origin/main" || s.Location != "/Users/me/trees" || s.SetupWhen != "" {
		t.Errorf("kept %+v", s)
	}
	dev, test := s.Actions[0], s.Actions[1]
	if dev.ID == "" || dev.ID == test.ID || dev.Name != "Dev" || dev.Command != "npm run dev" || dev.Folder != "" || dev.Place != "tab" || dev.Icon != "play" {
		t.Errorf("the first action is %+v", dev)
	}
	if test.Folder != "web" || test.Place != "down" {
		t.Errorf("the second action is %+v", test)
	}
}

func TestTheStoreKeepsEachRepositoryApart(t *testing.T) {
	dir := t.TempDir()
	st := Open(dir)
	saved, err := st.Put("local", "/Users/me/code/app/", Settings{Base: "main", Actions: []Action{{Name: "Dev", Command: "npm run dev"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put("gog", "/home/me/app", Settings{Setup: "pnpm i"}); err != nil {
		t.Fatal(err)
	}
	again := Open(dir)
	if got := again.Get("local", "/Users/me/code/app"); got.Base != "main" || len(got.Actions) != 1 {
		t.Errorf("the repository read back as %+v", got)
	}
	if a, ok := again.Action("local", "/Users/me/code/app", saved.Actions[0].ID); !ok || a.Command != "npm run dev" {
		t.Errorf("the action read back as %+v, %v", a, ok)
	}
	if _, ok := again.Action("gog", "/home/me/app", saved.Actions[0].ID); ok {
		t.Error("another machine's repository holds the first one's action")
	}
	again.Forget("gog")
	if got := Open(dir).Get("gog", "/home/me/app"); got.Setup != "" {
		t.Error("a removed machine's settings stayed")
	}
	if _, err := again.Put("local", "/Users/me/code/app", Settings{}); err != nil {
		t.Fatal(err)
	}
	if got := Open(dir).Get("local", "/Users/me/code/app"); got.Base != "" {
		t.Error("settings put back to nothing stayed")
	}
}

func TestATemplateIsFilledIn(t *testing.T) {
	got := Fill(DefaultIssuePrompt, 198, "Saved carts for signed-in shoppers", "acme/storefront", "dev/feat/198")
	want := "Work on GitHub issue #198 in acme/storefront: “Saved carts for signed-in shoppers”. Read it first with: gh issue view 198 -R acme/storefront --comments"
	if got != want {
		t.Errorf("got %q", got)
	}
	if got := Fill("dev/{number}-{slug}", 7, "Fix: the cart’s total (again)!", "", ""); got != "dev/7-fix-the-cart-s-total-again" {
		t.Errorf("the slug pattern made %q", got)
	}
	if got := TitleSlug(strings.Repeat("word ", 20)); len(got) > 40 || strings.HasSuffix(got, "-") {
		t.Errorf("a long title made %q", got)
	}
}
