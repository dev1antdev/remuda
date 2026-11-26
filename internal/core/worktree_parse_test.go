package core

import (
	"testing"

	"github.com/dev1antdev/remuda/internal/coreapi"
)

func TestACheckoutIsReadAsHerdrReportsIt(t *testing.T) {
	cases := []struct {
		name, out, path string
		want            *coreapi.WorktreeInfo
	}{
		{"a main checkout", "top /p/api\ncommon /p/api/.git\nown /p/api/.git\nmain /p/api\n", "",
			&coreapi.WorktreeInfo{CheckoutPath: "/p/api", RepoKey: "/p/api/.git", RepoName: "api", RepoRoot: "/p/api"}},
		{"a linked worktree just made", "path /h/.herdr/worktrees/api/feat-x\ntop /h/.herdr/worktrees/api/feat-x\ncommon /p/api/.git\nown /p/api/.git/worktrees/feat-x\nmain /p/api\n",
			"/h/.herdr/worktrees/api/feat-x",
			&coreapi.WorktreeInfo{CheckoutPath: "/h/.herdr/worktrees/api/feat-x", IsLinkedWorktree: true, RepoKey: "/p/api/.git", RepoName: "api", RepoRoot: "/p/api"}},
		{"a Windows PC's, as Git Bash's git spells it", "top C:/Users/me/app\r\ncommon C:/Users/me/app/.git\r\nown C:/Users/me/app/.git\r\nmain C:/Users/me/app\r\n", "",
			&coreapi.WorktreeInfo{CheckoutPath: `C:\Users\me\app`, RepoKey: `C:\Users\me\app\.git`, RepoName: "app", RepoRoot: `C:\Users\me\app`}},
		{"a folder outside git", "", "", nil},
	}
	for _, c := range cases {
		path, got := parseCheckout(c.out)
		if path != c.path {
			t.Errorf("%s: path %q", c.name, path)
		}
		switch {
		case c.want == nil && got != nil:
			t.Errorf("%s: read as %+v", c.name, got)
		case c.want != nil && (got == nil || *got != *c.want):
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}
