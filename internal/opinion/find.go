package opinion

// The reviewer's conversation found by the request it opens with. The pane's own session is read first (by the
// chat); when that is not the request — no session herdr names for the pane (a Codex pane, at times), a remote
// machine whose folder sessions are not listed, a conversation a compaction moved to a new file — the agent's
// session files of the last two days are searched for the request's ref line, and the file found followed to
// where the conversation lives now.

import (
	"strconv"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/script"
	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

// findTimeout is how long the search over a machine's session files may take.
var findTimeout = 30 * time.Second

// FindScript prints the session files of the agent's under the home that hold the request's ref line: Claude Code's
// in the folder's project, Codex's and Gemini CLI's written in the last two days; every format for another agent.
func FindScript(home, agent, dir, ref string) string {
	roots := transcript.SessionRoots(home)
	var finds []string
	add := func(root string, depth int, name string, recent bool) {
		find := "find " + script.Quote(root) + " -maxdepth " + strconv.Itoa(depth) + " -type f -name " + script.Quote(name)
		if recent {
			find += " -mtime -2"
		}
		finds = append(finds, find+" -exec grep -lF -- \"$line\" {} + 2>/dev/null")
	}
	claude := func() { add(roots[transcript.Claude]+"/"+transcript.EncodePath(dir), 1, "*.jsonl", false) }
	codex := func() { add(roots[transcript.Codex], 4, "rollout-*.jsonl", true) }
	gemini := func() { add(roots[transcript.Gemini], 3, "session-*.json*", true) }
	switch transcript.FormatOfAgent(agent) {
	case transcript.Claude:
		claude()
	case transcript.Codex:
		codex()
	case transcript.Gemini:
		gemini()
	default:
		claude()
		codex()
		gemini()
	}
	return script.PATH + "\nline=" + script.Quote(refLine(ref)) + "\n" + strings.Join(finds, "\n") + "\ntrue\n"
}

// PromptAndAnswer is a conversation's first prompt and the text its last turn ended with, "" while the person's turn
// is the last.
func PromptAndAnswer(turns []transcript.Turn) (prompt, answer string) {
	if len(turns) == 0 {
		return "", ""
	}
	if turns[0].Role == "user" {
		for _, part := range turns[0].Parts {
			if part.Type == "text" {
				prompt += part.Text
			}
		}
	}
	if last := turns[len(turns)-1]; last.Role == "assistant" {
		answer = last.Answer()
	}
	return prompt, answer
}

// Answered is the answer a conversation's turns end with, when the conversation opens with the opinion's request;
// false for any other conversation of the folder — the person's own agent, an earlier request's reviewer.
func Answered(turns []transcript.Turn, op Opinion) (string, bool) {
	prompt, answer := PromptAndAnswer(turns)
	if !IsRequest(prompt, op.Ref) {
		return "", false
	}
	return answer, true
}

// conversation reads one session file's first prompt and last answer at its present version.
func conversation(reader *transcript.Reader, path string) (prompt, answer string, ok bool) {
	version, err := reader.Version(path)
	if err != nil {
		return "", "", false
	}
	turns, err := reader.Turns(path, version)
	if err != nil {
		return "", "", false
	}
	prompt, answer = PromptAndAnswer(turns)
	return prompt, answer, true
}

// Request is the reviewer's answer read from the conversation that opens with the opinion's request, wherever it
// lives on the machine: found by its ref line, then followed to the file a compaction moved it to.
func Request(host shell.Host, reader *transcript.Reader, op Opinion) (string, bool) {
	if op.Ref == "" {
		return "", false
	}
	out, _ := shell.Text(host, FindScript(host.Home(), op.Agent, op.Dir, op.Ref), findTimeout)
	for _, path := range script.Lines(out) {
		if !transcript.UnderRoot(host.Home(), path) {
			continue
		}
		prompt, _, ok := conversation(reader, path)
		if !ok || !IsRequest(prompt, op.Ref) {
			continue
		}
		_, answer, ok := conversation(reader, reader.Live(path))
		return answer, ok
	}
	return "", false
}
