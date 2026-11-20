package core

import "testing"

// LiftPromptGate lets prompts through a core that does not check the program in front as it types, for one test.
func LiftPromptGate(t testing.TB) {
	was := promptsNeed
	promptsNeed = ""
	t.Cleanup(func() { promptsNeed = was })
}
