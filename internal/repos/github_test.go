package repos

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

// The answers gh gives, written from GitHub's documented JSON for an anonymized repository.
const (
	issuesJSON = `[{"number":198,"title":"Saved carts for signed-in shoppers"},{"number":203,"title":"Checkout redesign"}]`
	prsJSON    = `[{"number":212,"title":"Add saved carts","headRefName":"dev/feat/198","body":"","closingIssuesReferences":[{"number":198}],"isCrossRepository":false},` +
		`{"number":207,"title":"Redesign the checkout","headRefName":"checkout","body":"This fixes #203 and closes #9.","closingIssuesReferences":[],"isCrossRepository":true}]`
)

// standIn puts a gh of the test's own first on the scripts' PATH (they put $HOME/.local/bin first), answering as gh
// does and warning on stderr the way gh's update notice would.
func standIn(t *testing.T, body string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\necho 'A new release of gh is available' >&2\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestTheOpenIssuesAndPullRequestsAreReadWithGH(t *testing.T) {
	standIn(t, `case "$1 $2" in
  "issue list") printf '%s\n' '`+issuesJSON+`' ;;
  "pr list") printf '%s\n' '`+prsJSON+`' ;;
esac
`)
	lists, err := ListOpen(shell.Local{Name: "local"}, "acme/storefront")
	if err != nil {
		t.Fatal(err)
	}
	if len(lists.Issues) != 2 || lists.Issues[0].Number != 198 {
		t.Errorf("the issues are %+v", lists.Issues)
	}
	if len(lists.PRs) != 2 {
		t.Fatalf("the pull requests are %+v", lists.PRs)
	}
	first, second := lists.PRs[0], lists.PRs[1]
	if first.Head != "dev/feat/198" || len(first.Closes) != 1 || first.Closes[0] != 198 {
		t.Errorf("the first pull request is %+v", first)
	}
	if !second.CrossRepo || len(second.Closes) != 2 || second.Closes[0] != 203 || second.Closes[1] != 9 {
		t.Errorf("the second pull request is %+v", second)
	}
}

func TestANumberIsLookedUp(t *testing.T) {
	standIn(t, `case "$1" in
  api) printf 'true\tOPEN\tFix the cart\n' ;;
  pr) printf '%s\n' '{"headRefName":"cart-fix","isCrossRepository":false}' ;;
esac
`)
	found, err := Lookup(shell.Local{Name: "local"}, "acme/storefront", 301)
	if err != nil {
		t.Fatal(err)
	}
	if found.Kind != "pr" || found.Number != 301 || found.Title != "Fix the cart" || found.Head != "cart-fix" || found.State != "open" {
		t.Errorf("found %+v", found)
	}
	issue, err := ParseLookup(77, "false\tCLOSED\tOld bug\n\x1e\n")
	if err != nil || issue.Kind != "issue" || issue.State != "closed" {
		t.Errorf("an issue reads as %+v, %v", issue, err)
	}
}

func TestNoGHOrAFailingOneSaysSo(t *testing.T) {
	none := &shell.Fake{Name: "gog", Answer: func(string) (string, error) { return "no gh\n", &shell.ExitError{Code: 4} }}
	if _, err := ListOpen(none, "acme/storefront"); !errors.Is(err, ErrNoGH) {
		t.Errorf("a machine without gh answered %v", err)
	}
	refused := &shell.Fake{Name: "gog", Answer: func(string) (string, error) {
		return "To get started with GitHub CLI, please run:  gh auth login\n", &shell.ExitError{Code: 5}
	}}
	if _, err := ListOpen(refused, "acme/storefront"); err == nil || !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("a gh not signed in answered %v", err)
	}
	if _, err := ListOpen(none, "not a repo"); err == nil {
		t.Error("a name that is no owner/repo reached gh")
	}
}
