package undo

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
)

// ApplyTimeout bounds an undo's script: the patches, and a revert per commit.
var ApplyTimeout = 3 * time.Minute

// Result is what an undo did.
type Result struct {
	Files    int      `json:"files"`              // files put back
	Reverted []string `json:"reverted,omitempty"` // the revert commits made, oldest first
	Pushed   []string `json:"pushed,omitempty"`   // the remote-tracking branches that held the reverted commits
}

// Undo puts the turn's files back as they were when it began and, with revert, reverts its commits — git revert, a
// new commit each, newest first: never a reset, never a force. The checks of Plan run again first; anything that
// moved since refuses. The agent is told nothing.
func (s *Service) Undo(machineID, id string, revert bool) (Result, error) {
	turn, ok := s.Store.Get(id)
	if !ok || turn.Machine != machineID {
		return Result{}, ErrNotFound
	}
	folder := turn.Machine + "|" + turn.Dir
	s.mu.Lock()
	if s.undoing == nil {
		s.undoing = map[string]bool{}
	}
	if s.undoing[folder] {
		s.mu.Unlock()
		return Result{}, &Refusal{Why: "an undo is already running in this folder"}
	}
	s.undoing[folder] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.undoing, folder)
		s.mu.Unlock()
	}()
	plan, err := s.Plan(machineID, id)
	if err != nil {
		return Result{}, err
	}
	if plan.Refused != nil {
		return Result{}, plan.Refused
	}
	revert = revert && len(turn.Commits) > 0
	if revert && plan.RevertWhy != "" {
		return Result{}, &Refusal{Why: plan.RevertWhy}
	}
	startPatch, endPatch, err := s.Store.Patches(id)
	if err != nil {
		return Result{}, fmt.Errorf("the turn's changes are gone from the cockpit: %w", err)
	}
	steps := stepsOf(turn, revert, sections(startPatch, false), sections(endPatch, false))
	if steps.empty() {
		return Result{}, &Refusal{Why: "nothing is left to undo without reverting the turn's commits"}
	}
	host, err := s.Host(machineID)
	if err != nil {
		return Result{}, err
	}
	out, err := runSteps(host, turn, steps)
	result := Result{Files: steps.files}
	if revert {
		result.Pushed = plan.Pushed
	}
	for _, line := range script.Lines(out) {
		if made, ok := strings.CutPrefix(line, "reverted "); ok {
			result.Reverted = append(result.Reverted, made)
		}
	}
	if err != nil {
		return result, failure(out, err, len(result.Reverted), len(steps.reverts))
	}
	if !strings.Contains(out, "\nundone\n") && !strings.HasPrefix(out, "undone\n") {
		return result, errors.New("the undo stopped before its end: " + lastLine(out))
	}
	_ = s.Store.MarkUndone(id, Undone{At: time.Now(), Files: steps.files, Reverted: result.Reverted})
	return result, nil
}

// steps is an undo laid out: the end's sections taken out first, the index put back to HEAD for those paths, the
// reverts newest first, then the start's sections put back.
type steps struct {
	end, start string
	index      []byte // git update-index --index-info -z: HEAD's entry for every path the undo touches
	reverts    []Commit
	files      int
}

func (st steps) empty() bool { return st.end == "" && st.start == "" && len(st.reverts) == 0 }

// stepsOf picks what the undo does. With the reverts every file goes back to what the turn began with; without them
// only the uncommitted part goes: a file the commits changed is taken back to the commit when it was clean when the
// turn began, and left as it is when it was not (only a revert can tell the turn's part of it from what was there).
func stepsOf(turn Turn, revert bool, startSections, endSections map[string]string) steps {
	var st steps
	var end, start strings.Builder
	var index bytes.Buffer
	null := strings.Repeat("0", len(turn.HeadEnd))
	for _, file := range turn.Files {
		takeEnd := file.DirtyNow && (revert || !file.Committed || !file.DirtyThen)
		putStart := file.DirtyThen && (revert || !file.Committed)
		if !takeEnd && !putStart && !(revert && file.Committed) {
			continue
		}
		if takeEnd {
			end.WriteString(endSections[file.Path])
		}
		if putStart {
			start.WriteString(startSections[file.Path])
		}
		if file.Head.Absent() {
			fmt.Fprintf(&index, "0 %s 0\t%s\x00", null, file.Path)
		} else {
			fmt.Fprintf(&index, "%s %s 0\t%s\x00", file.Head.Mode, file.Head.Blob, file.Path)
		}
		if file.Was != WasCommitted || revert {
			st.files++
		}
	}
	st.end, st.start, st.index = end.String(), start.String(), index.Bytes()
	if revert {
		st.reverts = turn.Commits
	}
	return st
}

// runSteps puts the steps on the machine as one file and runs them at the repository's root.
func runSteps(host Host, turn Turn, st steps) (string, error) {
	tmp, err := host.Run(`mktemp "${TMPDIR:-/tmp}/remuda-undo-in.XXXXXX"`, 30*time.Second)
	if err != nil {
		return "", fmt.Errorf("no room on %s for the undo: %w", host.Label(), err)
	}
	inbox := strings.TrimSpace(string(tmp))
	data := append(append([]byte(st.end), st.start...), st.index...)
	if err := host.PutFile(inbox, data, 2*time.Minute); err != nil {
		_, _ = host.Run("rm -f "+script.Quote(inbox), 30*time.Second)
		return "", fmt.Errorf("the undo could not be copied to %s: %w", host.Label(), err)
	}
	out, err := host.Run(applyScript(turn.Root, inbox, len(st.end), len(st.start), st.reverts), ApplyTimeout)
	return string(out), err
}

// applyScript is the undo on the machine. The patches are checked whole before anything changes; a revert git stops
// on is aborted and, when no revert went through yet, the files are put back as the turn left them.
func applyScript(root, inbox string, endBytes, startBytes int, reverts []Commit) string {
	var b strings.Builder
	b.WriteString(script.PATH + "\n")
	b.WriteString("f=" + script.Quote(inbox) + "\n")
	b.WriteString(`d=$(mktemp -d "${TMPDIR:-/tmp}/remuda-undo.XXXXXX") || { rm -f "$f"; echo 'no temporary folder on the machine'; exit 4; }
trap 'rm -rf "$d"; rm -f "$f"' EXIT
`)
	b.WriteString("cd " + script.Quote(root) + " 2>/dev/null || { echo 'the folder is not there'; exit 3; }\n")
	b.WriteString("head -c " + strconv.Itoa(endBytes) + ` "$f" > "$d/end"` + "\n")
	b.WriteString("tail -c +" + strconv.Itoa(endBytes+1) + ` "$f" | head -c ` + strconv.Itoa(startBytes) + ` > "$d/start"` + "\n")
	b.WriteString("tail -c +" + strconv.Itoa(endBytes+startBytes+1) + ` "$f" > "$d/index"` + "\n")
	b.WriteString(`ap() { git -c apply.whitespace=nowarn -c apply.ignoreWhitespace=no apply --whitespace=nowarn "$@"; }
unstage() { [ -s "$d/index" ] || return 0; git update-index -z --index-info < "$d/index" || { echo 'git did not unstage the files put back'; exit 10; }; }
`)
	if len(reverts) == 0 {
		b.WriteString(`cat "$d/end" "$d/start" > "$d/all"
if [ -s "$d/all" ]; then
  ap --check "$d/all" 2>"$d/err" || { echo 'git would not take the changes back:'; cat "$d/err"; exit 5; }
  ap "$d/all" 2>"$d/err" || { echo 'git stopped taking the changes back:'; cat "$d/err"; exit 6; }
fi
unstage
echo undone
`)
		return b.String()
	}
	b.WriteString(`if [ -s "$d/end" ]; then
  ap --check "$d/end" 2>"$d/err" || { echo 'git would not take the changes back:'; cat "$d/err"; exit 5; }
  ap "$d/end" 2>"$d/err" || { echo 'git stopped taking the changes back:'; cat "$d/err"; exit 6; }
fi
unstage
if ! git diff-index --cached --quiet HEAD --; then
  [ -s "$d/end" ] && ap -R "$d/end"
  echo 'changes are staged in the folder: commit or unstage them first'; exit 7
fi
n=0
`)
	for _, commit := range reverts {
		parent := ""
		if commit.Merge {
			parent = "-m 1 "
		}
		b.WriteString(`if git revert --no-edit ` + parent + script.Quote(commit.SHA) + ` >"$d/err" 2>&1; then n=$((n+1)); printf 'reverted %s\n' "$(git rev-parse HEAD)"; else
  git revert --abort >/dev/null 2>&1
  [ "$n" = 0 ] && [ -s "$d/end" ] && ap -R "$d/end"
  echo "git did not revert ` + commit.SHA[:min(12, len(commit.SHA))] + `:"; cat "$d/err"; exit 8
fi
`)
	}
	b.WriteString(`if [ -s "$d/start" ]; then
  ap "$d/start" 2>"$d/err" || { echo 'the commits are reverted, but git would not put the earlier work back:'; cat "$d/err"; exit 9; }
fi
echo undone
`)
	return b.String()
}

// failure turns the script's own words into the error the person reads.
func failure(out string, err error, reverted, reverts int) error {
	var words []string
	for _, line := range script.Lines(out) {
		if !strings.HasPrefix(line, "reverted ") {
			words = append(words, strings.TrimSpace(line))
		}
	}
	said := strings.Join(words, " ")
	if said == "" {
		said = err.Error()
	}
	if reverted > 0 {
		return fmt.Errorf("%d of %d commits were reverted, then git stopped: %s", reverted, reverts, said)
	}
	return errors.New(said)
}

func lastLine(text string) string {
	lines := script.Lines(strings.TrimSpace(text))
	if len(lines) == 0 {
		return "no answer"
	}
	return lines[len(lines)-1]
}
