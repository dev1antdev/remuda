package prstatus

import (
	"strings"
	"testing"
	"time"

	"github.com/dev1antdev/remuda/internal/shell"
)

func TestAFailedLogIsCutOnTheMachine(t *testing.T) {
	m, _ := MergeParts("https://gitlab.com/team/app/-/merge_requests/9")
	for _, text := range []string{LogScript("github.com/o/r", "123"), TraceScript(m, 77)} {
		if !strings.HasSuffix(text, lastOfLog) {
			t.Fatalf("a whole CI log crosses the link: %q", text)
		}
	}
	long := "{ i=0; while [ $i -lt 60000 ]; do echo noise; i=$((i+1)); done; echo 'FAIL: TestThing'; }" // a log of 360 KB
	out, err := shell.Text(shell.Local{Name: "local"}, long+lastOfLog, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	whole := 60000*len("noise\n") + len("FAIL: TestThing\n")
	if len(out) > LogRead || !strings.HasSuffix(out, "FAIL: TestThing\n") {
		t.Fatalf("%d of %d bytes arrived, ending %q", len(out), whole, out[max(0, len(out)-40):])
	}
	if tail := Tail(out); !strings.Contains(tail, "FAIL: TestThing") || len(tail) > LogTail+8 {
		t.Fatalf("tail of %d bytes: %q", len(tail), tail[max(0, len(tail)-40):])
	}
}
