package ghpr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

// answered is what ghScript prints for a gh call that ended with code, its stdout and stderr apart.
func answered(stdout, stderr string, code int) string {
	return stdout + "\x1e\n" + stderr + "\n\x1e\n" + itoa(code) + "\n"
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// fakeGH is a machine whose gh answers each script by the first rule whose words the script holds.
func fakeGH(rules ...[2]string) *shell.Fake {
	return &shell.Fake{Name: "box", Answer: func(s string) (string, error) {
		for _, r := range rules {
			if strings.Contains(s, r[0]) {
				return r[1], nil
			}
		}
		return answered("", "unexpected call", 1), nil
	}}
}

// inputOf is the JSON a script hands gh on its stdin.
func inputOf(t *testing.T, s string) map[string]any {
	t.Helper()
	_, rest, ok := strings.Cut(s, "<<'REMUDA_INPUT'\n")
	if !ok {
		t.Fatalf("no input in %q", s)
	}
	line, _, _ := strings.Cut(rest, "\n")
	var v map[string]any
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		t.Fatalf("input %q: %v", line, err)
	}
	return v
}

func TestGhsAnswerIsReadApartAndItsTroubleNamed(t *testing.T) {
	got, ok := parseAnswer(answered(`{"a":1}`+"\n", "a warning", 0))
	if !ok || got.code != 0 || strings.TrimSpace(got.out) != `{"a":1}` || got.err != "a warning" {
		t.Fatalf("answer = %+v", got)
	}
	missing := fakeGH([2]string{"gh ", "\x1enogh\n"})
	if _, err := run(missing, []string{"api", "user"}, ""); err == nil {
		t.Fatal("no gh went unnoticed")
	} else if p, ok := AsProblem(err); !ok || p.Kind != "missing" || p.Command != "gh auth login" || p.Machine != "box" {
		t.Fatalf("problem = %+v", err)
	}
	out := fakeGH([2]string{"gh ", answered("", "To get started with GitHub CLI, please run:  gh auth login", 4)})
	if _, err := run(out, []string{"api", "user"}, ""); err == nil {
		t.Fatal("signed out went unnoticed")
	} else if p, ok := AsProblem(err); !ok || p.Kind != "signed-out" {
		t.Fatalf("problem = %v", err)
	}
	refused := fakeGH([2]string{"gh ", answered(`{"message":"Not Found","documentation_url":"x"}`, "gh: Not Found (HTTP 404)", 1)})
	if _, err := run(refused, []string{"api", "repos/o/r"}, ""); err == nil || err.Error() != "Not Found (HTTP 404)" {
		t.Fatalf("err = %v", err)
	}
	s := ghScript([]string{"api", "graphql", "--input", "-"}, `{"q":"it's"}`, "")
	if !strings.Contains(s, "gh 'api' 'graphql' '--input' '-' 2>\"$err\" <<'REMUDA_INPUT'\n{\"q\":\"it's\"}\nREMUDA_INPUT") {
		t.Fatalf("script = %q", s)
	}
	if s := ghScript([]string{"run", "view"}, "", lastBytes(10)); !strings.Contains(s, `tail -c 10 "$out"`) {
		t.Fatalf("a cut script = %q", s)
	}
}

func TestGraphQLErrorsAreGitHubsWords(t *testing.T) {
	host := fakeGH([2]string{"graphql", answered(`{"data":{"repository":null},"errors":[{"message":"Could not resolve to a Repository with the name 'o/x'."}]}`, "gh: Could not resolve", 1)})
	var v any
	if err := gql(host, "query{x}", nil, &v); err == nil || !strings.Contains(err.Error(), "Could not resolve to a Repository") {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoriesAndAddressesAreReadStrictly(t *testing.T) {
	for url, want := range map[string]string{
		"git@github.com:octo/app.git":           "octo/app",
		"https://github.com/octo/app":           "octo/app",
		"https://x@github.com/octo/app.git/":    "octo/app",
		"ssh://git@ssh.github.com:443/octo/app": "octo/app",
		"git@github-work:octo/app.git":          "octo/app",
		"git@gitlab.com:octo/app.git":           "",
		"https://github.com/octo/app/extra":     "",
		"https://github.com/../app":             "",
	} {
		if got := RepoOfRemote(url); got != want {
			t.Errorf("%s → %q, want %q", url, got, want)
		}
	}
	if _, err := ParseRepo("octo/app; rm -rf /"); err == nil {
		t.Fatal("a repository with a shell in it was taken")
	}
	repo, n, ok := FromURL("https://github.com/octo/app/pull/482/files")
	if !ok || repo.String() != "octo/app" || n != 482 {
		t.Fatalf("from url = %v %d %v", repo, n, ok)
	}
	if _, _, ok := FromURL("https://github.com/octo/app/issues/3"); ok {
		t.Fatal("an issue read as a pull request")
	}
	c := ParseCheckout("branch offline-trips\ntracking fork\nremote fork git@github.com:sam/app.git\nremote origin https://github.com/octo/app.git\nremote upstream git@gitlab.com:octo/app.git\n")
	if c.Branch != "offline-trips" || c.Repo != "sam/app" || len(c.Remotes) != 3 || c.Remotes[2].Repo != "" {
		t.Fatalf("checkout = %+v", c)
	}
	if !strings.Contains(CheckoutScript("/w/app"), "git --no-optional-locks -c core.fsmonitor=false symbolic-ref") {
		t.Fatal("the checkout's read can take the index lock")
	}
}
