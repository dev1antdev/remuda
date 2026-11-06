package worktrees

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
)

// IncludeFile is the file in a repository's main checkout that names the ignored files every new worktree gets: the
// one Claude Code reads, gitignore patterns applied only to the files git ignores (.env, local configs).
const IncludeFile = ".worktreeinclude"

// MaxInclude is the most files one worktree gets; past it nothing is copied, since half a node_modules is worse than none.
const MaxInclude = 1000

// IncludeTimeout is how long the copy may take.
var IncludeTimeout = 2 * time.Minute

// Included is what a new worktree got from its repository's .worktreeinclude.
type Included struct {
	Listed  bool     `json:"listed"`             // the main checkout has a .worktreeinclude
	Main    string   `json:"main,omitempty"`     // the checkout the files came from
	Copied  []string `json:"copied,omitempty"`   // relative to the repository
	Skipped []Skip   `json:"skipped,omitempty"`  // matched, ignored, and still left out
	TooMany int      `json:"too_many,omitempty"` // the files the patterns matched, when past MaxInclude: none copied
}

// Skip is a file the patterns named that the worktree did not get, and why.
type Skip struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

// IncludeScript copies into the new worktree every file its repository's main checkout ignores and lists in
// .worktreeinclude: never a tracked file, never one already in the worktree, a link only when it stays inside the
// repository, nothing written where the worktree's own folders lead out of it. It prints "none" without the file, else
// "main", then a line per file: "copied" or "skipped" with the reason; "toomany" with the count past MaxInclude.
func IncludeScript(worktree string) string {
	skip := func(why string) string {
		return `printf 'skipped\037%s\037%s\n' "$f" ` + script.Quote(why) + `; continue`
	}
	return script.PATH + "\n" +
		"cd " + script.Quote(worktree) + " 2>/dev/null || { echo 'the new worktree is not there'; exit 3; }\n" +
		"wt=$(pwd -P)\n" +
		"main=$(git worktree list --porcelain 2>/dev/null | sed -n '1s/^worktree //p')\n" +
		"[ -n \"$main\" ] || { echo 'not a git worktree'; exit 3; }\n" +
		"cd \"$main\" 2>/dev/null || { echo 'the main checkout is not there'; exit 3; }\n" +
		"main=$(pwd -P)\n" +
		"if [ \"$main\" = \"$wt\" ] || [ ! -f " + IncludeFile + " ]; then echo none; exit 0; fi\n" +
		"printf 'main\\037%s\\n' \"$main\"\n" +
		"list=$(mktemp \"${TMPDIR:-/tmp}/remuda-include.XXXXXX\") || { echo 'no temporary file'; exit 4; }\n" +
		"git -c core.quotepath=off ls-files --others --ignored --exclude-from=" + IncludeFile + " 2>/dev/null | " +
		"git -c core.quotepath=off check-ignore --stdin 2>/dev/null > \"$list\"\n" +
		"n=$(wc -l < \"$list\" | tr -d ' ')\n" +
		"if [ \"$n\" -gt " + strconv.Itoa(MaxInclude) + " ]; then printf 'toomany\\037%s\\n' \"$n\"; rm -f \"$list\"; exit 0; fi\n" +
		"while IFS= read -r f; do\n" +
		"  [ -n \"$f\" ] || continue\n" +
		"  src=\"$main/$f\"; dst=\"$wt/$f\"; dir=$(dirname \"$dst\")\n" +
		"  if [ -e \"$dst\" ] || [ -L \"$dst\" ]; then " + skip("already in the worktree") + "; fi\n" +
		"  from=$(cd \"$(dirname \"$src\")\" 2>/dev/null && pwd -P) || from=\n" +
		"  case \"$from/\" in \"$main\"/*) ;; *) " + skip("its folder leads out of the repository") + ";; esac\n" +
		"  if [ -L \"$src\" ]; then\n" +
		"    target=$(readlink \"$src\")\n" +
		"    case \"$target\" in /*) to=$target ;; *) to=\"$(dirname \"$src\")/$target\" ;; esac\n" +
		"    if [ -d \"$to\" ]; then todir=$(cd \"$to\" 2>/dev/null && pwd -P) || todir=\n" + // a link to a folder, .. among them, is where it leads
		"    else todir=$(cd \"$(dirname \"$to\")\" 2>/dev/null && pwd -P) || todir=; fi\n" +
		"    case \"$todir/\" in \"$main\"/*) ;; *) " + skip("a link out of the repository") + ";; esac\n" +
		"  elif [ ! -f \"$src\" ]; then " + skip("not a file") + "; fi\n" +
		"  up=$dir; while [ ! -d \"$up\" ] && [ \"$up\" != / ]; do up=$(dirname \"$up\"); done\n" +
		"  real=$(cd \"$up\" 2>/dev/null && pwd -P) || real=\n" +
		"  case \"$real/\" in \"$wt\"/*) ;; *) " + skip("its folder in the worktree leads out of it") + ";; esac\n" +
		"  mkdir -p \"$dir\" 2>/dev/null || { " + skip("its folder could not be made") + "; }\n" +
		"  if cp -pP \"$src\" \"$dst\" 2>/dev/null; then printf 'copied\\037%s\\n' \"$f\"; else " + skip("it could not be copied") + "; fi\n" +
		"done < \"$list\"\n" +
		"rm -f \"$list\"\n"
}

// ParseIncluded reads the script's lines.
func ParseIncluded(out string) Included {
	var in Included
	for _, line := range script.Lines(out) {
		fields := script.Fields(line)
		switch {
		case fields[0] == "main" && len(fields) == 2:
			in.Listed, in.Main = true, fields[1]
		case fields[0] == "copied" && len(fields) == 2:
			in.Copied = append(in.Copied, fields[1])
		case fields[0] == "skipped" && len(fields) == 3:
			in.Skipped = append(in.Skipped, Skip{Path: fields[1], Why: fields[2]})
		case fields[0] == "toomany" && len(fields) == 2:
			in.TooMany, _ = strconv.Atoi(fields[1])
		}
	}
	return in
}

// Include gives the new worktree the files its repository's .worktreeinclude names, on the worktree's machine.
func Include(host shell.Shell, worktree string) (Included, error) {
	out, err := shell.Text(host, IncludeScript(worktree), IncludeTimeout)
	if err != nil {
		return ParseIncluded(out), errors.New(lastLine(out, err.Error()))
	}
	return ParseIncluded(out), nil
}

// Words says what the worktree got, for a task's comment; "" when the repository has no .worktreeinclude.
func (in Included) Words() string {
	switch {
	case !in.Listed:
		return ""
	case in.TooMany > 0:
		return fmt.Sprintf("The repository's .worktreeinclude matches %d ignored files, more than the %d a worktree gets: nothing was copied — narrow its patterns.", in.TooMany, MaxInclude)
	case len(in.Copied) == 0 && len(in.Skipped) == 0:
		return "The repository's .worktreeinclude matches no ignored file: nothing to copy."
	}
	var words []string
	if len(in.Copied) > 0 {
		noun := "files"
		if len(in.Copied) == 1 {
			noun = "file"
		}
		words = append(words, fmt.Sprintf("Copied %d %s the repository's .worktreeinclude names into the new worktree: %s.", len(in.Copied), noun, someOf(in.Copied, 8)))
	}
	if len(in.Skipped) > 0 {
		left := make([]string, len(in.Skipped))
		for i, s := range in.Skipped {
			left[i] = s.Path + " (" + s.Why + ")"
		}
		words = append(words, "Left out: "+someOf(left, 5)+".")
	}
	return strings.Join(words, " ")
}

// someOf names the first n of a list and counts the rest.
func someOf(list []string, n int) string {
	if len(list) <= n {
		return strings.Join(list, ", ")
	}
	return strings.Join(list[:n], ", ") + fmt.Sprintf(" and %d more", len(list)-n)
}
