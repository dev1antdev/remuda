package repos

import (
	"regexp"
	"strings"
)

var (
	scriptsKey = regexp.MustCompile(`^scripts:\s*(#.*)?$`)
	blockMark  = regexp.MustCompile(`^[|>][+-]?\s*(#.*)?$`)
)

// YAMLScript is a script under a yaml file's top-level scripts: key — a | or > block, or a one-line value — the way
// herdr.yaml and Orca's orca.yaml keep a worktree's setup; "" when the file has none. A reader of that one shape, not of YAML.
func YAMLScript(text, name string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	start := -1
	for i, line := range lines {
		if scriptsKey.MatchString(line) {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	entry := regexp.MustCompile(`^(\s+)` + regexp.QuoteMeta(name) + `:\s*(.*)$`)
	for i := start + 1; i < len(lines); i++ {
		if lines[i] != "" && lines[i][0] != ' ' && lines[i][0] != '\t' && !strings.HasPrefix(lines[i], "#") {
			return "" // the next top-level key
		}
		m := entry.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		indent, rest := len(m[1]), strings.TrimSpace(m[2])
		if rest != "" && !blockMark.MatchString(rest) {
			return unquote(rest)
		}
		var body []string
		inner := -1
		for _, line := range lines[i+1:] {
			if strings.TrimSpace(line) == "" {
				body = append(body, "")
				continue
			}
			at := len(line) - len(strings.TrimLeft(line, " \t"))
			if at <= indent {
				break
			}
			if inner < 0 {
				inner = at
			}
			body = append(body, line[min(inner, at):])
		}
		return strings.TrimSpace(strings.Join(body, "\n"))
	}
	return ""
}

// unquote takes the quotes off a one-line yaml value.
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
