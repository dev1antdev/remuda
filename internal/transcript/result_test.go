package transcript

import (
	"encoding/json"
	"testing"
)

func TestResultTextReadsAPictureAsAnswered(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"plain text"`, "plain text"},
		{`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`, "a\nb"},
		{`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}]`, "[image]"},
		{`[{"type":"text","text":"a"},{"type":"image","source":{}}]`, "a"},
		{`[]`, ""},
	}
	for _, c := range cases {
		if got := resultText(json.RawMessage(c.in)); got != c.want {
			t.Errorf("resultText(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}
