package chat

import (
	"errors"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
	"github.com/dev1antdev/remuda/internal/transcript"
)

func TestETagMovesWithTheSubagents(t *testing.T) {
	plain := Conversation{ID: "s", Version: transcript.Version{Size: 1, MTime: 2}}
	withSubagents := plain
	withSubagents.stamp = "abc"
	moved := withSubagents
	moved.stamp = "abd"
	if plain.ETag() == withSubagents.ETag() || withSubagents.ETag() == moved.ETag() {
		t.Fatalf("etags %s, %s, %s should all differ", plain.ETag(), withSubagents.ETag(), moved.ETag())
	}
	if plain.ETag() != `"s-1-2"` {
		t.Fatalf("a conversation without subagents keeps the old etag: %s", plain.ETag())
	}
}

func TestSubagentUnknownToTheConversationIsNotFound(t *testing.T) {
	conv := Conversation{Path: "/nowhere/s.jsonl", Subagents: []transcript.Subagent{{ID: "agent-a1"}}, reader: transcript.NewReader(shell.Local{})}
	_, _, err := conv.Subagent("agent-zz")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("an id the listing does not have should be a NotFoundError, got %v", err)
	}
}
