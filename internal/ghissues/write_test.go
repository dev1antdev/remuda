package ghissues

import (
	"errors"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestACommentTravelsAsAFileAndTheListIsReadAgain(t *testing.T) {
	reads := 0
	host := newFake(func(script string) (string, error) {
		switch {
		case strings.Contains(script, "mktemp"):
			return "/tmp/remuda-issue.abc123\n", nil
		case strings.Contains(script, "gh issue list"):
			reads++
			return "alice\n" + rs + headers200 + rs + listJSON, nil
		}
		return "", nil
	})
	s, _ := serviceOn(host)
	if _, err := s.List("gog", "acme/widgets", DefaultList, false); err != nil {
		t.Fatal(err)
	}
	text := "It's fixed on main.\n\n```\n$ rm -rf build; echo `boom`\n```"
	if err := s.Comment("gog", "acme/widgets", 41, text); err != nil {
		t.Fatal(err)
	}
	if host.put["/tmp/remuda-issue.abc123"] != text {
		t.Fatalf("the comment's file: %q", host.put)
	}
	script := host.Scripts()[len(host.Scripts())-1]
	if strings.Contains(script, "boom") || !strings.Contains(script, `gh issue comment 41 -R 'acme/widgets' --body-file "$f"`) || !strings.Contains(script, `trap 'rm -f "$f"' EXIT`) {
		t.Fatalf("the comment's script:\n%s", script)
	}
	if _, err := s.List("gog", "acme/widgets", DefaultList, false); err != nil || reads != 2 {
		t.Fatalf("the list kept from before the comment was served: %d reads, %v", reads, err)
	}
	if err := s.Comment("gog", "acme/widgets", 41, "  \n "); !errors.Is(err, ErrEmpty) {
		t.Fatalf("an empty comment: %v", err)
	}
	if err := s.Comment("gog", "acme/widgets", 41, strings.Repeat("x", MaxBody+1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a comment past GitHub's size: %v", err)
	}
}

func TestACloseSaysItsReasonAndPostsTheCommentFirst(t *testing.T) {
	host := newFake(func(script string) (string, error) {
		if strings.Contains(script, "mktemp") {
			return "/tmp/remuda-issue.x\n", nil
		}
		return "", nil
	})
	s, _ := serviceOn(host)
	if err := s.Close("gog", "acme/widgets", 41, "not_planned", "Duplicate of #40."); err != nil {
		t.Fatal(err)
	}
	script := host.Scripts()[len(host.Scripts())-1]
	comment := strings.Index(script, "gh issue comment 41")
	closing := strings.Index(script, "gh issue close 41 -R 'acme/widgets' --reason 'not planned'")
	if comment < 0 || closing < comment {
		t.Fatalf("close with comment:\n%s", script)
	}
	if err := s.Close("gog", "acme/widgets", 41, "", ""); err != nil {
		t.Fatal(err)
	}
	if script := host.Scripts()[len(host.Scripts())-1]; !strings.Contains(script, "--reason 'completed'") || strings.Contains(script, "comment") {
		t.Fatalf("a plain close:\n%s", script)
	}
	if err := s.Close("gog", "acme/widgets", 41, "wontfix", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a reason gh does not take: %v", err)
	}
	if err := s.Reopen("gog", "acme/widgets", 41, ""); err != nil {
		t.Fatal(err)
	}
	if script := host.Scripts()[len(host.Scripts())-1]; !strings.Contains(script, "gh issue reopen 41 -R 'acme/widgets'") {
		t.Fatalf("reopen:\n%s", script)
	}
	for me, flag := range map[bool]string{true: "--add-assignee @me", false: "--remove-assignee @me"} {
		if err := s.Assign("gog", "acme/widgets", 41, me); err != nil {
			t.Fatal(err)
		}
		if script := host.Scripts()[len(host.Scripts())-1]; !strings.Contains(script, "gh issue edit 41 -R 'acme/widgets' "+flag) {
			t.Fatalf("assign %v:\n%s", me, script)
		}
	}
}

func TestAWriteGhRefusesSaysWhatGhSaid(t *testing.T) {
	host := newFake(func(string) (string, error) {
		return "GraphQL: Resource not accessible by integration (updateIssue)\n", &shell.ExitError{Code: 1}
	})
	s, _ := serviceOn(host)
	err := s.Reopen("gog", "acme/widgets", 41, "")
	var refused *Refused
	if !errors.As(err, &refused) || !strings.Contains(err.Error(), "Resource not accessible by integration") || !strings.Contains(err.Error(), "GOG") {
		t.Fatalf("%v", err)
	}
	host = newFake(func(string) (string, error) {
		return "To get started with GitHub CLI, please run:  gh auth login\n", &shell.ExitError{Code: 4}
	})
	s, _ = serviceOn(host)
	var trouble *Trouble
	if err := s.Assign("gog", "acme/widgets", 41, true); !errors.As(err, &trouble) || trouble.Kind != "signed-out" {
		t.Fatalf("a signed-out gh's write: %v", err)
	}
}
