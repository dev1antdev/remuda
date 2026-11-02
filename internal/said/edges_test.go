package said

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFirstLineOnMarkdownEdges(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"a fence never closed hides what follows it", "```go\nfunc main() {}\nstill code", ""},
		{"words before an unclosed fence", "Here it is:\n```\ncode", "Here it is:"},
		{"a heading only", "### Summary", "Summary"},
		{"a quote", "> Tests pass on main.", "Tests pass on main."},
		{"a nested quote and a list", "> > - nested item", "nested item"},
		{"a link alone", "[PR #15](https://github.com/o/r/pull/15)", "PR #15"},
		{"an image alone, then words", "![](shot.png)\nThe page renders.", "The page renders."},
		{"a tilde fence", "~~~\nx\n~~~\nAfter.", "After."},
		{"CRLF line ends", "Done.\r\nMore.", "Done."},
		{"a heading after blank rows", "\n\n\n# Title\nbody", "Title"},
	}
	for _, c := range cases {
		if got := FirstLine(c.in); got != c.want {
			t.Errorf("%s: FirstLine(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestFirstLineCutsWideTextAtTheLimit(t *testing.T) {
	cjk := strings.Repeat("测试通过了", 40) // no spaces: cut flat at Max characters
	got := FirstLine(cjk)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != Max+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("CJK cut: %d runes %q", utf8.RuneCountInString(got), got)
	}
	emoji := strings.Repeat("✅ done ", 30)
	got = FirstLine(emoji)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) > Max+1 || !strings.HasSuffix(got, "…") || strings.HasSuffix(got, " …") {
		t.Fatalf("emoji cut: %q", got)
	}
	exact := strings.Repeat("a", Max)
	if got := FirstLine(exact); got != exact {
		t.Fatalf("a line of exactly Max characters was cut: %q", got)
	}
}
