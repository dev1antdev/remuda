package ghissues

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Close with comment and Reopen with comment post the comment first, as GitHub's own buttons do. When gh then refuses
// the close or the reopen — anyone may comment on a public repository's issue, few may close it — the comment is on
// GitHub already: the answer says so, so the page neither claims nothing happened nor offers the same comment again.
func TestACommentPostedBeforeARefusedReopenIsSaidPosted(t *testing.T) {
	home, s := standIn(t)
	err := s.Reopen("here", "acme/widgets", 41, "Back again.")
	if posted, _ := os.ReadFile(filepath.Join(home, "posted.txt")); string(posted) != "Back again." {
		t.Fatalf("the comment was not posted first: %q", posted)
	}
	var partly *Partly
	if !errors.As(err, &partly) || !strings.Contains(err.Error(), "comment is posted") || !strings.Contains(err.Error(), "Could not reopen") {
		t.Fatalf("a reopen refused after its comment went: %v", err)
	}
	if strings.Contains(err.Error(), "\x1e") {
		t.Fatalf("the script's mark leaked into the words: %q", err)
	}
	if err := s.Reopen("here", "acme/widgets", 41, ""); err == nil || errors.As(err, &partly) {
		t.Fatalf("a reopen with no comment, refused, said a comment was posted: %v", err)
	}
}
