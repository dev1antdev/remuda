package tasks

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// MaxMarkdown is how much of a Markdown description is read; the rest is dropped.
const MaxMarkdown = 64 << 10

// MarkdownHTML turns a Markdown text, as an agent writes one, into the allow-listed HTML a task description holds:
// paragraphs, headings, lists one level deep with task items as checkboxes, fenced and inline code, bold, italic,
// http links, rules and quotes; everything else stays text.
func MarkdownHTML(md string) string {
	if len(md) > MaxMarkdown {
		md = md[:MaxMarkdown]
	}
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var out strings.Builder
	var para, quote []string
	list := &listWriter{out: &out}
	flushPara := func() {
		if len(para) == 0 {
			return
		}
		out.WriteString("<p>")
		for i, line := range para {
			if i > 0 {
				out.WriteString("<br>")
			}
			out.WriteString(inline(line))
		}
		out.WriteString("</p>")
		para = nil
	}
	flushQuote := func() {
		if len(quote) == 0 {
			return
		}
		out.WriteString("<blockquote><p>" + inline(strings.Join(quote, " ")) + "</p></blockquote>")
		quote = nil
	}
	flushAll := func() { flushPara(); flushQuote(); list.close() }
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(trimmed, "```"):
			flushAll()
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				code = append(code, lines[i])
			}
			out.WriteString("<pre><code>" + html.EscapeString(strings.Join(code, "\n")) + "</code></pre>")
		case trimmed == "":
			flushAll()
		case headingRe.MatchString(trimmed):
			flushAll()
			m := headingRe.FindStringSubmatch(trimmed)
			out.WriteString(fmt.Sprintf("<h%d>%s</h%d>", len(m[1]), inline(m[2]), len(m[1])))
		case ruleRe.MatchString(trimmed):
			flushAll()
			out.WriteString("<hr>")
		case strings.HasPrefix(trimmed, ">"):
			flushPara()
			list.close()
			quote = append(quote, strings.TrimSpace(strings.TrimPrefix(trimmed, ">")))
		case itemRe.MatchString(lines[i]):
			flushPara()
			flushQuote()
			list.item(lines[i])
		default:
			flushQuote()
			list.close()
			para = append(para, trimmed)
		}
	}
	flushAll()
	return out.String()
}

var (
	headingRe = regexp.MustCompile(`^(#{1,4})\s+(.*?)\s*#*$`)
	ruleRe    = regexp.MustCompile(`^(-{3,}|\*{3,}|_{3,})$`)
	itemRe    = regexp.MustCompile(`^(\s*)([-*+]|\d{1,3}[.)])\s+(.*)$`)
	linkRe    = regexp.MustCompile(`^\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	urlRe     = regexp.MustCompile(`^https?://[^\s<>"']+`)
)

// listWriter writes list items as they come, nesting one level by indentation; the innermost item stays open for a
// nested list until the next item or the list's end.
type listWriter struct {
	out  *strings.Builder
	tags []string // the lists open, outermost first
}

func (l *listWriter) item(line string) {
	m := itemRe.FindStringSubmatch(line)
	depth := 0
	if len(m[1]) >= 2 {
		depth = 1
	}
	tag := "ul"
	if m[2][0] >= '0' && m[2][0] <= '9' {
		tag = "ol"
	}
	switch {
	case len(l.tags) == 0:
		l.out.WriteString("<" + tag + ">")
		l.tags = []string{tag}
	case depth > len(l.tags)-1:
		l.out.WriteString("<" + tag + ">")
		l.tags = append(l.tags, tag)
	case depth < len(l.tags)-1:
		l.out.WriteString("</li></" + l.tags[1] + "></li>")
		l.tags = l.tags[:1]
	default:
		l.out.WriteString("</li>")
	}
	text := m[3]
	l.out.WriteString("<li>")
	switch {
	case strings.HasPrefix(text, "[ ] "):
		l.out.WriteString(`<input type="checkbox"> `)
		text = text[4:]
	case strings.HasPrefix(text, "[x] ") || strings.HasPrefix(text, "[X] "):
		l.out.WriteString(`<input type="checkbox" checked> `)
		text = text[4:]
	}
	l.out.WriteString(inline(text))
}

func (l *listWriter) close() {
	for i := len(l.tags) - 1; i >= 0; i-- {
		l.out.WriteString("</li></" + l.tags[i] + ">")
	}
	l.tags = nil
}

// inline draws a line's marks: code spans, bold, italic, links and bare addresses; everything else escaped.
func inline(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch {
		case s[i] == '`':
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				b.WriteString("<code>" + html.EscapeString(s[i+1:i+1+j]) + "</code>")
				i += j + 2
				continue
			}
		case strings.HasPrefix(s[i:], "**") || strings.HasPrefix(s[i:], "__"):
			mark := s[i : i+2]
			if j := strings.Index(s[i+2:], mark); j > 0 {
				b.WriteString("<strong>" + inline(s[i+2:i+2+j]) + "</strong>")
				i += j + 4
				continue
			}
		case s[i] == '*' || s[i] == '_':
			if j := emphasisEnd(s, i); j > 0 {
				b.WriteString("<em>" + inline(s[i+1:j]) + "</em>")
				i = j + 1
				continue
			}
		case s[i] == '[':
			if m := linkRe.FindStringSubmatch(s[i:]); m != nil {
				b.WriteString(`<a href="` + html.EscapeString(m[2]) + `">` + inline(m[1]) + "</a>")
				i += len(m[0])
				continue
			}
		case strings.HasPrefix(s[i:], "http://") || strings.HasPrefix(s[i:], "https://"):
			if m := urlRe.FindString(s[i:]); m != "" {
				url := strings.TrimRight(m, ".,;:!?)")
				b.WriteString(`<a href="` + html.EscapeString(url) + `">` + html.EscapeString(url) + "</a>")
				i += len(url)
				continue
			}
		}
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteByte(s[i])
		}
		i++
	}
	return b.String()
}

// emphasisEnd is where a *word* or _word_ closes, 0 when it does not: the mark hugs its text, and an underscore
// inside a word (snake_case) is no mark.
func emphasisEnd(s string, i int) int {
	mark := s[i]
	if i+1 >= len(s) || s[i+1] == ' ' || (mark == '_' && i > 0 && isWord(s[i-1])) {
		return 0
	}
	for j := i + 2; j < len(s); j++ {
		if s[j] != mark || s[j-1] == ' ' {
			continue
		}
		if mark == '_' && j+1 < len(s) && isWord(s[j+1]) {
			continue
		}
		return j
	}
	return 0
}

func isWord(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
