package server

import (
	"net/http"
	"strings"
	"testing"
)

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
