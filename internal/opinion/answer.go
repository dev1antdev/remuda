// Package opinion is the second opinion on a folder's changes: another agent asked, in a tab of its own, to review
// what the Changes lens shows without touching it, and its answer read back into comments on lines of the diff —
// kept per machine and folder until the person adds them to their own review, dismisses them, or asks again.
package opinion

import (
	"regexp"
	"strconv"
	"strings"
)

// Finding is one comment of the reviewer's: a line of a file, how much it matters, what is wrong.
type Finding struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Severity string `json:"severity,omitempty"` // high, medium or low; "" when the reviewer gave none
	Text     string `json:"text"`
}

// Answer is a reviewer's answer read: its findings, its summary, and whether it said it found nothing.
type Answer struct {
	Findings []Finding
	Summary  string
	None     bool
}

// Readable says whether the answer is one the format asked for: findings, or an explicit none.
func (a Answer) Readable() bool { return len(a.Findings) > 0 || a.None }

// MaxFindings is how many findings one answer keeps; MaxText how long a finding's text may be.
const (
	MaxFindings = 100
	MaxText     = 1000
	maxSummary  = 600
)

var (
	// a heading's word alone, or followed by a colon or a space and its text: never a file named summary.go
	sectionRe  = regexp.MustCompile(`(?i)^(?:#{1,6}\s*)?(?:\*\*|__)?\s*(findings|summary)(?:\s*(?:\*\*|__)?\s*(?::|\s|$)\s*(?:\*\*|__)?\s*(.*))?$`)
	leadRe     = regexp.MustCompile(`^(?:[-*+•]\s+|\d+[.)]\s+)`)
	linkRe     = regexp.MustCompile(`\[([^\]]+)\]\([^)\s]*\)`) // [src/a.go:12](src/a.go#L12), as Codex names a file
	starBoldRe = regexp.MustCompile(`(^|[\s(])\*\*([^*\s](?:[^*]*[^*\s])?)\*\*`)
	// __bold__ is a pair around more than a name: never a Python dunder (__len__) or a __tests__ folder inside a path
	underBoldRe  = regexp.MustCompile(`(^|[\s(])__([^_\s](?:[^_]*[^_\s])?)__($|[\s:,;!?)—–-])`)
	findingRe    = regexp.MustCompile(`^([^\s:*` + "`" + `"'<>]+?):(\d+)(?:\s*[-–]\s*\d+)?(?::\d+)?(?:\s*[:\-—–]\s*|\s+)(.+)$`) // [slug] and (group) folders are paths too
	fileRe       = regexp.MustCompile(`^([^\s:*` + "`" + `"'<>]+?)\s*:\s+(.+)$`)                                                // a finding on a file as a whole, under FINDINGS
	fileNameRe   = regexp.MustCompile(`(?:/|\.[A-Za-z0-9]+$)`)
	addressRe    = regexp.MustCompile(`^[\d.]+$`)                                                                                      // 127.0.0.1:8080 is an address, not a file and a line
	severityRe   = regexp.MustCompile(`(?i)^(?:\[\s*([a-z]+)\s*\]|\(\s*([a-z]+)\s*\)|\*\*([a-z]+)\*\*|([a-z]+)\s*(?::|—|–|\s-\s))\s*`) // low-level is a word, not low
	noneRe       = regexp.MustCompile(`(?i)^(?:none|no findings?|nothing (?:to report|found)|no issues? found)\.?$`)
	identifierRe = regexp.MustCompile(`^\w+$`)
)

// severities maps the words reviewers use to the three the page shows.
var severities = map[string]string{
	"high": "high", "critical": "high", "blocker": "high", "major": "high", "severe": "high",
	"medium": "medium", "moderate": "medium", "med": "medium",
	"low": "low", "minor": "low", "nit": "low", "trivial": "low", "info": "low",
}

// Parse reads a reviewer's answer: `path:line: [severity] text` lines under FINDINGS (or anywhere, when the answer has
// no sections), NONE for nothing found, the text under SUMMARY — bullets, numbers, bold, backticks, Markdown links and
// code fences around them allowed, an indented line carrying on the finding above it, and under FINDINGS a `path:
// text` (or a line 0) a finding on the file as a whole. A path that starts with the folder is made relative to it.
func Parse(text, dir string) Answer {
	var answer Answer
	var summary []string
	var loose []string
	section := ""
	seen := map[string]bool{}
	last := -1 // the finding an indented line carries on
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") || line == "" {
			last = -1
			continue
		}
		if m := sectionRe.FindStringSubmatch(line); m != nil {
			section, last = strings.ToLower(m[1]), -1
			if rest := strings.TrimSpace(m[2]); rest != "" {
				line = rest
			} else {
				continue
			}
		}
		if section == "summary" {
			summary = append(summary, clean(line))
			continue
		}
		bare := clean(line)
		if noneRe.MatchString(bare) {
			answer.None, last = true, -1
			continue
		}
		finding, ok := findingOf(bare, dir)
		if !ok && section == "findings" {
			finding, ok = fileFindingOf(bare, dir)
		}
		if ok {
			last = -1
			id := finding.Path + ":" + strconv.Itoa(finding.Line) + ":" + finding.Text
			if !seen[id] && len(answer.Findings) < MaxFindings {
				seen[id] = true
				answer.Findings = append(answer.Findings, finding)
				last = len(answer.Findings) - 1
			}
			continue
		}
		if last >= 0 && raw != strings.TrimLeft(raw, " \t") {
			answer.Findings[last].Text = cut(answer.Findings[last].Text+" "+bare, MaxText)
			continue
		}
		last = -1
		loose = append(loose, bare)
	}
	if len(summary) == 0 && len(loose) > 0 {
		summary = loose[len(loose)-1:]
	}
	answer.Summary = cut(strings.Join(summary, " "), maxSummary)
	return answer
}

// clean is a line without its list mark, links, bold and backticks
func clean(line string) string {
	line = leadRe.ReplaceAllString(line, "")
	line = linkRe.ReplaceAllString(line, "$1")
	line = strings.ReplaceAll(line, "`", "")
	line = starBoldRe.ReplaceAllString(line, "$1$2")
	line = underBoldRe.ReplaceAllStringFunc(line, func(m string) string {
		s := underBoldRe.FindStringSubmatch(m)
		if identifierRe.MatchString(s[2]) {
			return m
		}
		return s[1] + s[2] + s[3]
	})
	return strings.TrimSpace(line)
}

// findingOf reads one finding line, a URL or a time of day ("at 12:30 the job ran") being none
func findingOf(line, dir string) (Finding, bool) {
	m := findingRe.FindStringSubmatch(line)
	if m == nil || strings.Contains(m[1], "//") || addressRe.MatchString(m[1]) {
		return Finding{}, false
	}
	path := pathOf(m[1], dir)
	if !strings.ContainsAny(path, "./") && !isBareFileName(path) {
		return Finding{}, false
	}
	number, err := strconv.Atoi(m[2])
	if err != nil || number < 0 {
		return Finding{}, false
	}
	return withSeverity(path, number, m[3])
}

// fileFindingOf reads `path: text`, a finding on the file as a whole (line 0): a name a file has, with its folder or
// its extension, never a word followed by a colon
func fileFindingOf(line, dir string) (Finding, bool) {
	m := fileRe.FindStringSubmatch(line)
	if m == nil || strings.Contains(m[1], "//") || addressRe.MatchString(m[1]) || !fileNameRe.MatchString(m[1]) {
		return Finding{}, false
	}
	return withSeverity(pathOf(m[1], dir), 0, m[2])
}

// pathOf is a path the reviewer named, made relative to the folder when it starts with it
func pathOf(path, dir string) string {
	path = strings.TrimPrefix(path, "./")
	if dir != "" {
		path = strings.TrimPrefix(path, strings.TrimRight(dir, "/")+"/")
	}
	return path
}

// withSeverity is the finding with its text read: a severity word in front taken off and mapped
func withSeverity(path string, number int, text string) (Finding, bool) {
	rest := strings.TrimSpace(text)
	severity := ""
	if s := severityRe.FindStringSubmatch(rest); s != nil {
		word := strings.ToLower(s[1] + s[2] + s[3] + s[4])
		if mapped, ok := severities[word]; ok {
			severity = mapped
			rest = strings.TrimLeft(rest[len(s[0]):], ":-—– ")
		}
	}
	if rest == "" {
		return Finding{}, false
	}
	return Finding{Path: path, Line: number, Severity: severity, Text: cut(rest, MaxText)}, true
}

// isBareFileName is a file without an extension a repository keeps at its root: Makefile, Dockerfile, LICENSE
func isBareFileName(path string) bool {
	switch strings.ToLower(path) {
	case "makefile", "dockerfile", "gemfile", "rakefile", "procfile", "justfile", "license", "jenkinsfile", "vagrantfile":
		return true
	}
	return false
}

// cut keeps at most max runes, with an ellipsis when it cut
func cut(text string, max int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= max {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:max-1])) + "…"
}
