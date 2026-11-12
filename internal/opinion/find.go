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

// Reply is what a reviewer's conversation came to: the text its last turn ended with, or — Claude Code having refused
// call after call nobody was there to approve, under the second opinion's deny switch — that the turn stopped on them
// with no answer on its way.
type Reply struct {
	Answer  string
	Stopped bool
}

// PromptAndReply is a conversation's first prompt and what its last turn came to, no reply while the person's turn is
// the last.
func PromptAndReply(turns []transcript.Turn) (prompt string, reply Reply) {
	if len(turns) == 0 {
		return "", Reply{}
	}
	if turns[0].Role == "user" {
		for _, part := range turns[0].Parts {
			if part.Type == "text" {
				prompt += part.Text
			}
		}
	}
	if last := turns[len(turns)-1]; last.Role == "assistant" {
		reply.Answer = last.Answer()
		reply.Stopped = reply.Answer == "" && len(last.Parts) > 0 && transcript.IsDenied(last.Parts[len(last.Parts)-1])
	}
	return prompt, reply
}

// Answered is what a conversation's turns came to, when the conversation opens with the opinion's request; false for
// any other conversation of the folder — the person's own agent, an earlier request's reviewer.
func Answered(turns []transcript.Turn, op Opinion) (Reply, bool) {
	prompt, reply := PromptAndReply(turns)
	if !IsRequest(prompt, op.Ref) {
		return Reply{}, false
	}
	return reply, true
}

// conversation reads one session file's first prompt and what its last turn came to, at its present version.
func conversation(reader *transcript.Reader, path string) (prompt string, reply Reply, ok bool) {
	version, err := reader.Version(path)
	if err != nil {
		return "", Reply{}, false
	}
	turns, err := reader.Turns(path, version)
	if err != nil {
		return "", Reply{}, false
	}
	prompt, reply = PromptAndReply(turns)
	return prompt, reply, true
}

// Request is the reviewer's reply read from the conversation that opens with the opinion's request, wherever it lives
// on the machine: found by its ref line, then followed to the file a compaction moved it to.
func Request(host shell.Host, reader *transcript.Reader, op Opinion) (Reply, bool) {
	if op.Ref == "" {
		return Reply{}, false
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
		_, reply, ok := conversation(reader, reader.Live(path))
		return reply, ok
	}
	return Reply{}, false
}
