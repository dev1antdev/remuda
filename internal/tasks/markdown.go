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
	md = cutBytes(md, MaxMarkdown)
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
	links := linkScan{s: s, closeAt: -1, stopAt: -1}
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
			if text, url, end, ok := links.at(i); ok {
				b.WriteString(`<a href="` + html.EscapeString(url) + `">` + inline(text) + "</a>")
				i = end
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

// linkScan reads [text](http…) links off one line in time linear in its length: the closing bracket and the end of
// an address are each looked for once per stretch of the line, where a regexp tried at every '[' cost the line's
// square (65 536 brackets took 46 s).
type linkScan struct {
	s                  string
	closeFrom, closeAt int // closeAt is the first ']' at or after closeFrom, len(s) when there is none
	stopFrom, stopAt   int // stopAt is the first ')' or space at or after stopFrom, len(s) when there is none
}

// at is the link that opens at s[i], '[': its text, its address and where it ends; false when none does.
func (l *linkScan) at(i int) (text, url string, end int, ok bool) {
	closing := firstFrom(l.s, i+1, &l.closeFrom, &l.closeAt, func(c byte) bool { return c == ']' })
	if closing == i+1 || closing+1 >= len(l.s) || l.s[closing+1] != '(' {
		return "", "", 0, false
	}
	start := closing + 2
	scheme := 0
	switch rest := l.s[start:]; {
	case strings.HasPrefix(rest, "http://"):
		scheme = len("http://")
	case strings.HasPrefix(rest, "https://"):
		scheme = len("https://")
	default:
		return "", "", 0, false
	}
	stop := firstFrom(l.s, start, &l.stopFrom, &l.stopAt, func(c byte) bool {
		return c == ')' || c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
	})
	if stop >= len(l.s) || l.s[stop] != ')' || stop == start+scheme {
		return "", "", 0, false
	}
	return l.s[i+1 : closing], l.s[start:stop], stop + 1, true
}

// firstFrom is the first byte at or after p that matches, len(s) for none; the last answer is kept in from and at
// and given again for any p between them, since no match lies there.
func firstFrom(s string, p int, from, at *int, match func(byte) bool) int {
	if *from <= p && p <= *at {
		return *at
	}
	*from, *at = p, len(s)
	for j := p; j < len(s); j++ {
		if match(s[j]) {
			*at = j
			break
		}
	}
	return *at
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
