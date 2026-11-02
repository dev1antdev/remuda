package health

import (
	"sync"
	"testing"
	"time"
)

// TestRememberWritesACopyOfTheHistory: a probe of one machine writes the file while another machine's probe replaces
// its last sample in place, so the snapshot written must be the collector's own copy, not the slices it still changes.
func TestRememberWritesACopyOfTheHistory(t *testing.T) {
	c := New(t.TempDir(), nil, "v1", false)
	at := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for _, id := range []string{"local", "gog"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				c.remember(id, Report{At: at.Add(time.Duration(i) * time.Second), Load1: float64(i + 1), DiskFreeGB: 100, DiskUsedPct: 50})
			}
		}()
	}
	wg.Wait()
	for _, id := range []string{"local", "gog"} {
		if h := New(c.dir, nil, "v1", false).history[id]; len(h) != 1 || h[0].Load != 40 {
			t.Fatalf("%s: reads within a minute replace one another, the last kept: %+v", id, h)
		}
	}
}
