package jira

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// descriptionMarkdown is an issue's description as Markdown: Jira Cloud sends its document format (a JSON tree), a
// server sends wiki markup (a string); anything else is none.
func descriptionMarkdown(raw json.RawMessage) string {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return ""
	}
	if strings.HasPrefix(text, `"`) {
		var wiki string
		if json.Unmarshal(raw, &wiki) != nil {
			return ""
		}
		return WikiMarkdown(wiki)
	}
	var doc adfNode
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	return strings.TrimSpace(ADFMarkdown(doc))
}

// adfNode is one node of Atlassian's document format.
type adfNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Marks   []adfMark      `json:"marks"`
	Content []adfNode      `json:"content"`
}

type adfMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

// ADFMarkdown writes a document-format tree as Markdown the task's renderer reads: paragraphs, headings, lists, code,
// quotes, rules, links and marks; a table becomes a line a row; anything unknown keeps its text.
func ADFMarkdown(n adfNode) string {
	var b strings.Builder
	blocks(&b, n.Content, "")
	return b.String()
}

func blocks(b *strings.Builder, nodes []adfNode, indent string) {
	for _, n := range nodes {
		block(b, n, indent)
	}
}

func block(b *strings.Builder, n adfNode, indent string) {
	switch n.Type {
	case "paragraph":
		if line := inline(n.Content); strings.TrimSpace(line) != "" {
			b.WriteString(indent + line + "\n\n")
		}
	case "heading":
		level := 1
		if l, ok := n.Attrs["level"].(float64); ok {
			level = int(l)
		}
		level = min(max(level, 1), 4)
		b.WriteString(strings.Repeat("#", level) + " " + inline(n.Content) + "\n\n")
	case "bulletList", "orderedList":
		for i, item := range n.Content {
			mark := "- "
			if n.Type == "orderedList" {
				mark = itoa(i+1) + ". "
			}
			b.WriteString(indent + mark + strings.TrimSpace(itemText(item)) + "\n")
		}
		b.WriteString("\n")
	case "taskList":
		for _, item := range n.Content {
			box := "[ ] "
			if state, _ := item.Attrs["state"].(string); state == "DONE" {
				box = "[x] "
			}
			b.WriteString(indent + "- " + box + strings.TrimSpace(inline(item.Content)) + "\n")
		}
		b.WriteString("\n")
	case "codeBlock":
		lang, _ := n.Attrs["language"].(string)
		b.WriteString("```" + lang + "\n" + plainText(n.Content) + "\n```\n\n")
	case "blockquote":
		var inner strings.Builder
		blocks(&inner, n.Content, "")
		for _, line := range strings.Split(strings.TrimSpace(inner.String()), "\n") {
			b.WriteString("> " + line + "\n")
		}
		b.WriteString("\n")
	case "rule":
		b.WriteString("---\n\n")
	case "table":
		for _, row := range n.Content {
			var cells []string
			for _, cell := range row.Content {
				var inner strings.Builder
				blocks(&inner, cell.Content, "")
				cells = append(cells, strings.Join(strings.Fields(inner.String()), " "))
			}
			b.WriteString(strings.Join(cells, " · ") + "\n\n")
		}
	case "mediaSingle", "mediaGroup", "media":
		b.WriteString("[attachment]\n\n")
	case "text", "hardBreak", "mention", "emoji", "inlineCard", "status", "date":
		b.WriteString(inline([]adfNode{n}) + "\n\n")
	default:
		blocks(b, n.Content, indent)
	}
}

// itemText is a list item's paragraphs on one line, a list inside it below, indented.
func itemText(item adfNode) string {
	var parts []string
	var nested strings.Builder
	for _, c := range item.Content {
		switch c.Type {
		case "paragraph":
			parts = append(parts, inline(c.Content))
		case "bulletList", "orderedList", "taskList":
			block(&nested, c, "  ")
		default:
			var inner strings.Builder
			block(&inner, c, "")
			parts = append(parts, strings.TrimSpace(inner.String()))
		}
	}
	text := strings.Join(parts, " ")
	if nested.Len() > 0 {
		text += "\n" + strings.TrimRight(nested.String(), "\n")
	}
	return text
}

// inline is a paragraph's nodes as one Markdown line.
func inline(nodes []adfNode) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case "text":
			b.WriteString(marked(n.Text, n.Marks))
		case "hardBreak":
			b.WriteString("\n")
		case "mention":
			b.WriteString(attr(n, "text"))
		case "emoji":
			if text := attr(n, "text"); text != "" {
				b.WriteString(text)
			} else {
				b.WriteString(attr(n, "shortName"))
			}
		case "inlineCard", "blockCard":
			b.WriteString(attr(n, "url"))
		case "status":
			b.WriteString("[" + attr(n, "text") + "]")
		case "date":
			b.WriteString(attr(n, "timestamp"))
		default:
			b.WriteString(inline(n.Content))
		}
	}
	return b.String()
}

func marked(text string, marks []adfMark) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	for _, m := range marks {
		switch m.Type {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "strike":
			text = "~~" + text + "~~"
		case "link":
			if href, _ := m.Attrs["href"].(string); href != "" {
				text = "[" + text + "](" + href + ")"
			}
		}
	}
	return text
}

func plainText(nodes []adfNode) string {
	var b strings.Builder
	for _, n := range nodes {
		if n.Type == "hardBreak" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(n.Text)
		b.WriteString(plainText(n.Content))
	}
	return b.String()
}

func attr(n adfNode, name string) string {
	switch v := n.Attrs[name].(type) {
	case string:
		return v
	case float64:
		return itoa(int(v))
	}
	return ""
}

func itoa(n int) string { return strconv.Itoa(n) }

var (
	wikiHeading = regexp.MustCompile(`^h([1-6])\.\s+(.*)$`)
	wikiBullet  = regexp.MustCompile(`^([*-]+)\s+(.*)$`)
	wikiNumber  = regexp.MustCompile(`^(#+)\s+(.*)$`)
	wikiQuote   = regexp.MustCompile(`^bq\.\s+(.*)$`)
	wikiCode    = regexp.MustCompile(`^\{(code|noformat)(?::([^}|]*))?[^}]*\}(.*)$`)
	wikiLink    = regexp.MustCompile(`\[([^|\]]+)\|([^\]]+)\]`)
	wikiBare    = regexp.MustCompile(`\[((?:https?|mailto):[^\]|]+)\]`)
	wikiMono    = regexp.MustCompile(`\{\{(.+?)\}\}`)
	wikiBold    = regexp.MustCompile(`(^|[\s(])\*(\S(?:[^*]*\S)?)\*`)
	wikiItalic  = regexp.MustCompile(`(^|[\s(])_(\S(?:[^_]*\S)?)_`)
	wikiColor   = regexp.MustCompile(`\{color(?::[^}]*)?\}`)
)

// WikiMarkdown writes a server's wiki markup as Markdown: headings, lists, code and noformat blocks, quotes, links,
// monospace, bold and italic; a colour is dropped and keeps its text.
func WikiMarkdown(wiki string) string {
	var out []string
	lines := strings.Split(strings.ReplaceAll(wiki, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		if m := wikiCode.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			closing := "{" + m[1] + "}"
			var code []string
			if rest := strings.TrimSpace(m[3]); rest != "" {
				if strings.HasSuffix(rest, closing) {
					out = append(out, "```"+strings.TrimSpace(m[2]), strings.TrimSuffix(rest, closing), "```", "")
					continue
				}
				code = append(code, rest)
			}
			for i++; i < len(lines) && !strings.Contains(lines[i], closing); i++ {
				code = append(code, lines[i])
			}
			if i < len(lines) {
				if before, _, _ := strings.Cut(lines[i], closing); strings.TrimSpace(before) != "" {
					code = append(code, before)
				}
			}
			out = append(out, "```"+strings.TrimSpace(m[2]))
			out = append(out, code...)
			out = append(out, "```", "")
			continue
		}
		if strings.TrimSpace(line) == "{quote}" {
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "{quote}"; i++ {
				out = append(out, "> "+wikiInline(lines[i]))
			}
			out = append(out, "")
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "----":
			out = append(out, "---", "")
		case wikiHeading.MatchString(trimmed):
			m := wikiHeading.FindStringSubmatch(trimmed)
			level := min(int(m[1][0]-'0'), 4)
			out = append(out, strings.Repeat("#", level)+" "+wikiInline(m[2]), "")
		case wikiBullet.MatchString(trimmed) && !strings.HasPrefix(trimmed, "----"):
			m := wikiBullet.FindStringSubmatch(trimmed)
			out = append(out, strings.Repeat("  ", len(m[1])-1)+"- "+wikiInline(m[2]))
		case wikiNumber.MatchString(trimmed):
			m := wikiNumber.FindStringSubmatch(trimmed)
			out = append(out, strings.Repeat("  ", len(m[1])-1)+"1. "+wikiInline(m[2]))
		case wikiQuote.MatchString(trimmed):
			out = append(out, "> "+wikiInline(wikiQuote.FindStringSubmatch(trimmed)[1]))
		default:
			out = append(out, wikiInline(line))
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func wikiInline(s string) string {
	s = wikiColor.ReplaceAllString(s, "")
	s = wikiLink.ReplaceAllString(s, "[$1]($2)")
	s = wikiBare.ReplaceAllString(s, "$1")
	s = wikiMono.ReplaceAllString(s, "`$1`")
	s = wikiBold.ReplaceAllString(s, "$1**$2**")
	s = wikiItalic.ReplaceAllString(s, "$1*$2*")
	return s
}
