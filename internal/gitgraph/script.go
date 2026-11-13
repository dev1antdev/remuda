package gitgraph

import (
	"strconv"
	"strings"

	"github.com/dev1antdev/remuda/internal/script"
)

// Read is git as every read here runs it: no lock taken on the index, no fsmonitor woken, no refreshed index written
// back, a pathspec taken as the very path, paths printed as they are, never a colour, never a pager. Defined as the
// shell function g.
const Read = `g() { git --no-pager --no-optional-locks --literal-pathspecs -c core.fsmonitor=false -c diff.autoRefreshIndex=false -c core.quotepath=off -c color.ui=never "$@"; }; `

// logFormat is seven NUL-ended fields a commit: hash, parents, author, e-mail, time, labels, subject.
const logFormat = `%H%x00%P%x00%an%x00%ae%x00%at%x00%D%x00%s`

// stateScript enters the folder and prints the header — repo, the checkout's top, HEAD, the branch, an operation
// stopped in progress, the refs' fingerprint — then the tracked changes and the count of every file git status lists.
// A folder gone or outside git prints a header of one word, which reads as no repository.
func stateScript(dir string) string {
	return script.PATH + "; " + Read +
		`cd ` + script.Quote(dir) + ` 2>/dev/null || { printf 'gone\n'; exit 0; }; ` +
		`top=$(g rev-parse --show-toplevel 2>/dev/null) || { printf 'norepo\n'; exit 0; }; ` +
		`gd=$(g rev-parse --absolute-git-dir 2>/dev/null); head=$(g rev-parse -q --verify HEAD 2>/dev/null); branch=$(g symbolic-ref -q --short HEAD 2>/dev/null); ` +
		`op=; if [ -d "$gd/rebase-merge" ] || [ -d "$gd/rebase-apply" ]; then op=rebase; elif [ -f "$gd/MERGE_HEAD" ]; then op=merge; ` +
		`elif [ -f "$gd/CHERRY_PICK_HEAD" ]; then op=cherry-pick; elif [ -f "$gd/REVERT_HEAD" ]; then op=revert; fi; ` +
		`print=$({ printf '%s %s %s\n' "$head" "$branch" "$op"; g for-each-ref --format='%(objectname) %(refname)' refs/heads refs/tags refs/remotes 2>/dev/null; } | cksum); ` +
		`printf 'repo\n%s\n%s\n%s\n%s\n%s\n' "$top" "$head" "$branch" "$op" "$print"; ` +
		`printf '\036status\n'; ` +
		// a line break inside a path becomes \001 and every NUL a line, so awk reads one entry a line
		`g status --porcelain=v1 -z --no-renames --untracked-files=all 2>/dev/null | tr '\n\000' '\001\n' | ` +
		`awk -v max=` + strconv.Itoa(MaxChanges) + ` '{ n++; x = substr($0, 1, 2); if (x ~ /U/ || x == "AA" || x == "DD") c++; if (x != "??" && kept < max) { print; kept++ } } END { printf "\036count %d %d\n", n, c }'; `
}

// logScope is the walk: every local branch, tag and (asked for) remote branch with HEAD, or HEAD alone. Never --all,
// which walks other tools' private refs too and draws them as commits nobody can name.
func logScope(q Query) string {
	if !q.All {
		return "HEAD"
	}
	scope := "--branches --tags "
	if q.Remotes {
		scope += "--remotes "
	}
	return scope + "--ignore-missing HEAD"
}

// logScript is the state, then one page of the history (one commit past it, which says whether there are more), git's
// own words when it fails, the local branches and the remotes. A repository with no commit yet walks nothing.
func logScript(dir string, q Query) string {
	return stateScript(dir) +
		`printf '\036log\n'; exec 3>&1; err=; code=0; ` +
		`if [ -n "$head" ]; then err=$(g log -z --date-order --decorate=full --skip=` + strconv.Itoa(q.Skip) + ` -n` + strconv.Itoa(q.Limit+1) +
		` --format='` + logFormat + `' ` + logScope(q) + ` -- 2>&1 >&3 3>&-); code=$?; fi; ` +
		`printf '\036logend %s\n%s\n' "$code" "$err"; ` +
		`printf '\036branches\n'; g for-each-ref --format='%(refname)' refs/heads 2>/dev/null; ` +
		`printf '\036remotes\n'; g remote 2>/dev/null; ` +
		`printf '\036end\n'`
}

// section is one part of an answer: the word after the separator, the rest of its line, the lines under it.
type section struct {
	args string
	body string
}

// sections is an answer cut at its separators, the header before the first one apart.
type sections struct {
	head  string
	parts map[string]section
}

func (s sections) get(name string) (section, bool) {
	part, ok := s.parts[name]
	return part, ok
}

// sectionsOf cuts an answer at every \036 that begins a section: \036name args\nbody.
func sectionsOf(out string) sections {
	chunks := strings.Split(out, script.RS)
	parsed := sections{head: chunks[0], parts: map[string]section{}}
	for _, chunk := range chunks[1:] {
		line, body, _ := strings.Cut(chunk, "\n")
		name, args, _ := strings.Cut(line, " ")
		if _, seen := parsed.parts[name]; !seen {
			parsed.parts[name] = section{args: args, body: body}
		}
	}
	return parsed
}
