package opinion

import (
	"fmt"
	"strings"
)

// Scope is what the reviewer is asked to look at: what the Changes lens shows of the folder.
type Scope struct {
	Branch  string
	Base    string   // the commit the person last reviewed from, "" when never: only the uncommitted work then
	Commits int      // commits since Base
	Files   []string // the uncommitted files, "path (+3 −1)" or "path (new)"
}

// maxListed is how many files the prompt names; the rest are counted
const maxListed = 40

// PromptStart is how every request begins, by which a conversation is known for the review asked.
const PromptStart = "You are asked for a second opinion on the work in this folder"

// refLine names the request in its prompt.
func refLine(ref string) string { return "(second opinion request " + ref + ")" }

// Prompt is the reviewer's first prompt: the scope, the hands-off rules, the one answer form Parse reads, and the
// request's ref, by which its conversation is known.
func Prompt(scope Scope, ref string) string {
	var b strings.Builder
	b.WriteString(PromptStart)
	if scope.Branch != "" {
		fmt.Fprintf(&b, " (branch %s)", scope.Branch)
	}
	b.WriteString(". Review it; do not change it.\n\nWhat to review:\n")
	b.WriteString("- the uncommitted changes against HEAD: `git diff HEAD` and the untracked files (`git ls-files --others --exclude-standard`)\n")
	if scope.Base != "" && scope.Commits > 0 {
		fmt.Fprintf(&b, "- the %s since the last review: `git log -p %s..HEAD`\n", plural(scope.Commits, "commit"), scope.Base)
	}
	if len(scope.Files) > 0 {
		b.WriteString("\nUncommitted files:\n")
		for i, file := range scope.Files {
			if i == maxListed {
				fmt.Fprintf(&b, "- … and %d more\n", len(scope.Files)-maxListed)
				break
			}
			b.WriteString("- " + file + "\n")
		}
	}
	b.WriteString(`
Rules:
- Read only. Do not edit, create, delete, stage or commit any file, and run nothing that changes the working tree, the index or the repository: no formatter, no git add, stash, checkout or reset, no install.
- Look for bugs, regressions, security problems, missing or wrong tests, and anything the change gets wrong or leaves out. Skip matters of taste.

Answer in exactly this form and nothing else:
FINDINGS
path/to/file:LINE: [high|medium|low] what is wrong, and what to do about it
SUMMARY
one or two sentences on the change as a whole

One finding per line. The path is relative to this folder, LINE the line in the file as it is now. When you find nothing, write NONE on the line after FINDINGS.
`)
	if ref != "" {
		b.WriteString("\n" + refLine(ref) + "\n")
	}
	return b.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
