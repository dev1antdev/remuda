package transcript

import "testing"

func TestPullRequestIsTheLastOneTheAssistantNamed(t *testing.T) {
	text := func(role, s string) Turn { return Turn{Role: role, Parts: []Part{{Type: "text", Text: s}}} }
	turns := []Turn{
		text("user", "open a PR"),
		text("assistant", "Opened https://github.com/dev1antdev/remuda/pull/15 and then https://github.com/dev1antdev/remuda/pull/16."),
		{Role: "assistant", Parts: []Part{{Type: "tool", ToolName: "Bash", Output: "https://github.com/dev1antdev/remuda/pull/99"}}},
		text("user", "see https://github.com/dev1antdev/remuda/pull/77"),
	}
	link, ok := PullRequest(turns)
	if !ok || link.URL != "https://github.com/dev1antdev/remuda/pull/16" || link.Label != "PR #16" {
		t.Fatalf("got %+v %v", link, ok)
	}
	gitlab := []Turn{text("assistant", "MR открыт: [!9](https://gitlab.gog.com/gog/user-profiles/-/merge_requests/9/diffs), смотри.")}
	link, ok = PullRequest(gitlab)
	if !ok || link.URL != "https://gitlab.gog.com/gog/user-profiles/-/merge_requests/9" || link.Label != "MR !9" {
		t.Fatalf("gitlab: got %+v %v", link, ok)
	}
	if _, ok := PullRequest([]Turn{text("assistant", "No request here, https://github.com/dev1antdev/remuda/issues/3.")}); ok {
		t.Fatal("an issue is not a pull request")
	}
	if _, ok := PullRequest(nil); ok {
		t.Fatal("nothing from nothing")
	}
}
