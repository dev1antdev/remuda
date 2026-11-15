package transcript

import "testing"

// What reached Claude Code's prompt box as a paste is logged inside <pasted_content> tags: the chat shows the words.
func TestUserTextDropsThePasteWrapper(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"a message that is one paste", "<pasted_content id=\"fc98\">\nfix the build\n\nand the tests\n</pasted_content id=\"fc98\">", "fix the build\n\nand the tests"},
		{"words around a paste", "look at this:\n<pasted_content id=\"a1\">\nline one\nline two\n</pasted_content id=\"a1\">\nthanks", "look at this:\nline one\nline two\nthanks"},
		{"a wrapper without an id", "<pasted_content>one</pasted_content>", "one"},
		{"two pastes", "<pasted_content id=\"a\">\none\n</pasted_content id=\"a\">\n<pasted_content id=\"b\">\ntwo\n</pasted_content id=\"b\">", "one\ntwo"},
		{"no paste", "ship it", "ship it"},
		{"words that only name the tag", "why is my text sent as pasted_content?", "why is my text sent as pasted_content?"},
	} {
		if got := userText(c.in); got != c.want {
			t.Errorf("%s: userText = %q, want %q", c.name, got, c.want)
		}
	}
}
