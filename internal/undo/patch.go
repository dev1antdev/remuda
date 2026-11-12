package undo

import (
	"strconv"
	"strings"
)

// sections cuts a git patch into its files' sections by path, in order; a type change is two sections of one path,
// kept together. A patch that was cut short loses its last section, which is not whole.
func sections(patch string, cut bool) map[string]string {
	out := map[string]string{}
	starts := sectionStarts(patch)
	if cut && len(starts) > 0 {
		patch = patch[:starts[len(starts)-1]]
		starts = starts[:len(starts)-1]
	}
	for i, start := range starts {
		end := len(patch)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		text := patch[start:end]
		if path, ok := headerPath(text); ok {
			out[path] += text
		}
	}
	return out
}

// sectionStarts is where every "diff --git " header line begins: no hunk or binary line can start so, a hunk's lines
// start with a space, a plus, a minus or a backslash, and base85 holds no space.
func sectionStarts(patch string) []int {
	var starts []int
	const header = "diff --git "
	if strings.HasPrefix(patch, header) {
		starts = append(starts, 0)
	}
	for at := 0; ; {
		i := strings.Index(patch[at:], "\n"+header)
		if i < 0 {
			return starts
		}
		at += i + 1
		starts = append(starts, at)
	}
}

// headerPath is the path a section's header names, "diff --git a/<path> b/<path>" (the two prefixes swapped in a
// reverse patch), each side quoted the C way when the path holds a byte git quotes.
func headerPath(section string) (string, bool) {
	line, _, _ := strings.Cut(section, "\n")
	rest, ok := strings.CutPrefix(line, "diff --git ")
	if !ok {
		return "", false
	}
	if strings.HasPrefix(rest, `"`) {
		end := quotedEnd(rest)
		if end < 0 {
			return "", false
		}
		first, err := strconv.Unquote(rest[:end+1])
		if err != nil || len(first) < 2 {
			return "", false
		}
		return first[2:], true
	}
	// unquoted: "x/P y/P" with the same P on both sides, so the path is the first half less its prefix
	if len(rest) < 5 || (len(rest)-5)%2 != 0 {
		return "", false
	}
	n := (len(rest) - 5) / 2
	path := rest[2 : 2+n]
	if rest[2+n:] != " "+rest[3+n:5+n]+path || rest[4+n] != '/' {
		return "", false
	}
	return path, true
}

// quotedEnd is the index of the quote that closes the C-quoted string s opens, -1 for none.
func quotedEnd(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// binary says whether a section carries a binary patch.
func binary(section string) bool {
	return strings.Contains(section, "\nGIT binary patch\n") || strings.Contains(section, "\nBinary files ")
}
