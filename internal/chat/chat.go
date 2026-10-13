// Package chat finds the conversation behind a pane: the session file
// herdr names, followed through the files a compaction moved it to, and
// what its last turn says. The transcript is read, never written.
package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/dev1antdev/remuda/internal/herdr"
	"github.com/dev1antdev/remuda/internal/machines"
	"github.com/dev1antdev/remuda/internal/transcript"
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
	pane, _ := c.Pane(machineID, paneID)
	if pane == nil {
		return transcript.Turn{}, false
	}
	mc, path, ok := c.SessionFile(machineID, pane)
	if !ok {
		return transcript.Turn{}, false
	}
	version, err := mc.Transcripts.Version(path)
	if err != nil {
		return transcript.Turn{}, false
	}
	turns, err := mc.Transcripts.Turns(path, version)
	if err != nil || len(turns) == 0 {
		return transcript.Turn{}, false
	}
	last := turns[len(turns)-1]
	return last, last.Role == "assistant"
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
}

// ETag changes whenever the file does.
func (conv Conversation) ETag() string {
	return fmt.Sprintf(`"%s-%s"`, conv.ID, conv.Version)
}

// Turns parses the conversation at the version Open saw.
func (conv Conversation) Turns() ([]transcript.Turn, error) {
	return conv.reader.Turns(conv.Path, conv.Version)
}

// Open finds the conversation to show: a pane's live one (followed through compactions, unless a session of its folder is asked for by id),
// or, without a pane, a session by folder and id, read-only. An unknown pane or session is a NotFoundError; a Path of "" is an empty chat.
func (c *Service) Open(mc *machines.Machine, paneID, sessionID, dir string) (Conversation, error) {
	conv := Conversation{reader: mc.Transcripts}
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
	if conv.Sessions == nil && mc.Local() && paneID != "" {
		conv.Sessions, _ = transcript.Sessions(conv.Dir)
	}
	return conv, nil
}
