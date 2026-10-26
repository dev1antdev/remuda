package tasks

import (
	"strings"
	"testing"
)

func TestMarkdownHTMLDrawsEveryConstruct(t *testing.T) {
	md := `# Title
Two lines
of one paragraph.

## Steps
- first with **bold** and *italic* and ` + "`code`" + `
- second [link](https://example.com/a?b=1) and https://remuda.dev/x.
  - nested
- [ ] open item
- [x] done item

1. one
2) two

> a quote
> goes on

---

` + "```go\nfunc main() { fmt.Println(\"<hi>\") }\n```" + `

Last words_with_underscores and a * lone star.`
	got := MarkdownHTML(md)
	for _, want := range []string{
		"<h1>Title</h1>",
		"<p>Two lines<br>of one paragraph.</p>",
		"<h2>Steps</h2>",
		"<ul><li>first with <strong>bold</strong> and <em>italic</em> and <code>code</code></li>",
		`<li>second <a href="https://example.com/a?b=1">link</a> and <a href="https://remuda.dev/x">https://remuda.dev/x</a>.<ul><li>nested</li></ul></li>`,
		`<li><input type="checkbox"> open item</li>`,
		`<li><input type="checkbox" checked> done item</li></ul>`,
		"<ol><li>one</li><li>two</li></ol>",
		"<blockquote><p>a quote goes on</p></blockquote>",
		"<hr>",
		"<pre><code>func main() { fmt.Println(&#34;&lt;hi&gt;&#34;) }</code></pre>",
		"<p>Last words_with_underscores and a * lone star.</p>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestMarkdownHTMLEscapesAndGuards(t *testing.T) {
	got := MarkdownHTML("hello <script>alert(1)</script> & [x](javascript:alert(1)) [y](http://ok) \"q\"")
	if strings.Contains(got, "<script") || !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("a tag in the text was not escaped: %s", got)
	}
	if strings.Contains(got, `href="javascript`) || !strings.Contains(got, "[x](javascript:alert(1))") {
		t.Errorf("a javascript link was drawn instead of kept as text: %s", got)
	}
	if !strings.Contains(got, `<a href="http://ok">y</a>`) || !strings.Contains(got, "&amp;") || !strings.Contains(got, "&quot;q&quot;") {
		t.Errorf("escaping or the http link is off: %s", got)
	}
	if MarkdownHTML("") != "" || MarkdownHTML("\n\n") != "" {
		t.Errorf("an empty text is not empty HTML")
	}
	long := strings.Repeat("a", MaxMarkdown+100)
	if got := MarkdownHTML(long); len(got) > MaxMarkdown+len("<p></p>") {
		t.Errorf("the cap did not hold: %d", len(got))
	}
	if got := MarkdownHTML("```\nnever closed"); got != "<pre><code>never closed</code></pre>" {
		t.Errorf("an unclosed fence: %s", got)
	}
}

func TestSourceNamesTheAgentOnly(t *testing.T) {
	if Source("", "w3:p1") != "" {
		t.Errorf("a person's task took a source")
	}
	if got := Source(" GOG ", " w3:p1 "); got != "GOG · w3:p1" {
		t.Errorf("got %q", got)
	}
	if got := Source("GOG", ""); got != "GOG" {
		t.Errorf("got %q", got)
	}
	if got := Source("GOG", strings.Repeat("x", 200)); len(got) != MaxSource {
		t.Errorf("the cap did not hold: %d", len(got))
	}
}

func TestCreateFromKeepsTheSourceAndPatchesNeverChangeIt(t *testing.T) {
	st := open(t)
	ps, _ := st.Projects()
	task, err := st.CreateFrom(ps[0].ID, Patch{Title: str("By an agent")}, "GOG · w3:p1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Source != "GOG · w3:p1" {
		t.Fatalf("source %q", task.Source)
	}
	again, err := st.Update(task.ID, Patch{Title: str("Renamed"), Done: boolp(true)})
	if err != nil {
		t.Fatal(err)
	}
	if again.Source != "GOG · w3:p1" || again.Title != "Renamed" {
		t.Errorf("after a patch: %q %q", again.Source, again.Title)
	}
	mine, err := st.Create(ps[0].ID, Patch{Title: str("Mine")})
	if err != nil {
		t.Fatal(err)
	}
	if mine.Source != "" {
		t.Errorf("a person's task has a source: %q", mine.Source)
	}
	list, _, err := st.List(Query{View: "project", Project: ps[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if item.ID == task.ID && item.Source != "GOG · w3:p1" {
			t.Errorf("the list lost the source: %q", item.Source)
		}
	}
}
