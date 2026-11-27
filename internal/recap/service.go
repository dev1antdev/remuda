package recap

import (
	"time"

	"github.com/dev1antdev/remuda/internal/chat"
	"github.com/dev1antdev/remuda/internal/machines"
)

// Service reads the conversations the recaps are made of, through the chat service.
type Service struct {
	Chat *chat.Service
}

// Conversation is the recap of one conversation: a pane's live one, a session of its folder, or a transcript on its own.
func (s *Service) Conversation(mc *machines.Machine, paneID, sessionID, dir string, since time.Time) (Recap, error) {
	return s.recapOf(s.Chat.Open, mc, paneID, sessionID, dir, since)
}

// ConversationOwn is Conversation by chat.Service.OpenOwn: a pane's own conversation or none.
func (s *Service) ConversationOwn(mc *machines.Machine, paneID, sessionID, dir string, since time.Time) (Recap, error) {
	return s.recapOf(s.Chat.OpenOwn, mc, paneID, sessionID, dir, since)
}

type opener func(mc *machines.Machine, paneID, sessionID, dir string, tail bool) (chat.Conversation, error)

func (s *Service) recapOf(open opener, mc *machines.Machine, paneID, sessionID, dir string, since time.Time) (Recap, error) {
	conv, err := open(mc, paneID, sessionID, dir, false)
	if err != nil {
		return Recap{}, err
	}
	if conv.Path == "" {
		return Of(nil, since), nil
	}
	// every file the conversation lived in: a compaction moved it on, and the period may begin before that
	turns, err := conv.TurnsThroughCompactions()
	if err != nil {
		return Recap{}, err
	}
	r := Of(turns, since)
	r.Spans = Spans(turns, since, time.Now())
	return r, nil
}
