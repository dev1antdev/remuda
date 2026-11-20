package core

import "testing"

// LiftPromptGate lets prompts through a core that does not check the program in front as it types, for one test.
func LiftPromptGate(t testing.TB) {
	was := promptsNeed
	promptsNeed = ""
	t.Cleanup(func() { promptsNeed = was })
}

// HideRunLiteral has a core that starts a program as it is taken for one that cannot, for one test.
func HideRunLiteral(t testing.TB) {
	was := literalRuns
	literalRuns = []string{"run_literal_hidden"}
	t.Cleanup(func() { literalRuns = was })
}

// BeforeLayoutWrite runs do between a layout's last terminal and its document write, for one test.
func BeforeLayoutWrite(t testing.TB, do func()) {
	was := layoutBeforeWrite
	layoutBeforeWrite = do
	t.Cleanup(func() { layoutBeforeWrite = was })
}

// BeforeCloseWrite runs do between a workspace close's ending of the terminals it saw and its document write, for one
// test.
func BeforeCloseWrite(t testing.TB, do func()) {
	was := closeBeforeWrite
	closeBeforeWrite = do
	t.Cleanup(func() { closeBeforeWrite = was })
}
