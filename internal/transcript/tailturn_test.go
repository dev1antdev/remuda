package transcript

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/dev1antdev/remuda/internal/shell"
)

// sizedHost is a remote machine whose session files answer as many bytes as a read asks for, counting them.
func sizedHost(fetched *int64) *shell.Fake {
	take := regexp.MustCompile(`(?:tail|head) -c (\d+) *$`)
	return &shell.Fake{Name: "gog", Dir: "/home/me", Remote: true, Answer: func(s string) (string, error) {
		m := take.FindStringSubmatch(s) // the last count of the pipe is what reaches the reader
		if m == nil {
			return "", nil
		}
		n, _ := strconv.Atoi(m[1])
		*fetched += int64(n)
		return strings.Repeat("x", n), nil
	}}
}

// TestThirteenConversationsReadInTurnTravelOnce: a wall of thirteen agents on one machine, or the sweep of thirteen
// task runs, reads their files in turn; past eight files the cache let one go at random, and every miss brought a
// whole conversation over ssh again — 840 MB in thirty rounds where the agents wrote under a megabyte.
func TestThirteenConversationsReadInTurnTravelOnce(t *testing.T) {
	var fetched int64
	r := NewReader(sizedHost(&fetched))
	const files, size, grow = 13, 3 << 20, 2048
	sizes := make([]int64, files)
	for i := range sizes {
		sizes[i] = size
	}
	for round := 0; round < 30; round++ {
		for i := range sizes {
			sizes[i] += grow
			path := fmt.Sprintf("/home/me/.claude/projects/-p/s%d.jsonl", i)
			if _, err := r.remoteTail(path, Version{Size: sizes[i], MTime: sizes[i]}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if want := int64(files*(size+grow) + 29*files*grow); fetched != want {
		t.Fatalf("%.1f MB fetched, want each file once and then what it grew by: %.1f MB", float64(fetched)/(1<<20), float64(want)/(1<<20))
	}
}
