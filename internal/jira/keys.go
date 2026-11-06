package jira

import (
	"regexp"
	"strings"
)

// MaxKeys is how many keys one workspace shows.
const MaxKeys = 3

var (
	keyRe      = regexp.MustCompile(`\b([A-Z][A-Z0-9_]+)-([1-9][0-9]{0,6})\b`)
	wholeKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-[1-9][0-9]{0,6}$`)
	browseRe   = regexp.MustCompile(`(?i)(?:/browse/|[?&]selectedIssue=)([A-Z][A-Z0-9_]+-[1-9][0-9]{0,6})\b`)
)

// KeysIn is every issue key the texts name, the first text's first, each once: the agents write keys in capitals,
// so a lower-case word like utf-8 is none.
func KeysIn(texts []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, text := range texts {
		for _, key := range keyRe.FindAllString(text, -1) {
			if !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	return out
}

// BranchKeys is the keys a branch is named after, in any case: feature/crmops-6279-conflicts names CRMOPS-6279.
func BranchKeys(branch string) []string {
	return KeysIn([]string{strings.ToUpper(branch)})
}

// Project is a key's project: CRMOPS for CRMOPS-6279.
func Project(key string) string {
	project, _, _ := strings.Cut(key, "-")
	return project
}

// KeyOf reads what the person typed — a key, or an issue's address — as a key; false for anything else.
func KeyOf(typed string) (string, bool) {
	typed = strings.TrimSpace(typed)
	if m := browseRe.FindStringSubmatch(typed); m != nil {
		return strings.ToUpper(m[1]), true
	}
	key := strings.ToUpper(typed)
	return key, wholeKeyRe.MatchString(key)
}

// Merge is the keys of a workspace in the order the person reads them — its branch first, then its agents' — each
// once, at most MaxKeys.
func Merge(lists ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, list := range lists {
		for _, key := range list {
			if len(out) == MaxKeys {
				return out
			}
			if !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	return out
}
