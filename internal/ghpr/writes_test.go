package ghpr

import (
	"strings"
	"testing"
)

const head = "0123456789abcdef0123456789abcdef01234567"

func TestCommentsRepliesAndThreadsGoThroughGh(t *testing.T) {
	repo := Repo{"octo", "trips"}
	host := fakeGH([2]string{"gh ", answered(`{"data":{"ok":true}}`, "", 0)})
	if err := PostComment(host, repo, 482, "  Thanks — `retry` it's in.  "); err != nil {
		t.Fatal(err)
	}
	s := host.Scripts()[0]
	if !strings.Contains(s, "gh 'api' 'repos/octo/trips/issues/482/comments' '--method' 'POST' '--input' '-'") || inputOf(t, s)["body"] != "Thanks — `retry` it's in." {
		t.Fatalf("comment script = %q", s)
	}
	if err := Reply(host, "PRRT_1", "Done in the next push."); err != nil {
		t.Fatal(err)
	}
	in := inputOf(t, host.Scripts()[1])
	if !strings.Contains(in["query"].(string), "addPullRequestReviewThreadReply") || in["variables"].(map[string]any)["id"] != "PRRT_1" {
		t.Fatalf("reply = %v", in)
	}
	if err := Resolve(host, "PRRT_1", true); err != nil {
		t.Fatal(err)
	}
	if err := Resolve(host, "PRRT_1", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inputOf(t, host.Scripts()[2])["query"].(string), "resolveReviewThread(") || !strings.Contains(inputOf(t, host.Scripts()[3])["query"].(string), "unresolveReviewThread(") {
		t.Fatal("resolve and unresolve")
	}
	if err := AskAgain(host, repo, 482, "octo/mobile"); err != nil {
		t.Fatal(err)
	}
	if in := inputOf(t, host.Scripts()[4]); in["team_reviewers"] == nil || in["reviewers"] != nil {
		t.Fatalf("a team asked as a person: %v", in)
	}
	for _, bad := range []func() error{
		func() error { return PostComment(host, repo, 482, "   ") },
		func() error { return PostComment(host, repo, 482, strings.Repeat("x", MaxBody+1)) },
		func() error { return Reply(host, "PRRT_1'; rm", "x") },
		func() error { return AskAgain(host, repo, 482, "a b") },
	} {
		if bad() == nil {
			t.Fatal("a bad write went to gh")
		}
	}
	if len(host.Scripts()) != 5 {
		t.Fatalf("a refused write ran: %d scripts", len(host.Scripts()))
	}
}

func TestAMergeNamesTheHeadThePersonSaw(t *testing.T) {
	repo := Repo{"octo", "trips"}
	host := fakeGH([2]string{"graphql", answered(`{"data":{"enablePullRequestAutoMerge":{"pullRequest":{"id":"PR_kwDOAbc123"}}}}`, "", 0)}, [2]string{"gh ", answered("", "", 0)})
	if err := ChangeState(host, repo, 482, Change{Action: "squash", Head: head, Subject: "Save trips for offline use (#482)", Body: "Trips can be saved.\n\n- a button"}); err != nil {
		t.Fatal(err)
	}
	s := host.Scripts()[0]
	if !strings.Contains(s, "gh 'pr' 'merge' '482' '--repo' 'octo/trips' '--squash' '--match-head-commit' '"+head+"' '--subject' 'Save trips for offline use (#482)' '--body' 'Trips can be saved.\n\n- a button'") {
		t.Fatalf("squash = %q", s)
	}
	if err := ChangeState(host, repo, 482, Change{Action: "merge"}); err == nil {
		t.Fatal("a merge without the head seen went to gh")
	}
	if err := ChangeState(host, repo, 482, Change{Action: "auto", Head: head, ID: "PR_kwDOAbc123", Method: "squash"}); err != nil {
		t.Fatal(err)
	}
	in := inputOf(t, host.Scripts()[1])
	vars := in["variables"].(map[string]any)
	if !strings.Contains(in["query"].(string), "enablePullRequestAutoMerge") || vars["head"] != head || vars["method"] != "SQUASH" {
		t.Fatalf("auto = %v", in)
	}
	for action, want := range map[string]string{"close": "gh 'pr' 'close' '482' '--repo' 'octo/trips'", "reopen": "gh 'pr' 'reopen' '482'", "cancel-auto": "'--disable-auto'"} {
		if err := ChangeState(host, repo, 482, Change{Action: action}); err != nil {
			t.Fatal(err)
		}
		if got := host.Scripts(); !strings.Contains(got[len(got)-1], want) {
			t.Fatalf("%s = %q", action, got[len(got)-1])
		}
	}
	if err := ChangeState(host, repo, 482, Change{Action: "force"}); err == nil {
		t.Fatal("an unknown change ran")
	}
	moved := fakeGH([2]string{"gh ", answered("", "GraphQL: Head branch was modified. Review and try the merge again. (mergePullRequest)", 1)})
	if err := ChangeState(moved, repo, 482, Change{Action: "merge", Head: head}); err != ErrNewerHead {
		t.Fatalf("a newer push was not said: %v", err)
	}
	blocked := fakeGH([2]string{"gh ", answered("", "X Pull request octo/trips#482 is not mergeable: the base branch policy prohibits the merge.", 1)})
	if err := ChangeState(blocked, repo, 482, Change{Action: "rebase", Head: head}); err == nil || !strings.Contains(err.Error(), "base branch policy") {
		t.Fatalf("gh's refusal was not passed on: %v", err)
	}
}
