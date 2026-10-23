// Package chat finds the conversation behind a pane: the session file
// herdr names, followed through the files a compaction moved it to, and
// what its last turn says. The transcript is read, never written.
package chat

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/transcript"
	"github.com/dev1antdev/remuda/internal/usage"
	"github.com/dev1antdev/remuda/internal/watch"
)

// NotFoundError is a pane or session the lens cannot show.
type NotFoundError struct{ Err error }

func (e *NotFoundError) Error() string { return e.Err.Error() }

func (e *NotFoundError) Unwrap() error { return e.Err }

// Service reads conversations through the watcher's cache and each machine's transcripts.
type Service struct {
	Machines *machines.Manager
	Watch    *watch.Watcher

	tails tailCache
}

// Pane is a pane from the watcher's cache with its snapshot; the pane is nil when unknown, both when the machine has no snapshot.
func (c *Service) Pane(machineID, paneID string) (*herdr.PaneInfo, *herdr.Snapshot) {
	snap, _ := c.Watch.Snapshot(machineID)
	if snap == nil {
		return nil, nil
	}
	return snap.Pane(paneID), snap
}

// SessionFile is the file a pane's conversation lives in now, with the machine it is on.
func (c *Service) SessionFile(machineID string, pane *herdr.PaneInfo) (*machines.Machine, string, bool) {
	if pane.AgentSession == nil {
		return nil, "", false
	}
	mc, err := c.Machines.Get(machineID)
	if err != nil {
		return nil, "", false
	}
	path, err := mc.Transcripts.ResolveSession(pane.Dir(), pane.AgentSession.Kind, pane.AgentSession.Value)
	if err != nil {
		return nil, "", false
	}
	return mc, mc.Transcripts.Live(path), true
}

// LastWrite is when the pane's session file was last written.
func (c *Service) LastWrite(machineID string, pane *herdr.PaneInfo) (time.Time, bool) {
	mc, path, ok := c.SessionFile(machineID, pane)
	if !ok {
		return time.Time{}, false
	}
	version, err := mc.Transcripts.Version(path)
	if err != nil {
		return time.Time{}, false
	}
	return version.Time(), true
}

// LastWriteOf is LastWrite for the radar's recovery: zero when there is nothing to say.
func (c *Service) LastWriteOf(machineID string, pane *herdr.PaneInfo) time.Time {
	at, _ := c.LastWrite(machineID, pane)
	return at
}

// SessionVersion is the version of the pane's session file, for a reader that skips an unchanged file.
func (c *Service) SessionVersion(machineID, paneID string) (string, bool) {
	pane, _ := c.Pane(machineID, paneID)
	if pane == nil {
		return "", false
	}
	mc, path, ok := c.SessionFile(machineID, pane)
	if !ok {
		return "", false
	}
	version, err := mc.Transcripts.Version(path)
	if err != nil {
		return "", false
	}
	return version.String(), true
}

// LastWriteOfPane is LastWrite by pane id, for the watchdog.
func (c *Service) LastWriteOfPane(machineID, paneID string) (time.Time, bool) {
	pane, _ := c.Pane(machineID, paneID)
	if pane == nil {
		return time.Time{}, false
	}
	return c.LastWrite(machineID, pane)
}

// LastTurn is the assistant's latest turn in a pane's conversation; false when the turn is the person's or there is none.
func (c *Service) LastTurn(machineID, paneID string) (transcript.Turn, bool) {
	turns, ok := c.paneTurns(machineID, paneID)
	if !ok || len(turns) == 0 {
		return transcript.Turn{}, false
	}
	last := turns[len(turns)-1]
	return last, last.Role == "assistant"
}

// paneTurns parses a pane's live conversation whole; false when the pane or its file is unknown.
func (c *Service) paneTurns(machineID, paneID string) ([]transcript.Turn, bool) {
	pane, _ := c.Pane(machineID, paneID)
	if pane == nil {
		return nil, false
	}
	mc, path, ok := c.SessionFile(machineID, pane)
	if !ok {
		return nil, false
	}
	version, err := mc.Transcripts.Version(path)
	if err != nil {
		return nil, false
	}
	turns, err := mc.Transcripts.Turns(path, version)
	if err != nil {
		return nil, false
	}
	return turns, true
}

// PullRequest is the last pull or merge request the pane's agent named in an answer.
func (c *Service) PullRequest(machineID, paneID string) (transcript.Link, bool) {
	turns, ok := c.paneTurns(machineID, paneID)
	if !ok {
		return transcript.Link{}, false
	}
	return transcript.PullRequest(turns)
}

// LastToolCalls is the last n tool calls of the assistant's latest turn, for the loop check.
func (c *Service) LastToolCalls(machineID, paneID string, n int) []string {
	last, ok := c.LastTurn(machineID, paneID)
	if !ok {
		return nil
	}
	calls := last.ToolCalls()
	if len(calls) > n {
		calls = calls[len(calls)-n:]
	}
	return calls
}

// LastAnswer is the text the assistant's latest turn ended with and the id of its last record; "" when the turn ends in a tool call.
func (c *Service) LastAnswer(machineID, paneID string) (string, string) {
	last, ok := c.LastTurn(machineID, paneID)
	if !ok {
		return "", ""
	}
	return last.Answer(), last.LastID()
}

// Conversation is what the chat lens shows: one session file and its neighbours in the folder.
type Conversation struct {
	ID       string
	Path     string // "" when the pane has no conversation yet
	Dir      string
	Version  transcript.Version
	Sessions []transcript.SessionInfo

	reader *transcript.Reader
	tails  *tailCache
}

// ETag changes whenever the file does.
func (conv Conversation) ETag() string {
	return fmt.Sprintf(`"%s-%s"`, conv.ID, conv.Version)
}

// Turns parses the conversation at the version Open saw, every answer priced.
func (conv Conversation) Turns() ([]transcript.Turn, error) {
	turns, err := conv.reader.Turns(conv.Path, conv.Version)
	if err != nil {
		return nil, err
	}
	return usage.PriceTurns(turns), nil
}

// Tail is what a tile asks for at a file version: the last turns, and the agent's plan over the whole conversation, which
// a plan set thirty turns ago would otherwise leave out.
type Tail struct {
	Turns []transcript.Turn
	Plan  []transcript.PlanItem
}

// LastTurns is the conversation's last n turns and its plan at the version Open saw, parsed once per version however many tiles ask for them.
func (conv Conversation) LastTurns(n int) (Tail, error) {
	key := tailKey{path: conv.Path, version: conv.Version, n: n}
	if conv.tails != nil {
		if tail, ok := conv.tails.lookup(key); ok {
			return tail, nil
		}
	}
	turns, err := conv.Turns()
	if err != nil {
		return Tail{}, err
	}
	tail := Tail{Turns: lastTurns(turns, n), Plan: transcript.Plan(turns)}
	if conv.tails != nil {
		conv.tails.remember(key, tail)
	}
	return tail, nil
}

// lastTurns is the last n of the turns, all of them when there are no more than n: a copy, so the cache that
// keeps the tail does not pin the whole parsed conversation behind it.
func lastTurns(turns []transcript.Turn, n int) []transcript.Turn {
	if n <= 0 || len(turns) <= n {
		return turns
	}
	return append([]transcript.Turn(nil), turns[len(turns)-n:]...)
}

// tailCache keeps the tail of a session file at a version: the wall's tiles and the chat lens ask for the same slice within seconds of each other.
type tailCache struct {
	mu      sync.Mutex
	entries map[tailKey]Tail
	order   []tailKey // oldest first
}

type tailKey struct {
	path    string
	version transcript.Version
	n       int
}

const tailCacheSize = 64

func (c *tailCache) lookup(key tailKey) (Tail, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tail, ok := c.entries[key]
	return tail, ok
}

func (c *tailCache) remember(key tailKey, tail Tail) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[tailKey]Tail{}
	}
	if _, known := c.entries[key]; !known {
		c.order = append(c.order, key)
	}
	c.entries[key] = tail
	for len(c.order) > tailCacheSize {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
}

// Open finds the conversation to show: a pane's live one (followed through compactions, unless a session of its folder is asked for by id),
// or, without a pane, a session by folder and id, read-only. An unknown pane or session is a NotFoundError; a Path of "" is an empty chat.
// For a tail only (a tile's last turns) the folder's sessions are not listed: a tile never shows them, and the listing reads every file's title.
func (c *Service) Open(mc *machines.Machine, paneID, sessionID, dir string, tail bool) (Conversation, error) {
	conv := Conversation{reader: mc.Transcripts, tails: &c.tails}
	if paneID == "" {
		path, err := mc.Transcripts.ResolveSession(dir, "id", sessionID)
		if err != nil {
			return conv, &NotFoundError{fmt.Errorf("session %s: %w", sessionID, err)}
		}
		conv.Dir, conv.Path = dir, path
	} else {
		pane, _ := c.Pane(mc.ID(), paneID)
		if pane == nil {
			return conv, &NotFoundError{fmt.Errorf("pane %s not found", paneID)}
		}
		conv.Dir = pane.Dir()
		if sessionID != "" { // only a session of this folder: another folder's is the viewer's business
			if mc.Local() {
				conv.Sessions, _ = transcript.Sessions(conv.Dir)
				for _, session := range conv.Sessions {
					if session.ID == sessionID {
						conv.Path = session.Path
					}
				}
			} else if path, err := mc.Transcripts.ResolveSession(conv.Dir, "id", sessionID); err == nil && strings.Contains(path, "/"+transcript.EncodePath(conv.Dir)+"/") {
				conv.Path = path
			}
		}
		if conv.Path == "" && pane.AgentSession != nil && pane.AgentSession.Agent == "claude" {
			conv.Path, _ = mc.Transcripts.ResolveSession(conv.Dir, pane.AgentSession.Kind, pane.AgentSession.Value)
		}
		if conv.Path == "" && mc.Local() {
			if conv.Sessions == nil {
				conv.Sessions, _ = transcript.Sessions(conv.Dir)
			}
			if len(conv.Sessions) > 0 {
				conv.Path = conv.Sessions[0].Path
			}
		}
	}
	if conv.Path == "" {
		return conv, nil
	}
	// a compacted conversation went on in a newer file, unless this very file was asked for
	if sessionID == "" {
		conv.Path = mc.Transcripts.Live(conv.Path)
	}
	version, err := mc.Transcripts.Version(conv.Path)
	if err != nil {
		return conv, err
	}
	conv.Version = version
	conv.ID = transcript.SessionID(conv.Path)
	if conv.Sessions == nil && mc.Local() && paneID != "" && !tail {
		conv.Sessions, _ = transcript.Sessions(conv.Dir)
	}
	return conv, nil
}
