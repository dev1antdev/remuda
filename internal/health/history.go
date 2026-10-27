package health

import (
	"log"
	"time"

	"github.com/dev1antdev/remuda/internal/state"
)

// Sample is one probe's load and disk reading, kept so the ⓘ can show the day.
type Sample struct {
	At       time.Time `json:"at"`
	Load     float64   `json:"load"`
	DiskFree float64   `json:"disk_free"` // percent of the disk free
}

const (
	// HistoryKeep is how many samples a machine keeps: a day of probes 15 minutes apart.
	HistoryKeep = 96
	// HistoryFile holds every machine's samples in the state dir.
	HistoryFile = "vitals.json"
	// sampleApart is how close to the last sample a read replaces it instead of adding one (a read on demand).
	sampleApart = time.Minute
)

// history is every machine's samples, by machine id, as kept on disk.
type history map[string][]Sample

// loadHistory reads the samples kept from before; a damaged file is an empty history, said once.
func loadHistory(dir string) history {
	h := history{}
	if dir == "" {
		return h
	}
	if err := state.LoadJSON(dir, HistoryFile, &h); err != nil {
		log.Printf("health: %s is not readable (%v); the vitals start afresh", HistoryFile, err)
		return history{}
	}
	for id, samples := range h {
		h[id] = trim(samples)
	}
	return h
}

// note adds a sample to a machine's samples: one within sampleApart of the last replaces it, the oldest past HistoryKeep go.
func note(samples []Sample, s Sample) []Sample {
	if n := len(samples); n > 0 && s.At.Sub(samples[n-1].At) < sampleApart {
		samples = samples[:n-1]
	}
	return trim(append(samples, s))
}

func trim(samples []Sample) []Sample {
	if len(samples) > HistoryKeep {
		samples = samples[len(samples)-HistoryKeep:]
	}
	return samples
}

// sample is what a report adds to the history, and whether it read the vitals at all (a probe that reached nothing adds no zeros).
func sample(r Report) (Sample, bool) {
	if r.Load1 == 0 && r.DiskFreeGB == 0 {
		return Sample{}, false
	}
	return Sample{At: r.At, Load: r.Load1, DiskFree: float64(100 - r.DiskUsedPct)}, true
}

// remember keeps the report's sample for the machine and writes the history out; the file is written outside the lock.
func (c *Collector) remember(id string, r Report) {
	s, ok := sample(r)
	if !ok {
		return
	}
	c.mu.Lock()
	c.history[id] = note(c.history[id], s)
	snapshot := make(history, len(c.history))
	for k, v := range c.history {
		snapshot[k] = v
	}
	c.mu.Unlock()
	if c.dir == "" {
		return
	}
	if err := state.SaveJSON(c.dir, HistoryFile, snapshot); err != nil {
		log.Printf("health: %s not written: %v", HistoryFile, err)
	}
}
