package server

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const permissionDialog = `● Bash(rm -rf build)

Do you want to proceed?
❯ 1. Yes
  2. Yes, and don't ask again for rm commands in /Users/me/project
  3. No, and tell Claude what to do differently (esc)
`

// Two answers to one notification: the second reads the screen only once the first's keys are sent, and finds no
// dialog there any more, so its keys never reach the agent's next prompt.
func TestASecondAnswerFindsTheDialogGone(t *testing.T) {
	var mu sync.Mutex
	var state sync.Mutex
	screen := permissionDialog
	sent := 0
	read := func() (string, error) {
		state.Lock()
		s := screen
		state.Unlock()
		time.Sleep(20 * time.Millisecond) // the window in which a second read would see the same dialog
		return s, nil
	}
	send := func(keys []string) error {
		state.Lock()
		defer state.Unlock()
		sent++
		screen = "● Removed the build folder.\n\n❯ \n"
		return nil
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- answerOnScreen(&mu, read, 1, "Yes, and don't ask again for rm commands in /Users/me/project", send)
		}()
	}
	first, second := <-results, <-results
	if first != nil {
		first, second = second, first
	}
	var stale staleDialog
	if first != nil || !errors.As(second, &stale) || !strings.Contains(second.Error(), "no question") {
		t.Fatalf("one answer should land and the other find the dialog gone: %v / %v", first, second)
	}
	if sent != 1 {
		t.Fatalf("keys were sent %d times, want once", sent)
	}

	boom := errors.New("herdr is away")
	if err := answerOnScreen(&mu, func() (string, error) { return "", boom }, 0, "Yes", send); !errors.Is(err, boom) || errors.As(err, &stale) {
		t.Errorf("a failed read is the bridge's error, not a stale dialog: %v", err)
	}
	if err := answerOnScreen(&mu, func() (string, error) { return permissionDialog, nil }, 0, "Yes", func([]string) error { return boom }); !errors.Is(err, boom) {
		t.Errorf("a failed send is the bridge's error: %v", err)
	}
	if err := answerOnScreen(&mu, func() (string, error) { return permissionDialog, nil }, 0, "No", send); !errors.As(err, &stale) || !strings.Contains(err.Error(), "now reads") {
		t.Errorf("a choice that reads differently is a stale dialog: %v", err)
	}
}

// The answer route refuses a bad request before it reads any screen: what a service worker or a script can get wrong.
func TestAnswerRefusesBadRequests(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.answerRoutes(mux)
	cases := map[string]string{
		"no pane":        `{"index":0,"label":"Yes"}`,
		"no index":       `{"pane":"w1:p1","label":"Yes"}`,
		"negative index": `{"pane":"w1:p1","index":-1,"label":"Yes"}`,
		"no label":       `{"pane":"w1:p1","index":0}`,
	}
	for name, body := range cases {
		code, out := taskCall(mux, "POST", "/api/question/answer", "", body)
		msg, _ := out["error"].(string)
		if code != 400 || !strings.Contains(msg, "pane, index and label") {
			t.Errorf("%s: %d %q, want 400", name, code, msg)
		}
	}
}
